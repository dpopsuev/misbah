package runtime

import (
	"fmt"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/dpopsuev/misbah/tier"
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

	// 3. Resolve git-clone mounts
	gitMgr := NewGitCloneManager(lc.logger, config.GetTempDir())
	resolvedMounts, gitErr := gitMgr.ResolveGitCloneMounts(spec.Mounts)
	if gitErr != nil {
		err = fmt.Errorf("git-clone mount resolution failed: %w", gitErr)
		return err
	}
	spec.Mounts = resolvedMounts
	defer func() {
		if cleanErr := gitMgr.Cleanup(); cleanErr != nil {
			lc.logger.Warnf("Failed to clean up git clones: %v", cleanErr)
		}
	}()

	// 4. Resolve tier mounts (if configured)
	if spec.TierConfig != nil {
		lc.logger.Infof("Resolving tier mounts: tier=%s", spec.TierConfig.Tier)
		tierSpec := &tier.TierSpec{
			Tier:          tier.Tier(spec.TierConfig.Tier),
			WritablePaths: spec.TierConfig.WritablePaths,
		}
		// Extract repo paths from existing bind mounts
		for _, m := range spec.Mounts {
			if m.Type == model.MountTypeBind && m.Source != "" {
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

