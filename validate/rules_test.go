package validate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredFieldsRule(t *testing.T) {
	rule := &RequiredFieldsRule{}

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name: "valid manifest",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/source", Mount: "source"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: &model.Manifest{
				Sources: []model.SourceSpec{
					{Path: "/tmp/source", Mount: "source"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing sources",
			manifest: &model.Manifest{
				Name:    "test",
				Sources: []model.SourceSpec{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rule.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkspaceNameRule(t *testing.T) {
	rule := &WorkspaceNameRule{}

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name:     "valid name",
			manifest: &model.Manifest{Name: "my-workspace"},
			wantErr:  false,
		},
		{
			name:     "invalid name with space",
			manifest: &model.Manifest{Name: "my workspace"},
			wantErr:  true,
		},
		{
			name:     "invalid name starting with dash",
			manifest: &model.Manifest{Name: "-workspace"},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rule.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSourcePathsExistRule(t *testing.T) {
	tmpDir := t.TempDir()
	existingPath := filepath.Join(tmpDir, "existing")
	require.NoError(t, os.MkdirAll(existingPath, 0755))

	rule := &SourcePathsExistRule{}

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name: "existing path",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: existingPath, Mount: "source"},
				},
			},
			wantErr: false,
		},
		{
			name: "non-existent path",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: filepath.Join(tmpDir, "nonexistent"), Mount: "source"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rule.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSourcePathsAbsoluteRule(t *testing.T) {
	rule := &SourcePathsAbsoluteRule{}

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name: "absolute path",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/source", Mount: "source"},
				},
			},
			wantErr: false,
		},
		{
			name: "tilde path (resolves to absolute)",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "~/test", Mount: "source"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rule.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSourcePathsNotNestedRule(t *testing.T) {
	tmpDir := t.TempDir()

	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	dir1Sub := filepath.Join(tmpDir, "dir1", "sub")

	require.NoError(t, os.MkdirAll(dir1, 0755))
	require.NoError(t, os.MkdirAll(dir2, 0755))
	require.NoError(t, os.MkdirAll(dir1Sub, 0755))

	rule := &SourcePathsNotNestedRule{}

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name: "non-nested paths",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: dir1, Mount: "dir1"},
					{Path: dir2, Mount: "dir2"},
				},
			},
			wantErr: false,
		},
		{
			name: "nested paths",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: dir1, Mount: "dir1"},
					{Path: dir1Sub, Mount: "dir1sub"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rule.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMountNamesValidRule(t *testing.T) {
	rule := &MountNamesValidRule{}

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name: "valid mount names",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/a", Mount: "mount-a"},
					{Path: "/tmp/b", Mount: "mount_b"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid mount name with space",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/a", Mount: "invalid mount"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rule.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMountNamesUniqueRule(t *testing.T) {
	rule := &MountNamesUniqueRule{}

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name: "unique mount names",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/a", Mount: "mount-a"},
					{Path: "/tmp/b", Mount: "mount-b"},
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate mount names",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/a", Mount: "duplicate"},
					{Path: "/tmp/b", Mount: "duplicate"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rule.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNoWildcardsRule(t *testing.T) {
	rule := &NoWildcardsRule{}

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name: "no wildcards",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/source", Mount: "source"},
				},
			},
			wantErr: false,
		},
		{
			name: "asterisk wildcard",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/source*", Mount: "source"},
				},
			},
			wantErr: true,
		},
		{
			name: "question mark wildcard",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: "/tmp/source?", Mount: "source"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rule.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestContainsWildcard(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/tmp/source", false},
		{"/tmp/source*", true},
		{"/tmp/source?", true},
		{"/tmp/*/source", true},
		{"/tmp/?/source", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := containsWildcard(tt.path)
			assert.Equal(t, tt.want, result)
		})
	}
}
