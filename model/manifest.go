package model

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest represents a workspace manifest file.
type Manifest struct {
	// Name is the workspace name (required).
	Name string `yaml:"name"`

	// Description is an optional description of the workspace.
	Description string `yaml:"description,omitempty"`

	// Tags are optional tags for categorization.
	Tags []string `yaml:"tags,omitempty"`

	// Sources are the source directories to mount (required).
	Sources []SourceSpec `yaml:"sources"`

	// Providers contains provider-specific configuration.
	Providers map[string]interface{} `yaml:"providers,omitempty"`
}

// LoadManifest loads a manifest from a YAML file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}

	return &manifest, nil
}

// SaveManifest saves a manifest to a YAML file.
func (m *Manifest) SaveManifest(path string) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// Validate performs basic validation on the manifest.
func (m *Manifest) Validate() error {
	// Name is required
	if m.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidManifest)
	}

	// At least one source is required
	if len(m.Sources) == 0 {
		return fmt.Errorf("%w: at least one source is required", ErrInvalidManifest)
	}

	// Validate workspace name
	if err := ValidateWorkspaceName(m.Name); err != nil {
		return err
	}

	return nil
}

// ToSources converts the manifest's SourceSpecs to Sources with resolved paths.
func (m *Manifest) ToSources() ([]Source, error) {
	sources := make([]Source, len(m.Sources))

	for i, spec := range m.Sources {
		// Resolve path
		resolvedPath, err := ResolvePath(spec.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path %s: %w", spec.Path, err)
		}

		sources[i] = Source{
			Path:  resolvedPath,
			Mount: spec.Mount,
		}
	}

	return sources, nil
}

// GetProviderConfig returns the configuration for a specific provider.
func (m *Manifest) GetProviderConfig(provider string) (map[string]interface{}, error) {
	if m.Providers == nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, provider)
	}

	config, ok := m.Providers[provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, provider)
	}

	// Convert to map[string]interface{}
	configMap, ok := config.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid provider config for %s", provider)
	}

	return configMap, nil
}

// ValidateWorkspaceName validates a workspace name.
// Workspace names must be alphanumeric with optional dashes and underscores.
func ValidateWorkspaceName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: workspace name cannot be empty", ErrInvalidWorkspaceName)
	}

	// Check for valid characters (same rules as mount names)
	for _, char := range name {
		if !isAlphanumericOrDashOrUnderscore(char) {
			return fmt.Errorf("%w: %s (only alphanumeric, dash, and underscore allowed)", ErrInvalidWorkspaceName, name)
		}
	}

	// Cannot start with dash or underscore
	if name[0] == '-' || name[0] == '_' {
		return fmt.Errorf("%w: %s (cannot start with dash or underscore)", ErrInvalidWorkspaceName, name)
	}

	return nil
}
