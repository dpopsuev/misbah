package provider

import (
	"fmt"
	"os"
)

// CursorProvider implements the Provider interface for Cursor.
// Note: This is a placeholder implementation. Full Cursor support TBD.
type CursorProvider struct{}

// Name returns the provider name.
func (c *CursorProvider) Name() string {
	return "cursor"
}

// BinaryName returns the binary name.
func (c *CursorProvider) BinaryName() string {
	return "cursor"
}

// ConfigDir returns the config directory name.
func (c *CursorProvider) ConfigDir() string {
	return ".cursor"
}

// GenerateConfig generates Cursor configuration files.
func (c *CursorProvider) GenerateConfig(configDir string, config map[string]interface{}) error {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Placeholder: Cursor config generation not yet implemented
	// Full implementation will be added in a future version
	return nil
}

// ValidateConfig validates Cursor configuration.
func (c *CursorProvider) ValidateConfig(config map[string]interface{}) error {
	// Placeholder validation
	return nil
}
