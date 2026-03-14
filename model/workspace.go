package model

import (
	"fmt"
	"path/filepath"
)

// Workspace represents a misbah workspace.
type Workspace struct {
	// Name is the workspace name.
	Name string

	// Description is an optional description.
	Description string

	// Sources are the source directories mounted in the workspace.
	Sources []Source

	// Providers contains provider-specific configuration.
	Providers map[string]interface{}

	// Tags are optional tags for categorization.
	Tags []string

	// ManifestPath is the path to the manifest file.
	ManifestPath string
}

// NewWorkspace creates a new workspace from a manifest.
func NewWorkspace(manifest *Manifest, manifestPath string) (*Workspace, error) {
	// Convert SourceSpecs to Sources
	sources, err := manifest.ToSources()
	if err != nil {
		return nil, fmt.Errorf("failed to convert sources: %w", err)
	}

	return &Workspace{
		Name:         manifest.Name,
		Description:  manifest.Description,
		Sources:      sources,
		Providers:    manifest.Providers,
		Tags:         manifest.Tags,
		ManifestPath: manifestPath,
	}, nil
}

// LoadWorkspace loads a workspace from a manifest file.
func LoadWorkspace(manifestPath string) (*Workspace, error) {
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	return NewWorkspace(manifest, manifestPath)
}

// Validate validates the workspace configuration.
func (w *Workspace) Validate() error {
	// Validate name
	if err := ValidateWorkspaceName(w.Name); err != nil {
		return err
	}

	// Validate each source
	for _, source := range w.Sources {
		if err := source.Validate(); err != nil {
			return fmt.Errorf("invalid source %s: %w", source.Mount, err)
		}
	}

	// Validate sources are not nested
	if err := ValidateSourcesNonNested(w.Sources); err != nil {
		return err
	}

	// Validate mount names are unique
	if err := ValidateSourcesUniqueMounts(w.Sources); err != nil {
		return err
	}

	return nil
}

// GetMountPath returns the mount path for the workspace.
func (w *Workspace) GetMountPath() string {
	return filepath.Join("/tmp/misbah", w.Name)
}

// GetLockPath returns the lock file path for the workspace.
func (w *Workspace) GetLockPath() string {
	return filepath.Join("/tmp/misbah/.locks", w.Name+".lock")
}

// GetProviderConfigDir returns the provider config directory for the workspace.
func (w *Workspace) GetProviderConfigDir(provider string) string {
	// Provider configs are stored in ~/.config/misbah/workspaces/{workspace}/.{provider}/
	baseDir := filepath.Dir(w.ManifestPath)
	return filepath.Join(baseDir, "."+provider)
}

// HasProvider checks if the workspace has configuration for a specific provider.
func (w *Workspace) HasProvider(provider string) bool {
	if w.Providers == nil {
		return false
	}
	_, ok := w.Providers[provider]
	return ok
}

// GetProvider returns the configuration for a specific provider.
func (w *Workspace) GetProvider(provider string) (map[string]interface{}, error) {
	if !w.HasProvider(provider) {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, provider)
	}

	config, ok := w.Providers[provider].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid provider config for %s", provider)
	}

	return config, nil
}

// String returns a human-readable representation of the workspace.
func (w *Workspace) String() string {
	return fmt.Sprintf("Workspace{name=%s, sources=%d, providers=%d}",
		w.Name, len(w.Sources), len(w.Providers))
}
