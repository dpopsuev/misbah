package model

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceToContainerSpec(t *testing.T) {
	workspace := &Workspace{
		Name:        "test-workspace",
		Description: "Test workspace",
		Sources: []Source{
			{
				Path:  "/home/user/repo-a",
				Mount: "repo-a",
			},
			{
				Path:  "/home/user/repo-b",
				Mount: "repo-b",
			},
		},
		Tags: []string{"dev", "test"},
	}

	command := []string{"/usr/bin/claude"}

	spec, err := WorkspaceToContainerSpec(workspace, command)
	require.NoError(t, err)
	require.NotNil(t, spec)

	// Validate conversion
	assert.Equal(t, "1.0", spec.Version)
	assert.Equal(t, "test-workspace", spec.Metadata.Name)
	assert.Equal(t, "Test workspace", spec.Metadata.Description)
	assert.Equal(t, command, spec.Process.Command)
	assert.Equal(t, filepath.Join("/tmp/misbah", "test-workspace"), spec.Process.Cwd)

	// Check namespaces
	assert.True(t, spec.Namespaces.User)
	assert.True(t, spec.Namespaces.Mount)
	assert.True(t, spec.Namespaces.PID)

	// Check mounts
	assert.Len(t, spec.Mounts, 2)
	assert.Equal(t, "bind", spec.Mounts[0].Type)
	assert.Equal(t, "/home/user/repo-a", spec.Mounts[0].Source)
	assert.Equal(t, filepath.Join("/tmp/misbah", "test-workspace", "repo-a"), spec.Mounts[0].Destination)

	// Check tags converted to labels
	assert.Contains(t, spec.Metadata.Labels, "tag0")
	assert.Contains(t, spec.Metadata.Labels, "tag1")

	// Validate the resulting spec
	err = spec.Validate()
	assert.NoError(t, err)
}

func TestWorkspaceToContainerSpec_MissingCommand(t *testing.T) {
	workspace := &Workspace{
		Name:    "test-workspace",
		Sources: []Source{},
	}

	spec, err := WorkspaceToContainerSpec(workspace, []string{})
	assert.Error(t, err)
	assert.Nil(t, spec)
	assert.Contains(t, err.Error(), "command is required")
}

func TestContainerSpecToWorkspace(t *testing.T) {
	spec := &ContainerSpec{
		Version: "1.0",
		Metadata: ContainerMetadata{
			Name:        "test-container",
			Description: "Test container",
			Labels: map[string]string{
				"tag0": "production",
				"tag1": "kubernetes",
			},
		},
		Process: ProcessSpec{
			Command: []string{"/usr/bin/claude"},
			Cwd:     "/tmp/misbah/test-container",
		},
		Namespaces: NamespaceSpec{
			User:  true,
			Mount: true,
			PID:   true,
		},
		Mounts: []MountSpec{
			{
				Type:        "bind",
				Source:      "/home/user/repo-a",
				Destination: "/tmp/misbah/test-container/repo-a",
				Options:     []string{"rw"},
			},
			{
				Type:        "bind",
				Source:      "/home/user/repo-b",
				Destination: "/tmp/misbah/test-container/repo-b",
				Options:     []string{"ro"},
			},
		},
	}

	workspace, err := ContainerSpecToWorkspace(spec)
	require.NoError(t, err)
	require.NotNil(t, workspace)

	// Validate conversion
	assert.Equal(t, "test-container", workspace.Name)
	assert.Equal(t, "Test container", workspace.Description)
	assert.Len(t, workspace.Sources, 2)

	// Check sources
	assert.Equal(t, "/home/user/repo-a", workspace.Sources[0].Path)
	assert.Equal(t, "repo-a", workspace.Sources[0].Mount)

	// Check tags extracted from labels
	assert.Contains(t, workspace.Tags, "production")
	assert.Contains(t, workspace.Tags, "kubernetes")
}

func TestManifestToContainerSpec(t *testing.T) {
	manifest := &Manifest{
		Name:        "test-manifest",
		Description: "Test manifest",
		Sources: []SourceSpec{
			{
				Path:  "/home/user/repo-a",
				Mount: "repo-a",
			},
		},
		Providers: map[string]interface{}{
			"claude": map[string]interface{}{
				"model": "claude-sonnet-4",
			},
		},
		Tags: []string{"test"},
	}

	command := []string{"/usr/bin/claude"}

	spec, err := ManifestToContainerSpec(manifest, command)
	require.NoError(t, err)
	require.NotNil(t, spec)

	assert.Equal(t, "1.0", spec.Version)
	assert.Equal(t, "test-manifest", spec.Metadata.Name)
	assert.Len(t, spec.Mounts, 1)

	// Validate the spec
	err = spec.Validate()
	assert.NoError(t, err)
}

func TestRoundTrip_WorkspaceToContainerSpecToWorkspace(t *testing.T) {
	// Start with a workspace
	originalWorkspace := &Workspace{
		Name:        "roundtrip-test",
		Description: "Roundtrip test workspace",
		Sources: []Source{
			{Path: "/home/user/repo-a", Mount: "repo-a"},
			{Path: "/home/user/repo-b", Mount: "repo-b"},
		},
		Tags: []string{"tag1", "tag2"},
	}

	// Convert to container spec
	spec, err := WorkspaceToContainerSpec(originalWorkspace, []string{"/bin/bash"})
	require.NoError(t, err)

	// Convert back to workspace
	convertedWorkspace, err := ContainerSpecToWorkspace(spec)
	require.NoError(t, err)

	// Compare (note: some fields may not round-trip perfectly due to lossy conversion)
	assert.Equal(t, originalWorkspace.Name, convertedWorkspace.Name)
	assert.Equal(t, originalWorkspace.Description, convertedWorkspace.Description)
	assert.Len(t, convertedWorkspace.Sources, len(originalWorkspace.Sources))

	// Sources should match
	for i := range originalWorkspace.Sources {
		assert.Equal(t, originalWorkspace.Sources[i].Path, convertedWorkspace.Sources[i].Path)
		assert.Equal(t, originalWorkspace.Sources[i].Mount, convertedWorkspace.Sources[i].Mount)
	}

	// Tags should be present (may be in different order)
	for _, tag := range originalWorkspace.Tags {
		assert.Contains(t, convertedWorkspace.Tags, tag)
	}
}
