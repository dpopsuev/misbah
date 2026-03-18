package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBindMounter(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	bm := NewBindMounter(logger)
	assert.NotNil(t, bm)
	assert.NotNil(t, bm.logger)
}

func TestBindMounterPrepareMountPoint(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	bm := NewBindMounter(logger)

	mountPath := filepath.Join(tmpDir, "mount", "nested", "point")

	err := bm.PrepareMountPoint(mountPath)
	assert.NoError(t, err)

	// Verify directory was created
	info, err := os.Stat(mountPath)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestBindMounterCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	bm := NewBindMounter(logger)

	t.Run("cleanup existing mount point", func(t *testing.T) {
		mountPath := filepath.Join(tmpDir, "mount-point")

		// Create mount point with some files
		require.NoError(t, os.MkdirAll(mountPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(mountPath, "file.txt"), []byte("test"), 0644))

		// Cleanup
		err := bm.Cleanup(mountPath)
		assert.NoError(t, err)

		// Verify mount point was removed
		_, err = os.Stat(mountPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("cleanup non-existent mount point", func(t *testing.T) {
		// Should not error
		err := bm.Cleanup(filepath.Join(tmpDir, "nonexistent"))
		assert.NoError(t, err)
	})
}
