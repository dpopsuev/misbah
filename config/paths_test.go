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

	// Verify directory was created
	info, err := os.Stat(testDir)
	assert.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestEnsureWorkspaceDir(t *testing.T) {
	// Save original env and restore after test
	originalConfigDir := os.Getenv("JABAL_CONFIG_DIR")
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("JABAL_CONFIG_DIR", originalConfigDir)
		} else {
			os.Unsetenv("JABAL_CONFIG_DIR")
		}
	}()

	tmpDir := t.TempDir()
	os.Setenv("JABAL_CONFIG_DIR", tmpDir)

	err := EnsureWorkspaceDir("test-workspace")
	assert.NoError(t, err)

	// Verify directory was created
	workspaceDir := GetWorkspaceDir("test-workspace")
	info, err := os.Stat(workspaceDir)
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

	// Create a directory
	dirPath := filepath.Join(tmpDir, "dir")
	require.NoError(t, os.MkdirAll(dirPath, 0755))

	// Create a file
	filePath := filepath.Join(tmpDir, "file")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0644))

	assert.True(t, IsDirectory(dirPath))
	assert.False(t, IsDirectory(filePath))
	assert.False(t, IsDirectory(filepath.Join(tmpDir, "nonexistent")))
}

func TestListWorkspaces(t *testing.T) {
	// Save original env and restore after test
	originalConfigDir := os.Getenv("JABAL_CONFIG_DIR")
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("JABAL_CONFIG_DIR", originalConfigDir)
		} else {
			os.Unsetenv("JABAL_CONFIG_DIR")
		}
	}()

	tmpDir := t.TempDir()
	os.Setenv("JABAL_CONFIG_DIR", tmpDir)

	t.Run("empty workspaces directory", func(t *testing.T) {
		workspaces, err := ListWorkspaces()
		assert.NoError(t, err)
		assert.Empty(t, workspaces)
	})

	t.Run("workspaces with manifests", func(t *testing.T) {
		// Create workspace directories with manifests
		require.NoError(t, EnsureWorkspaceDir("workspace1"))
		require.NoError(t, EnsureWorkspaceDir("workspace2"))

		manifest1 := GetManifestPath("workspace1")
		manifest2 := GetManifestPath("workspace2")

		require.NoError(t, os.WriteFile(manifest1, []byte("name: workspace1\nsources: []"), 0644))
		require.NoError(t, os.WriteFile(manifest2, []byte("name: workspace2\nsources: []"), 0644))

		// Create a directory without manifest (should be ignored)
		require.NoError(t, EnsureWorkspaceDir("incomplete"))

		workspaces, err := ListWorkspaces()
		assert.NoError(t, err)
		assert.Len(t, workspaces, 2)
		assert.Contains(t, workspaces, "workspace1")
		assert.Contains(t, workspaces, "workspace2")
		assert.NotContains(t, workspaces, "incomplete")
	})
}

func TestWorkspaceExists(t *testing.T) {
	// Save original env and restore after test
	originalConfigDir := os.Getenv("JABAL_CONFIG_DIR")
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("JABAL_CONFIG_DIR", originalConfigDir)
		} else {
			os.Unsetenv("JABAL_CONFIG_DIR")
		}
	}()

	tmpDir := t.TempDir()
	os.Setenv("JABAL_CONFIG_DIR", tmpDir)

	// Create a workspace with manifest
	require.NoError(t, EnsureWorkspaceDir("existing"))
	manifestPath := GetManifestPath("existing")
	require.NoError(t, os.WriteFile(manifestPath, []byte("name: existing\nsources: []"), 0644))

	assert.True(t, WorkspaceExists("existing"))
	assert.False(t, WorkspaceExists("nonexistent"))
}
