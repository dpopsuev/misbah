package cri

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// fakeRuntimeService implements a minimal RuntimeServiceServer for testing.
type fakeRuntimeService struct {
	runtimeapi.UnimplementedRuntimeServiceServer

	containers map[string]*runtimeapi.ContainerStatus
	sandboxes  map[string]bool
	nextID     int
}

func newFakeRuntimeService() *fakeRuntimeService {
	return &fakeRuntimeService{
		containers: make(map[string]*runtimeapi.ContainerStatus),
		sandboxes:  make(map[string]bool),
	}
}

func (f *fakeRuntimeService) RunPodSandbox(_ context.Context, req *runtimeapi.RunPodSandboxRequest) (*runtimeapi.RunPodSandboxResponse, error) {
	f.nextID++
	id := fmt.Sprintf("sandbox-%d", f.nextID)
	f.sandboxes[id] = true
	return &runtimeapi.RunPodSandboxResponse{PodSandboxId: id}, nil
}

func (f *fakeRuntimeService) StopPodSandbox(_ context.Context, req *runtimeapi.StopPodSandboxRequest) (*runtimeapi.StopPodSandboxResponse, error) {
	if !f.sandboxes[req.PodSandboxId] {
		return nil, fmt.Errorf("sandbox not found: %s", req.PodSandboxId)
	}
	return &runtimeapi.StopPodSandboxResponse{}, nil
}

func (f *fakeRuntimeService) RemovePodSandbox(_ context.Context, req *runtimeapi.RemovePodSandboxRequest) (*runtimeapi.RemovePodSandboxResponse, error) {
	delete(f.sandboxes, req.PodSandboxId)
	return &runtimeapi.RemovePodSandboxResponse{}, nil
}

func (f *fakeRuntimeService) CreateContainer(_ context.Context, req *runtimeapi.CreateContainerRequest) (*runtimeapi.CreateContainerResponse, error) {
	if !f.sandboxes[req.PodSandboxId] {
		return nil, fmt.Errorf("sandbox not found: %s", req.PodSandboxId)
	}
	f.nextID++
	id := fmt.Sprintf("container-%d", f.nextID)
	f.containers[id] = &runtimeapi.ContainerStatus{
		Id:    id,
		State: runtimeapi.ContainerState_CONTAINER_CREATED,
	}
	return &runtimeapi.CreateContainerResponse{ContainerId: id}, nil
}

func (f *fakeRuntimeService) StartContainer(_ context.Context, req *runtimeapi.StartContainerRequest) (*runtimeapi.StartContainerResponse, error) {
	status, ok := f.containers[req.ContainerId]
	if !ok {
		return nil, fmt.Errorf("container not found: %s", req.ContainerId)
	}
	status.State = runtimeapi.ContainerState_CONTAINER_RUNNING
	return &runtimeapi.StartContainerResponse{}, nil
}

func (f *fakeRuntimeService) StopContainer(_ context.Context, req *runtimeapi.StopContainerRequest) (*runtimeapi.StopContainerResponse, error) {
	status, ok := f.containers[req.ContainerId]
	if !ok {
		return nil, fmt.Errorf("container not found: %s", req.ContainerId)
	}
	status.State = runtimeapi.ContainerState_CONTAINER_EXITED
	return &runtimeapi.StopContainerResponse{}, nil
}

func (f *fakeRuntimeService) RemoveContainer(_ context.Context, req *runtimeapi.RemoveContainerRequest) (*runtimeapi.RemoveContainerResponse, error) {
	delete(f.containers, req.ContainerId)
	return &runtimeapi.RemoveContainerResponse{}, nil
}

func (f *fakeRuntimeService) ContainerStatus(_ context.Context, req *runtimeapi.ContainerStatusRequest) (*runtimeapi.ContainerStatusResponse, error) {
	status, ok := f.containers[req.ContainerId]
	if !ok {
		return nil, fmt.Errorf("container not found: %s", req.ContainerId)
	}
	return &runtimeapi.ContainerStatusResponse{Status: status}, nil
}

func (f *fakeRuntimeService) ListContainers(_ context.Context, req *runtimeapi.ListContainersRequest) (*runtimeapi.ListContainersResponse, error) {
	var containers []*runtimeapi.Container
	for id, status := range f.containers {
		containers = append(containers, &runtimeapi.Container{
			Id:    id,
			State: status.State,
		})
	}
	return &runtimeapi.ListContainersResponse{Containers: containers}, nil
}

func (f *fakeRuntimeService) ExecSync(_ context.Context, req *runtimeapi.ExecSyncRequest) (*runtimeapi.ExecSyncResponse, error) {
	if _, ok := f.containers[req.ContainerId]; !ok {
		return nil, fmt.Errorf("container not found: %s", req.ContainerId)
	}
	return &runtimeapi.ExecSyncResponse{
		Stdout:   []byte("hello\n"),
		Stderr:   nil,
		ExitCode: 0,
	}, nil
}

// fakeImageService implements a minimal ImageServiceServer for testing.
type fakeImageService struct {
	runtimeapi.UnimplementedImageServiceServer
	images map[string]bool
}

func newFakeImageService() *fakeImageService {
	return &fakeImageService{images: make(map[string]bool)}
}

func (f *fakeImageService) PullImage(_ context.Context, req *runtimeapi.PullImageRequest) (*runtimeapi.PullImageResponse, error) {
	ref := req.Image.Image
	f.images[ref] = true
	return &runtimeapi.PullImageResponse{ImageRef: ref}, nil
}

// startFakeServer starts a fake CRI gRPC server and returns a client connected to it.
func startFakeServer(t *testing.T) (*Client, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	runtimeapi.RegisterRuntimeServiceServer(server, newFakeRuntimeService())
	runtimeapi.RegisterImageServiceServer(server, newFakeImageService())

	go func() { _ = server.Serve(lis) }()

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	client := &Client{
		conn:    conn,
		runtime: runtimeapi.NewRuntimeServiceClient(conn),
		image:   runtimeapi.NewImageServiceClient(conn),
		logger:  logger,
	}

	cleanup := func() {
		client.Close()
		server.Stop()
		lis.Close()
	}

	return client, cleanup
}

func TestPullImage(t *testing.T) {
	client, cleanup := startFakeServer(t)
	defer cleanup()

	err := client.PullImage(context.Background(), "alpine:latest")
	assert.NoError(t, err)
}

func TestRunPodSandbox(t *testing.T) {
	client, cleanup := startFakeServer(t)
	defer cleanup()

	sandboxID, err := client.RunPodSandbox(context.Background(), "test-sandbox", "kata")
	require.NoError(t, err)
	assert.NotEmpty(t, sandboxID)
	assert.Contains(t, sandboxID, "sandbox-")
}

func TestContainerLifecycle(t *testing.T) {
	client, cleanup := startFakeServer(t)
	defer cleanup()

	ctx := context.Background()

	// 1. Create sandbox
	sandboxID, err := client.RunPodSandbox(ctx, "test", "kata")
	require.NoError(t, err)

	// 2. Create container
	sandboxConfig := BuildPodSandboxConfig("test")
	containerConfig := &runtimeapi.ContainerConfig{
		Metadata: &runtimeapi.ContainerMetadata{Name: "test-container"},
		Image:    &runtimeapi.ImageSpec{Image: "alpine:latest"},
		Command:  []string{"/bin/sh"},
	}

	containerID, err := client.CreateContainer(ctx, sandboxID, containerConfig, sandboxConfig)
	require.NoError(t, err)
	assert.NotEmpty(t, containerID)

	// 3. Start container
	err = client.StartContainer(ctx, containerID)
	require.NoError(t, err)

	// 4. Get status
	status, err := client.ContainerStatus(ctx, containerID)
	require.NoError(t, err)
	assert.Equal(t, runtimeapi.ContainerState_CONTAINER_RUNNING, status.State)

	// 5. Exec
	stdout, stderr, exitCode, err := client.ExecSync(ctx, containerID, []string{"echo", "hello"}, 10)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(stdout))
	assert.Nil(t, stderr)
	assert.Equal(t, int32(0), exitCode)

	// 6. List
	containers, err := client.ListContainers(ctx)
	require.NoError(t, err)
	assert.Len(t, containers, 1)

	// 7. Stop
	err = client.StopContainer(ctx, containerID, 10)
	require.NoError(t, err)

	// 8. Remove container
	err = client.RemoveContainer(ctx, containerID)
	require.NoError(t, err)

	// 9. Stop and remove sandbox
	err = client.StopPodSandbox(ctx, sandboxID)
	require.NoError(t, err)
	err = client.RemovePodSandbox(ctx, sandboxID)
	require.NoError(t, err)
}

func TestCreateContainer_InvalidSandbox(t *testing.T) {
	client, cleanup := startFakeServer(t)
	defer cleanup()

	sandboxConfig := BuildPodSandboxConfig("test")
	containerConfig := &runtimeapi.ContainerConfig{
		Metadata: &runtimeapi.ContainerMetadata{Name: "test"},
		Image:    &runtimeapi.ImageSpec{Image: "alpine:latest"},
	}

	_, err := client.CreateContainer(context.Background(), "nonexistent", containerConfig, sandboxConfig)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox not found")
}

func TestStartContainer_NotFound(t *testing.T) {
	client, cleanup := startFakeServer(t)
	defer cleanup()

	err := client.StartContainer(context.Background(), "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "container not found")
}

func TestExecSync_NotFound(t *testing.T) {
	client, cleanup := startFakeServer(t)
	defer cleanup()

	_, _, _, err := client.ExecSync(context.Background(), "nonexistent", []string{"echo"}, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "container not found")
}

func TestClientClose(t *testing.T) {
	client, cleanup := startFakeServer(t)
	defer cleanup()

	err := client.Close()
	assert.NoError(t, err)
}

func TestClientClose_Nil(t *testing.T) {
	client := &Client{}
	err := client.Close()
	assert.NoError(t, err)
}
