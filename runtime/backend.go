package runtime

import (
	"errors"

	"github.com/dpopsuev/misbah/model"
)

// ErrNotSupported is returned when a backend does not support an operation.
var ErrNotSupported = errors.New("operation not supported by this backend")

// ContainerBackend defines the interface for container lifecycle operations.
// Both namespace-based and CRI-based backends implement this interface.
type ContainerBackend interface {
	Start(spec *model.ContainerSpec) (string, error)
	Stop(name string, force bool) error
	Destroy(name string) error
	Exec(name string, cmd []string, timeout int64) (stdout, stderr []byte, exitCode int32, err error)
	Status(name string) (*ContainerInfo, error)
	List() ([]*ContainerInfo, error)
	Close() error
}

// ContainerInfo holds runtime information about a container.
type ContainerInfo struct {
	ID        string
	Name      string
	State     string
	SandboxID string
	CreatedAt int64
	ExitCode  int32
}
