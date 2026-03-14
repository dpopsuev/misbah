package mount

import (
	"fmt"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/dpopsuev/misbah/validate"
)

// Lifecycle manages the complete workspace mount/unmount lifecycle.
type Lifecycle struct {
	lockManager      *LockManager
	namespaceManager *NamespaceManager
	bindMounter      *BindMounter
	logger           *metrics.Logger
	recorder         *metrics.MetricsRecorder
}

// NewLifecycle creates a new lifecycle manager.
func NewLifecycle(logger *metrics.Logger, recorder *metrics.MetricsRecorder) *Lifecycle {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	if recorder == nil {
		recorder = metrics.GetDefaultRecorder()
	}

	return &Lifecycle{
		lockManager:      NewLockManager(logger),
		namespaceManager: NewNamespaceManager(logger),
		bindMounter:      NewBindMounter(logger),
		logger:           logger,
		recorder:         recorder,
	}
}

// Mount mounts a workspace and launches the provider.
func (lc *Lifecycle) Mount(workspace *model.Workspace, provider string, providerBinary string) error {
	timer := metrics.NewTimer("mount.total", map[string]string{
		"workspace": workspace.Name,
		"provider":  provider,
	}, lc.recorder, lc.logger)
	defer timer.Stop()

	lc.logger.Infof("Mounting workspace %s with provider %s", workspace.Name, provider)

	// 1. Validate workspace
	lc.logger.Debugf("Validating workspace %s", workspace.Name)
	if err := validate.ValidateManifest(&model.Manifest{
		Name:        workspace.Name,
		Description: workspace.Description,
		Sources:     workspaceSourcesToSourceSpecs(workspace.Sources),
		Providers:   workspace.Providers,
		Tags:        workspace.Tags,
	}); err != nil {
		return fmt.Errorf("workspace validation failed: %w", err)
	}

	// 2. Check namespace support
	if err := lc.namespaceManager.CheckNamespaceSupport(); err != nil {
		return fmt.Errorf("namespace support check failed: %w", err)
	}

	// 3. Acquire lock
	lc.logger.Debugf("Acquiring lock for workspace %s", workspace.Name)
	lock, err := lc.lockManager.AcquireLock(workspace.Name, provider)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		// Release lock on any error
		if err != nil {
			lc.logger.Debugf("Releasing lock due to error")
			_ = lc.lockManager.ReleaseLock(workspace.Name)
		}
	}()

	// 4. Validate sources
	lc.logger.Debugf("Validating sources for workspace %s", workspace.Name)
	if err := lc.bindMounter.ValidateSources(workspace.Sources); err != nil {
		return fmt.Errorf("source validation failed: %w", err)
	}

	// 5. Prepare mount point
	mountPath := config.GetMountPath(workspace.Name)
	lc.logger.Debugf("Preparing mount point: %s", mountPath)
	if err := lc.bindMounter.PrepareMountPoint(mountPath); err != nil {
		return fmt.Errorf("failed to prepare mount point: %w", err)
	}

	// 6. Create namespace and execute provider
	lc.logger.Infof("Creating namespace and launching provider: %s", providerBinary)

	env := []string{
		fmt.Sprintf("MISBAH_WORKSPACE=%s", workspace.Name),
		fmt.Sprintf("MISBAH_PROVIDER=%s", provider),
		fmt.Sprintf("MISBAH_LOCK_PID=%d", lock.PID),
	}

	if err := lc.namespaceManager.CreateNamespace(mountPath, workspace.Sources, providerBinary, env); err != nil {
		return fmt.Errorf("namespace creation failed: %w", err)
	}

	// 7. Cleanup after provider exits
	lc.logger.Infof("Provider exited, cleaning up")

	// Release lock
	if err := lc.lockManager.ReleaseLock(workspace.Name); err != nil {
		lc.logger.Warnf("Failed to release lock: %v", err)
	}

	// Cleanup mount point
	if err := lc.bindMounter.Cleanup(mountPath); err != nil {
		lc.logger.Warnf("Failed to cleanup mount point: %v", err)
	}

	lc.logger.Infof("Workspace %s unmounted successfully", workspace.Name)
	return nil
}

// Unmount unmounts a workspace by releasing its lock and cleaning up.
func (lc *Lifecycle) Unmount(workspace string, force bool) error {
	lc.logger.Infof("Unmounting workspace %s (force=%v)", workspace, force)

	// Check if workspace is locked
	lock, err := lc.lockManager.GetLock(workspace)
	if err != nil {
		lc.logger.Warnf("No lock found for workspace %s, attempting cleanup anyway", workspace)
	} else {
		lc.logger.Infof("Found lock for workspace %s (PID %d, provider %s)", workspace, lock.PID, lock.Provider)

		if force {
			// Force release (terminate process)
			if err := lc.lockManager.ForceRelease(workspace); err != nil {
				return fmt.Errorf("failed to force release lock: %w", err)
			}
		} else {
			// Try to release gracefully
			if err := lc.lockManager.ReleaseLock(workspace); err != nil {
				return fmt.Errorf("failed to release lock: %w (use --force to terminate the process)", err)
			}
		}
	}

	// Cleanup mount point
	mountPath := config.GetMountPath(workspace)
	if err := lc.bindMounter.Cleanup(mountPath); err != nil {
		lc.logger.Warnf("Failed to cleanup mount point: %v", err)
	}

	lc.logger.Infof("Workspace %s unmounted successfully", workspace)
	return nil
}

// GetStatus returns the current status of a workspace.
func (lc *Lifecycle) GetStatus(workspace string) (*WorkspaceStatus, error) {
	lock, err := lc.lockManager.GetLock(workspace)
	if err != nil {
		return &WorkspaceStatus{
			Workspace: workspace,
			Mounted:   false,
		}, nil
	}

	isStale := lock.IsStale()

	return &WorkspaceStatus{
		Workspace: workspace,
		Mounted:   !isStale,
		Provider:  lock.Provider,
		PID:       lock.PID,
		User:      lock.User,
		StartedAt: lock.StartedAt,
		Stale:     isStale,
	}, nil
}

// WorkspaceStatus represents the current status of a workspace.
type WorkspaceStatus struct {
	Workspace string
	Mounted   bool
	Provider  string
	PID       int
	User      string
	StartedAt interface{}
	Stale     bool
}

// workspaceSourcesToSourceSpecs converts workspace sources to manifest source specs.
func workspaceSourcesToSourceSpecs(sources []model.Source) []model.SourceSpec {
	specs := make([]model.SourceSpec, len(sources))
	for i, source := range sources {
		specs[i] = model.SourceSpec{
			Path:  source.Path,
			Mount: source.Mount,
		}
	}
	return specs
}
