package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jabal/jabal/config"
	"github.com/jabal/jabal/metrics"
	"github.com/jabal/jabal/model"
)

// LockManager manages workspace locks.
type LockManager struct {
	locksDir string
	logger   *metrics.Logger
}

// NewLockManager creates a new lock manager.
func NewLockManager(logger *metrics.Logger) *LockManager {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}

	return &LockManager{
		locksDir: config.GetLocksDir(),
		logger:   logger,
	}
}

// SetLocksDir sets a custom locks directory (for testing).
func (lm *LockManager) SetLocksDir(dir string) {
	lm.locksDir = dir
}

// AcquireLock acquires a lock for a workspace.
func (lm *LockManager) AcquireLock(workspace, provider string) (*model.Lock, error) {
	// Ensure locks directory exists
	if err := os.MkdirAll(lm.locksDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create locks directory: %w", err)
	}

	lockPath := lm.getLockPath(workspace)
	lm.logger.Debugf("Acquiring lock for workspace %s at %s", workspace, lockPath)

	// Check if lock file exists
	if existingLock, err := lm.readLock(lockPath); err == nil {
		// Lock exists, check if it's stale
		if existingLock.IsStale() {
			lm.logger.Warnf("Found stale lock for workspace %s (PID %d), removing", workspace, existingLock.PID)
			if err := os.Remove(lockPath); err != nil {
				return nil, fmt.Errorf("failed to remove stale lock: %w", err)
			}
		} else {
			// Lock is still active
			return nil, fmt.Errorf("%w: workspace %s is locked by PID %d (user: %s, started: %s)",
				model.ErrWorkspaceLocked, workspace, existingLock.PID, existingLock.User, existingLock.StartedAt)
		}
	}

	// Create new lock
	currentPID := os.Getpid()
	lock, err := model.NewLock(workspace, provider, currentPID)
	if err != nil {
		return nil, fmt.Errorf("failed to create lock: %w", err)
	}

	// Write lock file atomically (write to temp file, then rename)
	tempPath := lockPath + ".tmp"
	data, err := lock.Marshal()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lock: %w", err)
	}

	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write lock file: %w", err)
	}

	if err := os.Rename(tempPath, lockPath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		return nil, fmt.Errorf("failed to create lock file: %w", err)
	}

	lm.logger.Infof("Acquired lock for workspace %s (PID %d)", workspace, currentPID)
	return lock, nil
}

// ReleaseLock releases a lock for a workspace.
func (lm *LockManager) ReleaseLock(workspace string) error {
	lockPath := lm.getLockPath(workspace)
	lm.logger.Debugf("Releasing lock for workspace %s", workspace)

	// Check if lock file exists
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		lm.logger.Warnf("Lock file does not exist for workspace %s", workspace)
		return nil // Already released
	}

	// Read existing lock
	lock, err := lm.readLock(lockPath)
	if err != nil {
		// If we can't read it, try to remove it anyway
		lm.logger.Warnf("Failed to read lock file, removing anyway: %v", err)
		return os.Remove(lockPath)
	}

	// Verify this process owns the lock
	currentPID := os.Getpid()
	if lock.PID != currentPID {
		lm.logger.Warnf("Lock for workspace %s is owned by PID %d, not current PID %d", workspace, lock.PID, currentPID)
		// Still remove if it's stale
		if lock.IsStale() {
			lm.logger.Warnf("Lock is stale, removing")
			return os.Remove(lockPath)
		}
		return fmt.Errorf("cannot release lock owned by another process (PID %d)", lock.PID)
	}

	// Remove lock file
	if err := os.Remove(lockPath); err != nil {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}

	lm.logger.Infof("Released lock for workspace %s", workspace)
	return nil
}

// ForceRelease forcefully releases a lock by terminating the process.
func (lm *LockManager) ForceRelease(workspace string) error {
	lockPath := lm.getLockPath(workspace)
	lm.logger.Warnf("Force releasing lock for workspace %s", workspace)

	// Read existing lock
	lock, err := lm.readLock(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			lm.logger.Infof("No lock to force release for workspace %s", workspace)
			return nil
		}
		return fmt.Errorf("failed to read lock file: %w", err)
	}

	// Check if process is still running
	if lock.IsStale() {
		lm.logger.Infof("Lock is already stale, removing lock file")
		return os.Remove(lockPath)
	}

	// Send SIGTERM to the process
	lm.logger.Infof("Sending SIGTERM to PID %d", lock.PID)
	if err := syscall.Kill(lock.PID, syscall.SIGTERM); err != nil {
		lm.logger.Warnf("Failed to send SIGTERM to PID %d: %v", lock.PID, err)
	}

	// Wait up to 5 seconds for process to exit
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if lock.IsStale() {
			lm.logger.Infof("Process %d exited gracefully", lock.PID)
			return os.Remove(lockPath)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Process didn't exit, send SIGKILL
	lm.logger.Warnf("Process %d did not exit after SIGTERM, sending SIGKILL", lock.PID)
	if err := syscall.Kill(lock.PID, syscall.SIGKILL); err != nil {
		lm.logger.Errorf("Failed to send SIGKILL to PID %d: %v", lock.PID, err)
	}

	// Wait another second
	time.Sleep(1 * time.Second)

	// Remove lock file regardless
	lm.logger.Infof("Removing lock file for workspace %s", workspace)
	return os.Remove(lockPath)
}

// GetLock returns the current lock for a workspace, if any.
func (lm *LockManager) GetLock(workspace string) (*model.Lock, error) {
	lockPath := lm.getLockPath(workspace)
	return lm.readLock(lockPath)
}

// IsLocked checks if a workspace is currently locked.
func (lm *LockManager) IsLocked(workspace string) bool {
	lock, err := lm.GetLock(workspace)
	if err != nil {
		return false
	}
	return !lock.IsStale()
}

// getLockPath returns the path to a workspace's lock file.
func (lm *LockManager) getLockPath(workspace string) string {
	return filepath.Join(lm.locksDir, workspace+".lock")
}

// readLock reads a lock file.
func (lm *LockManager) readLock(lockPath string) (*model.Lock, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, err
	}

	return model.UnmarshalLock(data)
}
