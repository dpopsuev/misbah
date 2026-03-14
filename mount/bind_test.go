package mount

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jabal/jabal/metrics"
	"github.com/jabal/jabal/model"
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

func TestBindMounterValidateSources(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	bm := NewBindMounter(logger)

	// Create test directories
	source1 := filepath.Join(tmpDir, "source1")
	source2 := filepath.Join(tmpDir, "source2")
	require.NoError(t, os.MkdirAll(source1, 0755))
	require.NoError(t, os.MkdirAll(source2, 0755))

	t.Run("valid sources", func(t *testing.T) {
		sources := []model.Source{
			{Path: source1, Mount: "source1"},
			{Path: source2, Mount: "source2"},
		}

		err := bm.ValidateSources(sources)
		assert.NoError(t, err)
	})

	t.Run("non-existent source", func(t *testing.T) {
		sources := []model.Source{
			{Path: filepath.Join(tmpDir, "nonexistent"), Mount: "source1"},
		}

		err := bm.ValidateSources(sources)
		assert.Error(t, err)
	})

	t.Run("invalid mount name", func(t *testing.T) {
		sources := []model.Source{
			{Path: source1, Mount: "invalid mount"},
		}

		err := bm.ValidateSources(sources)
		assert.Error(t, err)
	})
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
