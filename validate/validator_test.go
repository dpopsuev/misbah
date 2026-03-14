package validate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jabal/jabal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidator(t *testing.T) {
	validator := NewValidator()
	assert.NotNil(t, validator)
	assert.Len(t, validator.rules, 8) // 8 default rules
}

func TestValidatorValidate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test directories
	source1 := filepath.Join(tmpDir, "source1")
	source2 := filepath.Join(tmpDir, "source2")
	require.NoError(t, os.MkdirAll(source1, 0755))
	require.NoError(t, os.MkdirAll(source2, 0755))

	tests := []struct {
		name     string
		manifest *model.Manifest
		wantErr  bool
	}{
		{
			name: "valid manifest",
			manifest: &model.Manifest{
				Name:        "test-workspace",
				Description: "Test",
				Sources: []model.SourceSpec{
					{Path: source1, Mount: "source1"},
					{Path: source2, Mount: "source2"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: &model.Manifest{
				Sources: []model.SourceSpec{
					{Path: source1, Mount: "source1"},
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
		{
			name: "invalid workspace name",
			manifest: &model.Manifest{
				Name: "invalid workspace",
				Sources: []model.SourceSpec{
					{Path: source1, Mount: "source1"},
				},
			},
			wantErr: true,
		},
		{
			name: "non-existent source path",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: filepath.Join(tmpDir, "nonexistent"), Mount: "source1"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid mount name",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: source1, Mount: "invalid mount"},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate mount names",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: source1, Mount: "duplicate"},
					{Path: source2, Mount: "duplicate"},
				},
			},
			wantErr: true,
		},
		{
			name: "wildcard in path",
			manifest: &model.Manifest{
				Name: "test",
				Sources: []model.SourceSpec{
					{Path: filepath.Join(tmpDir, "source*"), Mount: "source1"},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := NewValidator()
			err := validator.Validate(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatorValidateFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test source
	source1 := filepath.Join(tmpDir, "source1")
	require.NoError(t, os.MkdirAll(source1, 0755))

	t.Run("valid manifest file", func(t *testing.T) {
		manifestPath := filepath.Join(tmpDir, "valid.yaml")
		manifestContent := `name: test-workspace
description: Test workspace
sources:
  - path: ` + source1 + `
    mount: source1
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

		validator := NewValidator()
		err := validator.ValidateFile(manifestPath)
		assert.NoError(t, err)
	})

	t.Run("invalid YAML file", func(t *testing.T) {
		manifestPath := filepath.Join(tmpDir, "invalid.yaml")
		invalidYAML := `name: test
sources:
  - path: ` + source1 + `
    mount: test
  invalid yaml
`
		require.NoError(t, os.WriteFile(manifestPath, []byte(invalidYAML), 0644))

		validator := NewValidator()
		err := validator.ValidateFile(manifestPath)
		assert.Error(t, err)
	})

	t.Run("non-existent file", func(t *testing.T) {
		validator := NewValidator()
		err := validator.ValidateFile("/nonexistent/manifest.yaml")
		assert.Error(t, err)
	})
}

func TestValidateYAML(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "valid YAML",
			yaml: `name: test
description: Test workspace
sources:
  - path: /tmp/source
    mount: source
`,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			yaml: `name: test
sources:
  - path: /tmp
    mount: test
  invalid yaml
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateYAML([]byte(tt.yaml))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateManifest(t *testing.T) {
	tmpDir := t.TempDir()
	source1 := filepath.Join(tmpDir, "source1")
	require.NoError(t, os.MkdirAll(source1, 0755))

	manifest := &model.Manifest{
		Name: "test",
		Sources: []model.SourceSpec{
			{Path: source1, Mount: "source1"},
		},
	}

	err := ValidateManifest(manifest)
	assert.NoError(t, err)
}

func TestValidateManifestFile(t *testing.T) {
	tmpDir := t.TempDir()
	source1 := filepath.Join(tmpDir, "source1")
	require.NoError(t, os.MkdirAll(source1, 0755))

	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	manifestContent := `name: test-workspace
sources:
  - path: ` + source1 + `
    mount: source1
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

	err := ValidateManifestFile(manifestPath)
	assert.NoError(t, err)
}
