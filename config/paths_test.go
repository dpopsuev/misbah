package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvePath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		path     string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "tilde expansion",
			path:     "~/test",
			wantPath: filepath.Join(homeDir, "test"),
			wantErr:  false,
		},
		{
			name:     "absolute path",
			path:     "/absolute/path",
			wantPath: "/absolute/path",
			wantErr:  false,
		},
		{
			name:    "environment variable",
			path:    "$HOME/test",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolvePath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.wantPath != "" {
					assert.Equal(t, tt.wantPath, result)
				}
			}
		})
	}
}

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test", "nested", "dir")

	err := EnsureDir(testDir)
	assert.NoError(t, err)

	info, err := os.Stat(testDir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestPathExists(t *testing.T) {
	tmpDir := t.TempDir()
	existingPath := filepath.Join(tmpDir, "existing")
	require.NoError(t, os.MkdirAll(existingPath, 0755))

	assert.True(t, PathExists(existingPath))
	assert.False(t, PathExists(filepath.Join(tmpDir, "nonexistent")))
}

func TestIsDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	dirPath := filepath.Join(tmpDir, "dir")
	require.NoError(t, os.MkdirAll(dirPath, 0755))

	filePath := filepath.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0644))

	assert.True(t, IsDirectory(dirPath))
	assert.False(t, IsDirectory(filePath))
	assert.False(t, IsDirectory(filepath.Join(tmpDir, "nonexistent")))
}
