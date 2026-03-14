package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// GlobalConfig represents the global misbah configuration.
type GlobalConfig struct {
	// ConfigDir is the configuration directory.
	ConfigDir string `yaml:"config_dir,omitempty"`

	// TempDir is the temporary directory for mounts.
	TempDir string `yaml:"temp_dir,omitempty"`

	// DefaultProvider is the default provider to use.
	DefaultProvider string `yaml:"default_provider,omitempty"`

	// LogLevel is the default log level.
	LogLevel string `yaml:"log_level,omitempty"`

	// AutoCleanup enables automatic cleanup of stale mounts.
	AutoCleanup bool `yaml:"auto_cleanup,omitempty"`
}

// LoadGlobalConfig loads the global configuration from ~/.config/misbah/config.yaml.
func LoadGlobalConfig() (*GlobalConfig, error) {
	configPath := GetGlobalConfigPath()

	// If config file doesn't exist, return default config
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultGlobalConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}

	var config GlobalConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	// Apply defaults for unset fields
	applyDefaults(&config)

	return &config, nil
}

// SaveGlobalConfig saves the global configuration to ~/.config/misbah/config.yaml.
func (c *GlobalConfig) SaveGlobalConfig() error {
	configPath := GetGlobalConfigPath()

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal global config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write global config: %w", err)
	}

	return nil
}

// GetGlobalConfigPath returns the path to the global config file.
func GetGlobalConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.yaml")
}

// DefaultGlobalConfig returns a GlobalConfig with default values.
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		ConfigDir:       GetConfigDir(),
		TempDir:         GetTempDir(),
		DefaultProvider: "claude",
		LogLevel:        "info",
		AutoCleanup:     true,
	}
}

// applyDefaults applies default values to unset config fields.
func applyDefaults(config *GlobalConfig) {
	defaults := DefaultGlobalConfig()

	if config.ConfigDir == "" {
		config.ConfigDir = defaults.ConfigDir
	}
	if config.TempDir == "" {
		config.TempDir = defaults.TempDir
	}
	if config.DefaultProvider == "" {
		config.DefaultProvider = defaults.DefaultProvider
	}
	if config.LogLevel == "" {
		config.LogLevel = defaults.LogLevel
	}
}
