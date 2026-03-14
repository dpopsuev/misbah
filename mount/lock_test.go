package mount

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLockManager(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	lm := NewLockManager(logger)
	assert.NotNil(t, lm)
	assert.NotNil(t, lm.logger)
}

func TestLockManagerAcquireLock(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	lm := NewLockManager(logger)
	lm.locksDir = tmpDir

	t.Run("acquire new lock", func(t *testing.T) {
		lock, err := lm.AcquireLock("test-workspace", "claude")
		require.NoError(t, err)
		assert.NotNil(t, lock)
		assert.Equal(t, "test-workspace", lock.Workspace)
		assert.Equal(t, "claude", lock.Provider)
		assert.Equal(t, os.Getpid(), lock.PID)

		// Verify lock file was created
		lockPath := filepath.Join(tmpDir, "test-workspace.lock")
		_, err = os.Stat(lockPath)
		assert.NoError(t, err)

		// Cleanup
		_ = os.Remove(lockPath)
	})

	t.Run("acquire lock when already locked", func(t *testing.T) {
		workspace := "locked-workspace"

		// Acquire first lock
		lock1, err := lm.AcquireLock(workspace, "claude")
		require.NoError(t, err)
		assert.NotNil(t, lock1)

		// Try to acquire again (should fail)
		lock2, err := lm.AcquireLock(workspace, "aider")
		assert.Error(t, err)
		assert.Nil(t, lock2)
		assert.ErrorIs(t, err, model.ErrWorkspaceLocked)

		// Cleanup
		lockPath := filepath.Join(tmpDir, workspace+".lock")
		_ = os.Remove(lockPath)
	})

	t.Run("acquire lock after stale lock", func(t *testing.T) {
		workspace := "stale-workspace"
		lockPath := filepath.Join(tmpDir, workspace+".lock")

		// Create a stale lock (with non-existent PID)
		staleLock, err := model.NewLock(workspace, "claude", 999999)
		require.NoError(t, err)

		data, err := staleLock.Marshal()
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(lockPath, data, 0644))

		// Try to acquire lock (should succeed by removing stale lock)
		lock, err := lm.AcquireLock(workspace, "aider")
		require.NoError(t, err)
		assert.NotNil(t, lock)
		assert.Equal(t, os.Getpid(), lock.PID)

		// Cleanup
		_ = os.Remove(lockPath)
	})
}

func TestLockManagerReleaseLock(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	lm := NewLockManager(logger)
	lm.locksDir = tmpDir

	t.Run("release existing lock", func(t *testing.T) {
		workspace := "test-workspace"

		// Acquire lock
		lock, err := lm.AcquireLock(workspace, "claude")
		require.NoError(t, err)
		assert.NotNil(t, lock)

		// Release lock
		err = lm.ReleaseLock(workspace)
		assert.NoError(t, err)

		// Verify lock file was removed
		lockPath := filepath.Join(tmpDir, workspace+".lock")
		_, err = os.Stat(lockPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("release non-existent lock", func(t *testing.T) {
		// Should not error
		err := lm.ReleaseLock("nonexistent-workspace")
		assert.NoError(t, err)
	})
}

func TestLockManagerForceRelease(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	lm := NewLockManager(logger)
	lm.locksDir = tmpDir

	t.Run("force release stale lock", func(t *testing.T) {
		workspace := "stale-workspace"
		lockPath := filepath.Join(tmpDir, workspace+".lock")

		// Create a stale lock
		staleLock, err := model.NewLock(workspace, "claude", 999999)
		require.NoError(t, err)

		data, err := staleLock.Marshal()
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(lockPath, data, 0644))

		// Force release
		err = lm.ForceRelease(workspace)
		assert.NoError(t, err)

		// Verify lock file was removed
		_, err = os.Stat(lockPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("force release non-existent lock", func(t *testing.T) {
		err := lm.ForceRelease("nonexistent-workspace")
		assert.NoError(t, err)
	})
}

func TestLockManagerGetLock(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	lm := NewLockManager(logger)
	lm.locksDir = tmpDir

	t.Run("get existing lock", func(t *testing.T) {
		workspace := "test-workspace"

		// Acquire lock
		originalLock, err := lm.AcquireLock(workspace, "claude")
		require.NoError(t, err)

		// Get lock
		lock, err := lm.GetLock(workspace)
		require.NoError(t, err)
		assert.Equal(t, originalLock.Workspace, lock.Workspace)
		assert.Equal(t, originalLock.Provider, lock.Provider)
		assert.Equal(t, originalLock.PID, lock.PID)

		// Cleanup
		_ = lm.ReleaseLock(workspace)
	})

	t.Run("get non-existent lock", func(t *testing.T) {
		_, err := lm.GetLock("nonexistent-workspace")
		assert.Error(t, err)
	})
}

func TestLockManagerIsLocked(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	lm := NewLockManager(logger)
	lm.locksDir = tmpDir

	t.Run("workspace is locked", func(t *testing.T) {
		workspace := "test-workspace"

		// Acquire lock
		_, err := lm.AcquireLock(workspace, "claude")
		require.NoError(t, err)

		// Check if locked
		assert.True(t, lm.IsLocked(workspace))

		// Cleanup
		_ = lm.ReleaseLock(workspace)
	})

	t.Run("workspace is not locked", func(t *testing.T) {
		assert.False(t, lm.IsLocked("nonexistent-workspace"))
	})

	t.Run("workspace has stale lock", func(t *testing.T) {
		workspace := "stale-workspace"
		lockPath := filepath.Join(tmpDir, workspace+".lock")

		// Create a stale lock
		staleLock, err := model.NewLock(workspace, "claude", 999999)
		require.NoError(t, err)

		data, err := staleLock.Marshal()
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(lockPath, data, 0644))

		// Should not be considered locked
		assert.False(t, lm.IsLocked(workspace))

		// Cleanup
		_ = os.Remove(lockPath)
	})
}
