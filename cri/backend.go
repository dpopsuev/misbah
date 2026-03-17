package cri

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/dpopsuev/misbah/runtime"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Backend implements runtime.ContainerBackend using CRI gRPC.
// One pod sandbox per container (simplest model).
type Backend struct {
	client         *Client
	runtimeHandler string
	logger         *metrics.Logger

	mu         sync.Mutex
	sandboxes  map[string]string // name -> sandboxID
	containers map[string]string // name -> containerID
}

// NewBackend creates a CRI-based container backend.
func NewBackend(endpoint, runtimeHandler string, logger *metrics.Logger) (*Backend, error) {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}

	client, err := NewClient(endpoint, logger)
	if err != nil {
		return nil, err
	}

	return &Backend{
		client:         client,
		runtimeHandler: runtimeHandler,
		logger:         logger,
		sandboxes:      make(map[string]string),
		containers:     make(map[string]string),
	}, nil
}

// Start creates and starts a container via CRI.
// Flow: PullImage -> RunPodSandbox -> CreateContainer -> StartContainer
func (b *Backend) Start(spec *model.ContainerSpec) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	name := spec.Metadata.Name

	// 1. Pull image
	if err := b.client.PullImage(ctx, spec.Image); err != nil {
		return "", fmt.Errorf("pull image: %w", err)
	}

	// 2. Create pod sandbox
	sandboxConfig := BuildPodSandboxConfig(name)
	ApplyNetworkConfig(sandboxConfig, spec.Network)
	sandboxID, err := b.client.RunPodSandbox(ctx, name, b.runtimeHandler)
	if err != nil {
		return "", fmt.Errorf("run pod sandbox: %w", err)
	}

	b.mu.Lock()
	b.sandboxes[name] = sandboxID
	b.mu.Unlock()

	// 3. Create container
	containerConfig := BuildContainerConfig(spec)
	containerID, err := b.client.CreateContainer(ctx, sandboxID, containerConfig, sandboxConfig)
	if err != nil {
		_ = b.cleanupSandbox(ctx, sandboxID)
		return "", fmt.Errorf("create container: %w", err)
	}

	b.mu.Lock()
	b.containers[name] = containerID
	b.mu.Unlock()

	// 4. Start container
	if err := b.client.StartContainer(ctx, containerID); err != nil {
		_ = b.cleanupContainer(ctx, containerID, sandboxID)
		return "", fmt.Errorf("start container: %w", err)
	}

	b.logger.Infof("CRI container started: %s (container=%s, sandbox=%s)", name, containerID, sandboxID)
	return containerID, nil
}

// Stop stops a running container.
// Flow: StopContainer -> RemoveContainer -> StopPodSandbox -> RemovePodSandbox
func (b *Backend) Stop(name string, force bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	b.mu.Lock()
	containerID := b.containers[name]
	sandboxID := b.sandboxes[name]
	b.mu.Unlock()

	if containerID == "" {
		return fmt.Errorf("no CRI container found for %s", name)
	}

	timeout := int64(10)
	if force {
		timeout = 0
	}

	return b.cleanupFull(ctx, containerID, sandboxID, timeout, name)
}

// Destroy removes all resources for a container.
func (b *Backend) Destroy(name string) error {
	return b.Stop(name, true)
}

// Exec executes a command in a running container.
func (b *Backend) Exec(name string, cmd []string, timeout int64) ([]byte, []byte, int32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	b.mu.Lock()
	containerID := b.containers[name]
	b.mu.Unlock()

	if containerID == "" {
		return nil, nil, -1, fmt.Errorf("no CRI container found for %s", name)
	}

	return b.client.ExecSync(ctx, containerID, cmd, timeout)
}

// Status returns the status of a container.
func (b *Backend) Status(name string) (*runtime.ContainerInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	b.mu.Lock()
	containerID := b.containers[name]
	sandboxID := b.sandboxes[name]
	b.mu.Unlock()

	if containerID == "" {
		return nil, fmt.Errorf("no CRI container found for %s", name)
	}

	status, err := b.client.ContainerStatus(ctx, containerID)
	if err != nil {
		return nil, err
	}

	return &runtime.ContainerInfo{
		ID:        containerID,
		Name:      name,
		State:     containerStateString(status.State),
		SandboxID: sandboxID,
		CreatedAt: status.CreatedAt,
		ExitCode:  status.ExitCode,
	}, nil
}

// List returns all tracked containers.
func (b *Backend) List() ([]*runtime.ContainerInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	criContainers, err := b.client.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	var result []*runtime.ContainerInfo
	for _, c := range criContainers {
		if c.Labels["misbah.dev/managed"] != "true" {
			continue
		}
		result = append(result, &runtime.ContainerInfo{
			ID:    c.Id,
			Name:  c.Labels["misbah.dev/name"],
			State: containerStateString(c.State),
		})
	}

	return result, nil
}

// Close closes the CRI client connection.
func (b *Backend) Close() error {
	return b.client.Close()
}

func (b *Backend) cleanupContainer(ctx context.Context, containerID, sandboxID string) error {
	_ = b.client.RemoveContainer(ctx, containerID)
	return b.cleanupSandbox(ctx, sandboxID)
}

func (b *Backend) cleanupSandbox(ctx context.Context, sandboxID string) error {
	_ = b.client.StopPodSandbox(ctx, sandboxID)
	return b.client.RemovePodSandbox(ctx, sandboxID)
}

func (b *Backend) cleanupFull(ctx context.Context, containerID, sandboxID string, timeout int64, name string) error {
	if err := b.client.StopContainer(ctx, containerID, timeout); err != nil {
		b.logger.Warnf("Failed to stop container: %v", err)
	}
	if err := b.client.RemoveContainer(ctx, containerID); err != nil {
		b.logger.Warnf("Failed to remove container: %v", err)
	}
	if sandboxID != "" {
		if err := b.client.StopPodSandbox(ctx, sandboxID); err != nil {
			b.logger.Warnf("Failed to stop sandbox: %v", err)
		}
		if err := b.client.RemovePodSandbox(ctx, sandboxID); err != nil {
			b.logger.Warnf("Failed to remove sandbox: %v", err)
		}
	}

	b.mu.Lock()
	delete(b.containers, name)
	delete(b.sandboxes, name)
	b.mu.Unlock()

	return nil
}

func containerStateString(state runtimeapi.ContainerState) string {
	switch state {
	case runtimeapi.ContainerState_CONTAINER_CREATED:
		return "created"
	case runtimeapi.ContainerState_CONTAINER_RUNNING:
		return "running"
	case runtimeapi.ContainerState_CONTAINER_EXITED:
		return "exited"
	case runtimeapi.ContainerState_CONTAINER_UNKNOWN:
		return "unknown"
	default:
		return "unknown"
	}
}
