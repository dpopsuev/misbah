package daemon

import (
	"github.com/dpopsuev/misbah/model"
)

// ContainerLifecycle is the interface for container lifecycle operations.
// Implemented by runtime.Lifecycle. Defined here to avoid import cycle
// (daemon → runtime → proxy → daemon).
type ContainerLifecycle interface {
	Start(spec *model.ContainerSpec) error
	Stop(name string, force bool) error
	Destroy(name string) error
	Exec(name string, cmd []string, timeout int64) (stdout, stderr []byte, exitCode int32, err error)
	Status(name string) (*model.ContainerInfo, error)
	List() ([]*model.ContainerInfo, error)
}


// ContainerStartRequest is sent by the CLI to start a Kata container via the daemon.
type ContainerStartRequest struct {
	Spec *model.ContainerSpec `json:"spec"`
}

// ContainerStartResponse is returned after the container starts.
type ContainerStartResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// ContainerStopRequest is sent to stop a running container.
type ContainerStopRequest struct {
	Name  string `json:"name"`
	Force bool   `json:"force"`
}

// ContainerDestroyRequest is sent to destroy a container.
type ContainerDestroyRequest struct {
	Name string `json:"name"`
}

// ContainerActionResponse is a generic response for stop/destroy actions.
type ContainerActionResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// ContainerStatusRequest is sent to get a container's status.
type ContainerStatusRequest struct {
	Name string `json:"name"`
}

// ContainerExecRequest is sent to execute a command in a running container.
type ContainerExecRequest struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
	Timeout int64    `json:"timeout"`
}

// ContainerExecResponse is returned with exec results.
type ContainerExecResponse struct {
	ExitCode int32  `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// ContainerListResponse contains the list of managed containers.
type ContainerListResponse struct {
	Containers []*model.ContainerInfo `json:"containers"`
}

// ContainerLogsResponse contains captured stdout/stderr for a container.
type ContainerLogsResponse struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

// ContainerDiffRequest requests the diff for a container's overlay.
type ContainerDiffRequest struct {
	Name string `json:"name"`
}

// ContainerDiffResponse contains the list of changed files.
type ContainerDiffResponse struct {
	Entries []DiffEntry `json:"entries"`
}

// ContainerCommitRequest promotes selected files from overlay to real workspace.
type ContainerCommitRequest struct {
	Name  string   `json:"name"`
	Paths []string `json:"paths"`
}
