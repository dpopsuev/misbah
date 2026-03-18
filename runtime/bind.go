package runtime

import (
	"fmt"
	"os"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
)

// BindMounter handles bind mount operations.
type BindMounter struct {
	logger *metrics.Logger
}

// NewBindMounter creates a new bind mounter.
func NewBindMounter(logger *metrics.Logger) *BindMounter {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}

	return &BindMounter{
		logger: logger,
	}
}

// PrepareMountPoint prepares the mount point directory.
func (bm *BindMounter) PrepareMountPoint(mountPath string) error {
	bm.logger.Debugf("Preparing mount point: %s", mountPath)

	// Create mount point directory
	if err := os.MkdirAll(mountPath, 0755); err != nil {
		return fmt.Errorf("%w: failed to create mount point %s: %v", model.ErrMountFailed, mountPath, err)
	}

	return nil
}

// Cleanup removes the mount point directory and all contents.
func (bm *BindMounter) Cleanup(mountPath string) error {
	bm.logger.Debugf("Cleaning up mount point: %s", mountPath)

	// Check if mount point exists
	if _, err := os.Stat(mountPath); os.IsNotExist(err) {
		bm.logger.Debugf("Mount point %s does not exist, nothing to clean up", mountPath)
		return nil
	}

	// Note: We don't need to manually unmount because the namespace is destroyed
	// when the provider process exits. The bind mounts are automatically cleaned up.

	// Remove the mount point directory
	if err := os.RemoveAll(mountPath); err != nil {
		return fmt.Errorf("failed to remove mount point %s: %w", mountPath, err)
	}

	bm.logger.Infof("Cleaned up mount point: %s", mountPath)
	return nil
}
