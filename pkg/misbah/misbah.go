// Package misbah is the public facade for Djinn integration.
//
// Import this package instead of reaching into misbah internals.
// It wraps daemon.Client with a higher-level API and re-exports
// key types so consumers don't need to import model/ or proxy/.
package misbah

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/dpopsuev/misbah/daemon"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
)

// DefaultSocketPath is the standard daemon socket location.
const DefaultSocketPath = "/run/misbah/permission.sock"

// Re-export key types so consumers don't import internals.
type (
	ContainerSpec  = model.ContainerSpec
	ContainerInfo  = model.ContainerInfo
	NamespaceSpec  = model.NamespaceSpec
	MountSpec      = model.MountSpec
	ProcessSpec    = model.ProcessSpec
	TierConfig     = model.TierConfig
	ResourceSpec   = model.ResourceSpec
)

// ExecResult holds the output of a command executed inside a container.
type ExecResult struct {
	ExitCode int32
	Stdout   string
	Stderr   string
}

// Misbah is the facade for container lifecycle operations.
type Misbah struct {
	client     *daemon.Client
	socketPath string
}

// New creates a Misbah facade connecting to the daemon at the given socket path.
// Use DefaultSocketPath for the standard location.
func New(socketPath string) *Misbah {
	logger := metrics.GetDefaultLogger()
	return &Misbah{
		client:     daemon.NewClient(socketPath, logger),
		socketPath: socketPath,
	}
}

// Close releases the facade's resources.
func (m *Misbah) Close() {
	m.client.Close()
}

// Available checks if the daemon socket is reachable.
func (m *Misbah) Available() bool {
	conn, err := net.DialTimeout("unix", m.socketPath, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Create starts a new container from the given spec.
// Returns the container ID.
func (m *Misbah) Create(ctx context.Context, spec *ContainerSpec) (string, error) {
	resp, err := m.client.ContainerStart(ctx, spec)
	if err != nil {
		return "", fmt.Errorf("misbah: create: %w", err)
	}
	return resp.ID, nil
}

// Stop gracefully stops a running container.
func (m *Misbah) Stop(ctx context.Context, name string) error {
	return m.client.ContainerStop(ctx, name, false)
}

// ForceStop forcefully terminates a container.
func (m *Misbah) ForceStop(ctx context.Context, name string) error {
	return m.client.ContainerStop(ctx, name, true)
}

// Destroy removes a container and cleans up its resources.
func (m *Misbah) Destroy(ctx context.Context, name string) error {
	return m.client.ContainerDestroy(ctx, name)
}

// Exec runs a command inside a container and returns the captured output.
func (m *Misbah) Exec(ctx context.Context, name string, cmd []string) (ExecResult, error) {
	resp, err := m.client.ContainerExec(ctx, name, cmd, 120)
	if err != nil {
		return ExecResult{}, fmt.Errorf("misbah: exec: %w", err)
	}
	return ExecResult{
		ExitCode: resp.ExitCode,
		Stdout:   resp.Stdout,
		Stderr:   resp.Stderr,
	}, nil
}

// ExecTimeout runs a command with a custom timeout in seconds.
func (m *Misbah) ExecTimeout(ctx context.Context, name string, cmd []string, timeoutSec int64) (ExecResult, error) {
	resp, err := m.client.ContainerExec(ctx, name, cmd, timeoutSec)
	if err != nil {
		return ExecResult{}, fmt.Errorf("misbah: exec: %w", err)
	}
	return ExecResult{
		ExitCode: resp.ExitCode,
		Stdout:   resp.Stdout,
		Stderr:   resp.Stderr,
	}, nil
}

// Status returns the current state of a container.
func (m *Misbah) Status(ctx context.Context, name string) (*ContainerInfo, error) {
	return m.client.ContainerStatus(ctx, name)
}

// List returns all managed containers.
func (m *Misbah) List(ctx context.Context) ([]*ContainerInfo, error) {
	resp, err := m.client.ContainerList(ctx)
	if err != nil {
		return nil, fmt.Errorf("misbah: list: %w", err)
	}
	return resp.Containers, nil
}

// ContainerEvent re-exports the daemon event type.
type ContainerEvent = daemon.ContainerEvent

// Events subscribes to lifecycle events for the given container.
// Pass empty name for all containers. Close ctx to stop.
func (m *Misbah) Events(ctx context.Context, name string) (<-chan ContainerEvent, error) {
	return m.client.ContainerEvents(ctx, name)
}

// DiffEntry re-exports the overlay diff entry type.
type DiffEntry = daemon.DiffEntry

// Diff returns files changed by the agent in the container's overlay.
func (m *Misbah) Diff(ctx context.Context, name string) ([]DiffEntry, error) {
	resp, err := m.client.ContainerDiff(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("misbah: diff: %w", err)
	}
	return resp.Entries, nil
}

// Commit promotes selected files from overlay to real workspace.
func (m *Misbah) Commit(ctx context.Context, name string, paths []string) error {
	if err := m.client.ContainerCommit(ctx, name, paths); err != nil {
		return fmt.Errorf("misbah: commit: %w", err)
	}
	return nil
}

// Logs returns captured stdout/stderr for a container.
func (m *Misbah) Logs(ctx context.Context, name string) (stdout, stderr string, err error) {
	resp, err := m.client.ContainerLogs(ctx, name)
	if err != nil {
		return "", "", fmt.Errorf("misbah: logs: %w", err)
	}
	return resp.Stdout, resp.Stderr, nil
}

// LoadWhitelist pre-loads permission whitelist rules from a container spec.
func (m *Misbah) LoadWhitelist(ctx context.Context, spec *ContainerSpec) error {
	return m.client.WhitelistLoad(ctx, spec)
}

// NewSpec creates a ContainerSpec with sensible defaults for the namespace backend.
func NewSpec(name string, cmd []string) *ContainerSpec {
	return &ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: name,
		},
		Process: ProcessSpec{
			Command: cmd,
			Cwd:     "/workspace",
		},
		Namespaces: NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Runtime: "namespace",
	}
}

// NewSpecWithTier creates a ContainerSpec with tier-based mount isolation.
func NewSpecWithTier(name string, cmd []string, tier string, repos []string, writablePaths []string) *ContainerSpec {
	spec := NewSpec(name, cmd)
	spec.TierConfig = &TierConfig{
		Tier:          tier,
		WritablePaths: writablePaths,
	}
	for _, repo := range repos {
		spec.Mounts = append(spec.Mounts, MountSpec{
			Type:        "bind",
			Source:      repo,
			Destination: "/workspace",
			Options:     []string{"rbind", "ro"},
		})
	}
	return spec
}
