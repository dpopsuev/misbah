package model

// Provider represents a CLI AI agent provider (Claude Code, Aider, Cursor, etc).
type Provider interface {
	// Name returns the provider name.
	Name() string

	// GenerateConfig generates provider-specific configuration files.
	// configDir is the directory where config files should be written.
	// config is the provider-specific configuration from the manifest.
	GenerateConfig(configDir string, config map[string]interface{}) error

	// ValidateConfig validates provider-specific configuration.
	ValidateConfig(config map[string]interface{}) error

	// BinaryName returns the name of the provider's binary (e.g., "claude", "aider").
	BinaryName() string

	// ConfigDir returns the name of the config directory (e.g., ".claude", ".aider").
	ConfigDir() string
}

// ProviderInfo contains metadata about a provider.
type ProviderInfo struct {
	// Name is the provider name.
	Name string

	// Description is a human-readable description.
	Description string

	// BinaryName is the name of the provider's binary.
	BinaryName string

	// ConfigDir is the name of the config directory.
	ConfigDir string

	// Supported indicates if the provider is currently supported.
	Supported bool
}
