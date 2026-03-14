package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateMountName(t *testing.T) {
	tests := []struct {
		name      string
		mountName string
		wantErr   bool
	}{
		{"valid alphanumeric", "repo1", false},
		{"valid with dash", "my-repo", false},
		{"valid with underscore", "my_repo", false},
		{"valid mixed", "my-repo_123", false},
		{"empty name", "", true},
		{"starts with dash", "-repo", true},
		{"starts with underscore", "_repo", true},
		{"contains space", "my repo", true},
		{"contains special chars", "my@repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMountName(tt.mountName)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSourceValidate(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	tests := []struct {
		name    string
		source  Source
		wantErr bool
	}{
		{
			name: "valid source",
			source: Source{
				Path:  tmpDir,
				Mount: "test-mount",
			},
			wantErr: false,
		},
		{
			name: "invalid mount name",
			source: Source{
				Path:  tmpDir,
				Mount: "invalid mount",
			},
			wantErr: true,
		},
		{
			name: "non-existent path",
			source: Source{
				Path:  filepath.Join(tmpDir, "nonexistent"),
				Mount: "test",
			},
			wantErr: true,
		},
		{
			name: "relative path",
			source: Source{
				Path:  "./relative",
				Mount: "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.source.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSourcesNonNested(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test directories
	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	dir1Sub := filepath.Join(tmpDir, "dir1", "sub")

	require.NoError(t, os.MkdirAll(dir1, 0755))
	require.NoError(t, os.MkdirAll(dir2, 0755))
	require.NoError(t, os.MkdirAll(dir1Sub, 0755))

	tests := []struct {
		name    string
		sources []Source
		wantErr bool
	}{
		{
			name: "non-nested sources",
			sources: []Source{
				{Path: dir1, Mount: "dir1"},
				{Path: dir2, Mount: "dir2"},
			},
			wantErr: false,
		},
		{
			name: "nested sources",
			sources: []Source{
				{Path: dir1, Mount: "dir1"},
				{Path: dir1Sub, Mount: "dir1sub"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSourcesNonNested(tt.sources)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSourcesUniqueMounts(t *testing.T) {
	tests := []struct {
		name    string
		sources []Source
		wantErr bool
	}{
		{
			name: "unique mounts",
			sources: []Source{
				{Path: "/path/a", Mount: "mount-a"},
				{Path: "/path/b", Mount: "mount-b"},
			},
			wantErr: false,
		},
		{
			name: "duplicate mounts",
			sources: []Source{
				{Path: "/path/a", Mount: "mount-a"},
				{Path: "/path/b", Mount: "mount-a"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSourcesUniqueMounts(tt.sources)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolvePath(tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPath, result)
			}
		})
	}
}
