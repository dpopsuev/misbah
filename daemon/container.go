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
