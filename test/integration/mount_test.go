//go:build integration && linux

package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/dpopsuev/misbah/mount"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamespaceCreation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Namespace tests require Linux")
	}

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := mount.NewNamespaceManager(logger)

	// Test that namespace support check works
	err := nm.CheckNamespaceSupport()
	if err != nil {
		t.Skipf("Namespace support not available: %v", err)
	}
}

func TestLockManagerIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	lm := mount.NewLockManager(logger)
	lm.SetLocksDir(tmpDir) // Use custom locks dir for testing

	workspace := "integration-test"

	t.Run("acquire_and_release", func(t *testing.T) {
		lock, err := lm.AcquireLock(workspace, "claude")
		require.NoError(t, err)
		assert.NotNil(t, lock)
		assert.Equal(t, os.Getpid(), lock.PID)

		// Verify locked
		assert.True(t, lm.IsLocked(workspace))

		// Release
		err = lm.ReleaseLock(workspace)
		require.NoError(t, err)

		// Verify unlocked
		assert.False(t, lm.IsLocked(workspace))
	})

	t.Run("concurrent_lock_prevention", func(t *testing.T) {
		// Acquire lock
		lock1, err := lm.AcquireLock(workspace, "claude")
		require.NoError(t, err)
		assert.NotNil(t, lock1)

		// Try to acquire again (should fail)
		lock2, err := lm.AcquireLock(workspace, "aider")
		assert.Error(t, err)
		assert.Nil(t, lock2)

		// Cleanup
		_ = lm.ReleaseLock(workspace)
	})
}

func TestBindMounterIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	bm := mount.NewBindMounter(logger)

	// Create test sources
	sourceA := filepath.Join(tmpDir, "source-a")
	sourceB := filepath.Join(tmpDir, "source-b")

	require.NoError(t, os.MkdirAll(sourceA, 0755))
	require.NoError(t, os.MkdirAll(sourceB, 0755))

	sources := []model.Source{
		{Path: sourceA, Mount: "source-a"},
		{Path: sourceB, Mount: "source-b"},
	}

	t.Run("validate_sources", func(t *testing.T) {
		err := bm.ValidateSources(sources)
		assert.NoError(t, err)
	})

	t.Run("prepare_mount_point", func(t *testing.T) {
		mountPath := filepath.Join(tmpDir, "mount")
		err := bm.PrepareMountPoint(mountPath)
		assert.NoError(t, err)

		// Verify created
		info, err := os.Stat(mountPath)
		assert.NoError(t, err)
		assert.True(t, info.IsDir())
	})

	t.Run("cleanup", func(t *testing.T) {
		mountPath := filepath.Join(tmpDir, "cleanup-test")
		require.NoError(t, os.MkdirAll(mountPath, 0755))

		err := bm.Cleanup(mountPath)
		assert.NoError(t, err)

		// Verify removed
		_, err = os.Stat(mountPath)
		assert.True(t, os.IsNotExist(err))
	})
}

