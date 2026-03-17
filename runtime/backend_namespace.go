package runtime

import (
	"fmt"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
)

// NamespaceBackend wraps NamespaceManager to implement ContainerBackend.
// Used for Phase 1 namespace-based containers and sub-agent isolation inside Kata VMs.
type NamespaceBackend struct {
	nm     *NamespaceManager
	logger *metrics.Logger
}

// NewNamespaceBackend creates a new namespace-based container backend.
func NewNamespaceBackend(logger *metrics.Logger) *NamespaceBackend {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &NamespaceBackend{
		nm:     NewNamespaceManager(logger),
		logger: logger,
	}
}

func (nb *NamespaceBackend) Start(spec *model.ContainerSpec) (string, error) {
	cgroupMgr := NewCgroupManager(spec.Metadata.Name)
	if err := nb.nm.CreateContainer(spec, cgroupMgr); err != nil {
		return "", err
	}
	return spec.Metadata.Name, nil
}

func (nb *NamespaceBackend) Stop(name string, force bool) error {
	nb.logger.Infof("Namespace backend: stop is a no-op (process already exited)")
	return nil
}

func (nb *NamespaceBackend) Destroy(name string) error {
	cgroupMgr := NewCgroupManager(name)
	return cgroupMgr.Cleanup()
}

func (nb *NamespaceBackend) Exec(name string, cmd []string, timeout int64) ([]byte, []byte, int32, error) {
	return nil, nil, -1, fmt.Errorf("%w: exec not available for namespace backend", ErrNotSupported)
}

func (nb *NamespaceBackend) Status(name string) (*ContainerInfo, error) {
	return nil, fmt.Errorf("%w: status not available for namespace backend", ErrNotSupported)
}

func (nb *NamespaceBackend) List() ([]*ContainerInfo, error) {
	return nil, fmt.Errorf("%w: list not available for namespace backend", ErrNotSupported)
}

func (nb *NamespaceBackend) Close() error {
	return nil
}
