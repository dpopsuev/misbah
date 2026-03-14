package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadManifest(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid manifest", func(t *testing.T) {
		manifestPath := filepath.Join(tmpDir, "valid.yaml")
		manifestContent := `name: test-workspace
description: Test workspace
tags:
  - test
  - demo
sources:
  - path: /tmp/source1
    mount: source1
  - path: /tmp/source2
    mount: source2
providers:
  claude:
    mcp_servers:
      scribe: http://localhost:8080
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

		manifest, err := LoadManifest(manifestPath)
		require.NoError(t, err)
		assert.Equal(t, "test-workspace", manifest.Name)
		assert.Equal(t, "Test workspace", manifest.Description)
		assert.Len(t, manifest.Sources, 2)
		assert.Len(t, manifest.Tags, 2)
		assert.Contains(t, manifest.Tags, "test")
		assert.Contains(t, manifest.Tags, "demo")
		assert.NotNil(t, manifest.Providers)
	})

	t.Run("invalid YAML", func(t *testing.T) {
		manifestPath := filepath.Join(tmpDir, "invalid.yaml")
		invalidYAML := `name: test
sources:
  - path: /tmp
    mount: test
  invalid yaml here
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(invalidYAML), 0644))

		_, err := LoadManifest(manifestPath)
		assert.Error(t, err)
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := LoadManifest("/nonexistent/manifest.yaml")
		assert.Error(t, err)
	})
}

func TestManifestValidate(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		wantErr  bool
	}{
		{
			name: "valid manifest",
			manifest: Manifest{
				Name: "valid-workspace",
				Sources: []SourceSpec{
					{Path: "/tmp/source", Mount: "source"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: Manifest{
				Sources: []SourceSpec{
					{Path: "/tmp/source", Mount: "source"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing sources",
			manifest: Manifest{
				Name:    "test",
				Sources: []SourceSpec{},
			},
			wantErr: true,
		},
		{
			name: "invalid workspace name",
			manifest: Manifest{
				Name: "invalid workspace name",
				Sources: []SourceSpec{
					{Path: "/tmp/source", Mount: "source"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestManifestToSources(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir, err := os.UserHomeDir()
	require.NoError(t, err)

	manifest := Manifest{
		Name: "test",
		Sources: []SourceSpec{
			{Path: tmpDir, Mount: "absolute"},
			{Path: "~/test", Mount: "tilde"},
		},
	}

	sources, err := manifest.ToSources()
	require.NoError(t, err)
	assert.Len(t, sources, 2)
	assert.Equal(t, tmpDir, sources[0].Path)
	assert.Equal(t, filepath.Join(homeDir, "test"), sources[1].Path)
}

func TestManifestGetProviderConfig(t *testing.T) {
	manifest := Manifest{
		Name: "test",
		Sources: []SourceSpec{
			{Path: "/tmp/source", Mount: "source"},
		},
		Providers: map[string]interface{}{
			"claude": map[string]interface{}{
				"mcp_servers": map[string]interface{}{
					"scribe": "http://localhost:8080",
				},
			},
		},
	}

	t.Run("existing provider", func(t *testing.T) {
		config, err := manifest.GetProviderConfig("claude")
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Contains(t, config, "mcp_servers")
	})

	t.Run("non-existent provider", func(t *testing.T) {
		_, err := manifest.GetProviderConfig("nonexistent")
		assert.Error(t, err)
	})
}

func TestManifestSaveManifest(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "test.yaml")

	manifest := Manifest{
		Name:        "test-workspace",
		Description: "Test",
		Sources: []SourceSpec{
			{Path: "/tmp/source", Mount: "source"},
		},
	}

	err := manifest.SaveManifest(manifestPath)
	require.NoError(t, err)

	// Verify file was created
	_, err = os.Stat(manifestPath)
	assert.NoError(t, err)

	// Verify we can load it back
	loaded, err := LoadManifest(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, manifest.Name, loaded.Name)
	assert.Equal(t, manifest.Description, loaded.Description)
	assert.Len(t, loaded.Sources, 1)
}

func TestValidateWorkspaceName(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		wantErr   bool
	}{
		{"valid name", "my-workspace", false},
		{"valid with numbers", "workspace-123", false},
		{"valid with underscore", "my_workspace", false},
		{"empty name", "", true},
		{"starts with dash", "-workspace", true},
		{"starts with underscore", "_workspace", true},
		{"contains space", "my workspace", true},
		{"contains special char", "my@workspace", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkspaceName(tt.workspace)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
