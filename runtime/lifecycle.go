package runtime

import (
	"fmt"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/dpopsuev/misbah/tier"
	"github.com/dpopsuev/misbah/validate"
)

// Lifecycle manages the complete container/workspace lifecycle.
type Lifecycle struct {
	lockManager      *LockManager
	namespaceManager *NamespaceManager
	bindMounter      *BindMounter
	backend          ContainerBackend
	logger           *metrics.Logger
	recorder         *metrics.MetricsRecorder
}

// NewLifecycle creates a new lifecycle manager.
// If backend is nil, the default NamespaceBackend is used.
func NewLifecycle(logger *metrics.Logger, recorder *metrics.MetricsRecorder, backend ...ContainerBackend) *Lifecycle {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	if recorder == nil {
		recorder = metrics.GetDefaultRecorder()
	}

	var b ContainerBackend
	if len(backend) > 0 && backend[0] != nil {
		b = backend[0]
	} else {
		b = NewNamespaceBackend(logger)
	}

	return &Lifecycle{
		lockManager:      NewLockManager(logger),
		namespaceManager: NewNamespaceManager(logger),
		bindMounter:      NewBindMounter(logger),
		backend:          b,
		logger:           logger,
		recorder:         recorder,
	}
}

// Start starts a container using ContainerSpec via the configured backend.
func (lc *Lifecycle) Start(spec *model.ContainerSpec) error {
	timer := metrics.NewTimer("container.start.total", map[string]string{
		"container": spec.Metadata.Name,
		"runtime":   spec.Runtime,
	}, lc.recorder, lc.logger)
	defer timer.Stop()

	lc.logger.Infof("Starting container %s (runtime=%s)", spec.Metadata.Name, spec.Runtime)

	// 1. Validate container spec
	lc.logger.Debugf("Validating container spec for %s", spec.Metadata.Name)
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("container spec validation failed: %w", err)
	}

	// 2. Acquire lock
	lc.logger.Debugf("Acquiring lock for container %s", spec.Metadata.Name)
	provider := "container"
	if label, ok := spec.Metadata.Labels["provider"]; ok {
		provider = label
	}

	_, err := lc.lockManager.AcquireLock(spec.Metadata.Name, provider)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		if err != nil {
			lc.logger.Debugf("Releasing lock due to error")
			_ = lc.lockManager.ReleaseLock(spec.Metadata.Name)
		}
	}()

	// 3. Resolve tier mounts (if configured)
	if spec.TierConfig != nil {
		lc.logger.Infof("Resolving tier mounts: tier=%s", spec.TierConfig.Tier)
		tierSpec := &tier.TierSpec{
			Tier:          tier.Tier(spec.TierConfig.Tier),
			WritablePaths: spec.TierConfig.WritablePaths,
		}
		// Extract repo paths from existing bind mounts
		for _, m := range spec.Mounts {
			if m.Type == "bind" && m.Source != "" {
				tierSpec.Repos = append(tierSpec.Repos, m.Source)
			}
		}
		if len(tierSpec.Repos) > 0 {
			tierMounts, tierErr := tier.GenerateTierMounts(tierSpec)
			if tierErr != nil {
				err = fmt.Errorf("tier mount generation failed: %w", tierErr)
				return err
			}
			spec.Mounts = tierMounts
			lc.logger.Infof("Tier mounts generated: %d mounts", len(tierMounts))
		}
	}

	// 4. Delegate to backend
	lc.logger.Infof("Creating container via backend: %v", spec.Process.Command)

	if _, err = lc.backend.Start(spec); err != nil {
		return fmt.Errorf("container creation failed: %w", err)
	}

	// 4. Cleanup after process exits
	lc.logger.Infof("Process exited, cleaning up")

	if err := lc.lockManager.ReleaseLock(spec.Metadata.Name); err != nil {
		lc.logger.Warnf("Failed to release lock: %v", err)
	}

	lc.logger.Infof("Container %s stopped successfully", spec.Metadata.Name)
	return nil
}

// Stop stops a running container by name.
func (lc *Lifecycle) Stop(name string, force bool) error {
	lc.logger.Infof("Stopping container %s (force=%v)", name, force)

	lock, err := lc.lockManager.GetLock(name)
	if err != nil {
		lc.logger.Warnf("No lock found for container %s, attempting cleanup anyway", name)
	} else {
		lc.logger.Infof("Found lock for container %s (PID %d, provider %s)", name, lock.PID, lock.Provider)

		if force {
			if err := lc.lockManager.ForceRelease(name); err != nil {
				return fmt.Errorf("failed to force release lock: %w", err)
			}
		} else {
			if err := lc.lockManager.ReleaseLock(name); err != nil {
				return fmt.Errorf("failed to release lock: %w (use --force to terminate the process)", err)
			}
		}
	}

	if err := lc.backend.Stop(name, force); err != nil {
		lc.logger.Warnf("Backend stop: %v", err)
	}

	lc.logger.Infof("Container %s stopped successfully", name)
	return nil
}

// Destroy destroys a container and cleans up all resources.
func (lc *Lifecycle) Destroy(name string) error {
	lc.logger.Infof("Destroying container %s", name)

	if err := lc.backend.Destroy(name); err != nil {
		lc.logger.Warnf("Backend destroy: %v", err)
	}

	lc.logger.Infof("Container %s destroyed", name)
	return nil
}

// CreateContainer is an alias for Start (backward compatibility).
func (lc *Lifecycle) CreateContainer(spec *model.ContainerSpec) error {
	return lc.Start(spec)
}

// Mount mounts a workspace and launches the provider (legacy workspace path).
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

	if err := lc.lockManager.ReleaseLock(workspace.Name); err != nil {
		lc.logger.Warnf("Failed to release lock: %v", err)
	}

	if err := lc.bindMounter.Cleanup(mountPath); err != nil {
		lc.logger.Warnf("Failed to cleanup mount point: %v", err)
	}

	lc.logger.Infof("Workspace %s unmounted successfully", workspace.Name)
	return nil
}

// Unmount unmounts a workspace by releasing its lock and cleaning up.
func (lc *Lifecycle) Unmount(workspace string, force bool) error {
	lc.logger.Infof("Unmounting workspace %s (force=%v)", workspace, force)

	lock, err := lc.lockManager.GetLock(workspace)
	if err != nil {
		lc.logger.Warnf("No lock found for workspace %s, attempting cleanup anyway", workspace)
	} else {
		lc.logger.Infof("Found lock for workspace %s (PID %d, provider %s)", workspace, lock.PID, lock.Provider)

		if force {
			if err := lc.lockManager.ForceRelease(workspace); err != nil {
				return fmt.Errorf("failed to force release lock: %w", err)
			}
		} else {
			if err := lc.lockManager.ReleaseLock(workspace); err != nil {
				return fmt.Errorf("failed to release lock: %w (use --force to terminate the process)", err)
			}
		}
	}

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
