package cri

import (
	"context"
	"fmt"

	"github.com/dpopsuev/misbah/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Client wraps CRI gRPC connections for runtime and image operations.
type Client struct {
	conn    *grpc.ClientConn
	runtime runtimeapi.RuntimeServiceClient
	image   runtimeapi.ImageServiceClient
	logger  *metrics.Logger
}

// NewClient creates a CRI gRPC client connected to the given endpoint.
func NewClient(endpoint string, logger *metrics.Logger) (*Client, error) {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}

	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to CRI endpoint %s: %w", endpoint, err)
	}

	return &Client{
		conn:    conn,
		runtime: runtimeapi.NewRuntimeServiceClient(conn),
		image:   runtimeapi.NewImageServiceClient(conn),
		logger:  logger,
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// PullImage ensures an image is available locally.
func (c *Client) PullImage(ctx context.Context, imageRef string) error {
	c.logger.Infof("Pulling image: %s", imageRef)

	_, err := c.image.PullImage(ctx, &runtimeapi.PullImageRequest{
		Image: &runtimeapi.ImageSpec{
			Image: imageRef,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageRef, err)
	}

	c.logger.Infof("Image pulled: %s", imageRef)
	return nil
}

// RunPodSandbox creates and starts a pod sandbox with the given runtime handler.
func (c *Client) RunPodSandbox(ctx context.Context, name, runtimeHandler string) (string, error) {
	c.logger.Infof("Creating pod sandbox: %s (handler=%s)", name, runtimeHandler)

	config := BuildPodSandboxConfig(name)

	resp, err := c.runtime.RunPodSandbox(ctx, &runtimeapi.RunPodSandboxRequest{
		Config:         config,
		RuntimeHandler: runtimeHandler,
	})
	if err != nil {
		return "", fmt.Errorf("failed to run pod sandbox %s: %w", name, err)
	}

	c.logger.Infof("Pod sandbox created: %s -> %s", name, resp.PodSandboxId)
	return resp.PodSandboxId, nil
}

// CreateContainer creates a container within a pod sandbox.
func (c *Client) CreateContainer(ctx context.Context, sandboxID string, containerConfig *runtimeapi.ContainerConfig, sandboxConfig *runtimeapi.PodSandboxConfig) (string, error) {
	c.logger.Infof("Creating container in sandbox %s", sandboxID)

	resp, err := c.runtime.CreateContainer(ctx, &runtimeapi.CreateContainerRequest{
		PodSandboxId:  sandboxID,
		Config:        containerConfig,
		SandboxConfig: sandboxConfig,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create container in sandbox %s: %w", sandboxID, err)
	}

	c.logger.Infof("Container created: %s", resp.ContainerId)
	return resp.ContainerId, nil
}

// StartContainer starts a previously created container.
func (c *Client) StartContainer(ctx context.Context, containerID string) error {
	c.logger.Infof("Starting container: %s", containerID)

	_, err := c.runtime.StartContainer(ctx, &runtimeapi.StartContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		return fmt.Errorf("failed to start container %s: %w", containerID, err)
	}

	c.logger.Infof("Container started: %s", containerID)
	return nil
}

// StopContainer stops a running container with a timeout.
func (c *Client) StopContainer(ctx context.Context, containerID string, timeout int64) error {
	c.logger.Infof("Stopping container: %s (timeout=%ds)", containerID, timeout)

	_, err := c.runtime.StopContainer(ctx, &runtimeapi.StopContainerRequest{
		ContainerId: containerID,
		Timeout:     timeout,
	})
	if err != nil {
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}

	c.logger.Infof("Container stopped: %s", containerID)
	return nil
}

// RemoveContainer removes a stopped container.
func (c *Client) RemoveContainer(ctx context.Context, containerID string) error {
	c.logger.Infof("Removing container: %s", containerID)

	_, err := c.runtime.RemoveContainer(ctx, &runtimeapi.RemoveContainerRequest{
		ContainerId: containerID,
	})
	if err != nil {
		return fmt.Errorf("failed to remove container %s: %w", containerID, err)
	}

	c.logger.Infof("Container removed: %s", containerID)
	return nil
}

// StopPodSandbox stops a pod sandbox.
func (c *Client) StopPodSandbox(ctx context.Context, sandboxID string) error {
	c.logger.Infof("Stopping pod sandbox: %s", sandboxID)

	_, err := c.runtime.StopPodSandbox(ctx, &runtimeapi.StopPodSandboxRequest{
		PodSandboxId: sandboxID,
	})
	if err != nil {
		return fmt.Errorf("failed to stop pod sandbox %s: %w", sandboxID, err)
	}

	c.logger.Infof("Pod sandbox stopped: %s", sandboxID)
	return nil
}

// RemovePodSandbox removes a stopped pod sandbox.
func (c *Client) RemovePodSandbox(ctx context.Context, sandboxID string) error {
	c.logger.Infof("Removing pod sandbox: %s", sandboxID)

	_, err := c.runtime.RemovePodSandbox(ctx, &runtimeapi.RemovePodSandboxRequest{
		PodSandboxId: sandboxID,
	})
	if err != nil {
		return fmt.Errorf("failed to remove pod sandbox %s: %w", sandboxID, err)
	}

	c.logger.Infof("Pod sandbox removed: %s", sandboxID)
	return nil
}

// ExecSync executes a command in a container synchronously.
func (c *Client) ExecSync(ctx context.Context, containerID string, cmd []string, timeout int64) ([]byte, []byte, int32, error) {
	c.logger.Debugf("Exec in container %s: %v", containerID, cmd)

	resp, err := c.runtime.ExecSync(ctx, &runtimeapi.ExecSyncRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		Timeout:     timeout,
	})
	if err != nil {
		return nil, nil, -1, fmt.Errorf("failed to exec in container %s: %w", containerID, err)
	}

	return resp.Stdout, resp.Stderr, resp.ExitCode, nil
}

// ListContainers lists all containers matching optional filters.
func (c *Client) ListContainers(ctx context.Context) ([]*runtimeapi.Container, error) {
	resp, err := c.runtime.ListContainers(ctx, &runtimeapi.ListContainersRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	return resp.Containers, nil
}

// ContainerStatus returns the status of a container.
func (c *Client) ContainerStatus(ctx context.Context, containerID string) (*runtimeapi.ContainerStatus, error) {
	resp, err := c.runtime.ContainerStatus(ctx, &runtimeapi.ContainerStatusRequest{
		ContainerId: containerID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get container status %s: %w", containerID, err)
	}

	return resp.Status, nil
}

// ListImages lists all images available locally.
func (c *Client) ListImages(ctx context.Context) ([]*runtimeapi.Image, error) {
	resp, err := c.image.ListImages(ctx, &runtimeapi.ListImagesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}
	return resp.Images, nil
}

// ImageStatus returns the status of a specific image.
func (c *Client) ImageStatus(ctx context.Context, imageRef string) (*runtimeapi.Image, error) {
	resp, err := c.image.ImageStatus(ctx, &runtimeapi.ImageStatusRequest{
		Image: &runtimeapi.ImageSpec{Image: imageRef},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get image status %s: %w", imageRef, err)
	}
	return resp.Image, nil
}

// RemoveImage removes an image.
func (c *Client) RemoveImage(ctx context.Context, imageRef string) error {
	_, err := c.image.RemoveImage(ctx, &runtimeapi.RemoveImageRequest{
		Image: &runtimeapi.ImageSpec{Image: imageRef},
	})
	if err != nil {
		return fmt.Errorf("failed to remove image %s: %w", imageRef, err)
	}
	return nil
}
