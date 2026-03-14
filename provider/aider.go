package provider

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AiderProvider implements the Provider interface for Aider.
type AiderProvider struct{}

// Name returns the provider name.
func (a *AiderProvider) Name() string {
	return "aider"
}

// BinaryName returns the binary name.
func (a *AiderProvider) BinaryName() string {
	return "aider"
}

// ConfigDir returns the config directory name.
func (a *AiderProvider) ConfigDir() string {
	return ".aider"
}

// GenerateConfig generates Aider configuration files.
func (a *AiderProvider) GenerateConfig(configDir string, config map[string]interface{}) error {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Generate .aider.conf.yml
	configPath := filepath.Join(configDir, ".aider.conf.yml")

	// Build Aider configuration
	aiderConfig := make(map[string]interface{})

	// Copy all config keys directly
	// Aider uses YAML config with simple key-value pairs
	for key, value := range config {
		aiderConfig[key] = value
	}

	// If config is empty, create minimal config
	if len(aiderConfig) == 0 {
		aiderConfig = map[string]interface{}{
			"auto-commits": false,
		}
	}

	// Marshal to YAML
	data, err := yaml.Marshal(aiderConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal Aider config: %w", err)
	}

	// Write config file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write Aider config: %w", err)
	}

	return nil
}

// ValidateConfig validates Aider configuration.
func (a *AiderProvider) ValidateConfig(config map[string]interface{}) error {
	// Basic validation
	if config == nil {
		return nil // Empty config is valid
	}

	// Validate common Aider options
	if model, ok := config["model"]; ok {
		if _, ok := model.(string); !ok {
			return fmt.Errorf("model must be a string")
		}
	}

	if autoCommits, ok := config["auto-commits"]; ok {
		if _, ok := autoCommits.(bool); !ok {
			return fmt.Errorf("auto-commits must be a boolean")
		}
	}

	if editFormat, ok := config["edit-format"]; ok {
		if _, ok := editFormat.(string); !ok {
			return fmt.Errorf("edit-format must be a string")
		}
	}

	return nil
}
