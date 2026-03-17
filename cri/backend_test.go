package cri

import (
	"net"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// startFakeBackend creates a Backend connected to a fake CRI server.
func startFakeBackend(t *testing.T) (*Backend, func()) {
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

	backend := &Backend{
		client:         client,
		runtimeHandler: "kata",
		logger:         logger,
		sandboxes:      make(map[string]string),
		containers:     make(map[string]string),
	}

	cleanup := func() {
		backend.Close()
		server.Stop()
		lis.Close()
	}

	return backend, cleanup
}

func testSpec() *model.ContainerSpec {
	return &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: "test-agent",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh", "-c", "echo hello"},
			Env:     []string{"FOO=bar"},
			Cwd:     "/workspace",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Image:   "alpine:latest",
		Runtime: "kata",
	}
}

func TestBackend_StartAndStop(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	spec := testSpec()

	// Start
	containerID, err := backend.Start(spec)
	require.NoError(t, err)
	assert.NotEmpty(t, containerID)

	// Verify tracked
	backend.mu.Lock()
	assert.NotEmpty(t, backend.containers["test-agent"])
	assert.NotEmpty(t, backend.sandboxes["test-agent"])
	backend.mu.Unlock()

	// Stop
	err = backend.Stop("test-agent", false)
	require.NoError(t, err)

	// Verify cleaned up
	backend.mu.Lock()
	assert.Empty(t, backend.containers["test-agent"])
	assert.Empty(t, backend.sandboxes["test-agent"])
	backend.mu.Unlock()
}

func TestBackend_StartAndDestroy(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	spec := testSpec()

	containerID, err := backend.Start(spec)
	require.NoError(t, err)
	assert.NotEmpty(t, containerID)

	err = backend.Destroy("test-agent")
	require.NoError(t, err)

	backend.mu.Lock()
	assert.Empty(t, backend.containers["test-agent"])
	backend.mu.Unlock()
}

func TestBackend_Exec(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	spec := testSpec()

	_, err := backend.Start(spec)
	require.NoError(t, err)

	stdout, stderr, exitCode, err := backend.Exec("test-agent", []string{"echo", "hello"}, 10)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(stdout))
	assert.Nil(t, stderr)
	assert.Equal(t, int32(0), exitCode)
}

func TestBackend_Exec_NotFound(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	_, _, _, err := backend.Exec("nonexistent", []string{"echo"}, 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no CRI container found")
}

func TestBackend_Status(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	spec := testSpec()

	_, err := backend.Start(spec)
	require.NoError(t, err)

	info, err := backend.Status("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", info.Name)
	assert.Equal(t, "running", info.State)
	assert.NotEmpty(t, info.ID)
	assert.NotEmpty(t, info.SandboxID)
}

func TestBackend_Status_NotFound(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	_, err := backend.Status("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no CRI container found")
}

func TestBackend_List(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	// Empty list
	infos, err := backend.List()
	require.NoError(t, err)
	assert.Empty(t, infos)

	// Start a container — note: fake server doesn't set labels,
	// so List() filters them out (misbah.dev/managed != "true").
	// This tests the filtering logic.
	spec := testSpec()
	_, err = backend.Start(spec)
	require.NoError(t, err)

	infos, err = backend.List()
	require.NoError(t, err)
	// Fake containers don't have misbah labels, so filtered out
	assert.Empty(t, infos)
}

func TestBackend_Stop_NotFound(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	err := backend.Stop("nonexistent", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no CRI container found")
}

func TestBackend_StartWithNetwork(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	spec := testSpec()
	spec.Network = &model.NetworkConfig{
		Mode:       "none",
		DNSServers: []string{"8.8.8.8"},
		Hostname:   "test-host",
	}

	containerID, err := backend.Start(spec)
	require.NoError(t, err)
	assert.NotEmpty(t, containerID)

	// Verify it started (network config is applied to sandbox, not container)
	info, err := backend.Status("test-agent")
	require.NoError(t, err)
	assert.Equal(t, "running", info.State)
}

func TestBackend_MultipleContainers(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	spec1 := testSpec()
	spec1.Metadata.Name = "agent-1"

	spec2 := testSpec()
	spec2.Metadata.Name = "agent-2"

	id1, err := backend.Start(spec1)
	require.NoError(t, err)

	id2, err := backend.Start(spec2)
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2)

	// Both tracked
	backend.mu.Lock()
	assert.Len(t, backend.containers, 2)
	assert.Len(t, backend.sandboxes, 2)
	backend.mu.Unlock()

	// Stop one
	err = backend.Stop("agent-1", false)
	require.NoError(t, err)

	backend.mu.Lock()
	assert.Len(t, backend.containers, 1)
	assert.NotEmpty(t, backend.containers["agent-2"])
	backend.mu.Unlock()

	// Stop other
	err = backend.Stop("agent-2", true)
	require.NoError(t, err)

	backend.mu.Lock()
	assert.Empty(t, backend.containers)
	backend.mu.Unlock()
}

func TestBackend_Close(t *testing.T) {
	backend, cleanup := startFakeBackend(t)
	defer cleanup()

	err := backend.Close()
	assert.NoError(t, err)
}
