package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeProvider implements the Provider interface for Claude Code.
type ClaudeProvider struct{}

// Name returns the provider name.
func (c *ClaudeProvider) Name() string {
	return "claude"
}

// BinaryName returns the binary name.
func (c *ClaudeProvider) BinaryName() string {
	return "claude"
}

// ConfigDir returns the config directory name.
func (c *ClaudeProvider) ConfigDir() string {
	return ".claude"
}

// GenerateConfig generates Claude Code configuration files.
func (c *ClaudeProvider) GenerateConfig(configDir string, config map[string]interface{}) error {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Generate settings.local.json
	settingsPath := filepath.Join(configDir, "settings.local.json")

	// Build Claude configuration
	claudeConfig := make(map[string]interface{})

	// Handle MCP servers if present
	if mcpServers, ok := config["mcp_servers"].(map[string]interface{}); ok {
		claudeConfig["mcpServers"] = convertMCPServers(mcpServers)
	}

	// Handle other settings
	if settings, ok := config["settings"].(map[string]interface{}); ok {
		for key, value := range settings {
			claudeConfig[key] = value
		}
	}

	// If config is empty, create minimal config
	if len(claudeConfig) == 0 {
		claudeConfig = map[string]interface{}{
			"mcpServers": map[string]interface{}{},
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(claudeConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Claude config: %w", err)
	}

	// Write config file
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write Claude config: %w", err)
	}

	return nil
}

// ValidateConfig validates Claude Code configuration.
func (c *ClaudeProvider) ValidateConfig(config map[string]interface{}) error {
	// Basic validation
	// Claude config is flexible, so we just verify it's a valid map
	if config == nil {
		return nil // Empty config is valid
	}

	// If mcp_servers is present, validate it's a map
	if mcpServers, ok := config["mcp_servers"]; ok {
		if _, ok := mcpServers.(map[string]interface{}); !ok {
			return fmt.Errorf("mcp_servers must be a map")
		}
	}

	return nil
}

// convertMCPServers converts manifest MCP server config to Claude format.
func convertMCPServers(mcpServers map[string]interface{}) map[string]interface{} {
	converted := make(map[string]interface{})

	for name, config := range mcpServers {
		// If config is a string (URL), convert to object
		if url, ok := config.(string); ok {
			converted[name] = map[string]interface{}{
				"url": url,
			}
		} else if configMap, ok := config.(map[string]interface{}); ok {
			// Already a map, use as-is
			converted[name] = configMap
		}
	}

	return converted
}
