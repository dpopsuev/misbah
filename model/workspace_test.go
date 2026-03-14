package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test directories
	source1 := filepath.Join(tmpDir, "source1")
	source2 := filepath.Join(tmpDir, "source2")
	require.NoError(t, os.MkdirAll(source1, 0755))
	require.NoError(t, os.MkdirAll(source2, 0755))

	manifest := &Manifest{
		Name:        "test-workspace",
		Description: "Test workspace",
		Tags:        []string{"test"},
		Sources: []SourceSpec{
			{Path: source1, Mount: "source1"},
			{Path: source2, Mount: "source2"},
		},
		Providers: map[string]interface{}{
			"claude": map[string]interface{}{},
		},
	}

	workspace, err := NewWorkspace(manifest, "/path/to/manifest.yaml")
	require.NoError(t, err)
	assert.Equal(t, "test-workspace", workspace.Name)
	assert.Equal(t, "Test workspace", workspace.Description)
	assert.Len(t, workspace.Sources, 2)
	assert.Len(t, workspace.Tags, 1)
	assert.Equal(t, "/path/to/manifest.yaml", workspace.ManifestPath)
}

func TestLoadWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test sources
	source1 := filepath.Join(tmpDir, "source1")
	require.NoError(t, os.MkdirAll(source1, 0755))

	// Create manifest
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	manifestContent := `name: test-workspace
description: Test workspace
sources:
  - path: ` + source1 + `
    mount: source1
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

	workspace, err := LoadWorkspace(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, "test-workspace", workspace.Name)
	assert.Len(t, workspace.Sources, 1)
}

func TestWorkspaceValidate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test directories
	source1 := filepath.Join(tmpDir, "source1")
	source2 := filepath.Join(tmpDir, "source2")
	require.NoError(t, os.MkdirAll(source1, 0755))
	require.NoError(t, os.MkdirAll(source2, 0755))

	t.Run("valid workspace", func(t *testing.T) {
		workspace := &Workspace{
			Name: "test-workspace",
			Sources: []Source{
				{Path: source1, Mount: "source1"},
				{Path: source2, Mount: "source2"},
			},
		}
		err := workspace.Validate()
		assert.NoError(t, err)
	})

	t.Run("invalid workspace name", func(t *testing.T) {
		workspace := &Workspace{
			Name: "invalid workspace",
			Sources: []Source{
				{Path: source1, Mount: "source1"},
			},
		}
		err := workspace.Validate()
		assert.Error(t, err)
	})

	t.Run("duplicate mount names", func(t *testing.T) {
		workspace := &Workspace{
			Name: "test",
			Sources: []Source{
				{Path: source1, Mount: "duplicate"},
				{Path: source2, Mount: "duplicate"},
			},
		}
		err := workspace.Validate()
		assert.Error(t, err)
	})
}

func TestWorkspaceGetMountPath(t *testing.T) {
	workspace := &Workspace{Name: "test-workspace"}
	mountPath := workspace.GetMountPath()
	assert.Equal(t, "/tmp/jabal/test-workspace", mountPath)
}

func TestWorkspaceGetLockPath(t *testing.T) {
	workspace := &Workspace{Name: "test-workspace"}
	lockPath := workspace.GetLockPath()
	assert.Equal(t, "/tmp/jabal/.locks/test-workspace.lock", lockPath)
}

func TestWorkspaceGetProviderConfigDir(t *testing.T) {
	workspace := &Workspace{
		Name:         "test-workspace",
		ManifestPath: "/home/user/.config/jabal/workspaces/test-workspace/manifest.yaml",
	}
	configDir := workspace.GetProviderConfigDir("claude")
	assert.Equal(t, "/home/user/.config/jabal/workspaces/test-workspace/.claude", configDir)
}

func TestWorkspaceHasProvider(t *testing.T) {
	workspace := &Workspace{
		Name: "test",
		Providers: map[string]interface{}{
			"claude": map[string]interface{}{},
		},
	}

	assert.True(t, workspace.HasProvider("claude"))
	assert.False(t, workspace.HasProvider("aider"))
}

func TestWorkspaceGetProvider(t *testing.T) {
	workspace := &Workspace{
		Name: "test",
		Providers: map[string]interface{}{
			"claude": map[string]interface{}{
				"setting": "value",
			},
		},
	}

	t.Run("existing provider", func(t *testing.T) {
		config, err := workspace.GetProvider("claude")
		require.NoError(t, err)
		assert.Contains(t, config, "setting")
	})

	t.Run("non-existent provider", func(t *testing.T) {
		_, err := workspace.GetProvider("nonexistent")
		assert.Error(t, err)
	})
}

func TestWorkspaceString(t *testing.T) {
	workspace := &Workspace{
		Name: "test-workspace",
		Sources: []Source{
			{Path: "/tmp/source1", Mount: "source1"},
			{Path: "/tmp/source2", Mount: "source2"},
		},
		Providers: map[string]interface{}{
			"claude": map[string]interface{}{},
		},
	}

	str := workspace.String()
	assert.Contains(t, str, "test-workspace")
	assert.Contains(t, str, "sources=2")
	assert.Contains(t, str, "providers=1")
}
