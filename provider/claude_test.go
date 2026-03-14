package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeProviderName(t *testing.T) {
	provider := &ClaudeProvider{}
	assert.Equal(t, "claude", provider.Name())
}

func TestClaudeProviderBinaryName(t *testing.T) {
	provider := &ClaudeProvider{}
	assert.Equal(t, "claude", provider.BinaryName())
}

func TestClaudeProviderConfigDir(t *testing.T) {
	provider := &ClaudeProvider{}
	assert.Equal(t, ".claude", provider.ConfigDir())
}

func TestClaudeProviderGenerateConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".claude")
	provider := &ClaudeProvider{}

	t.Run("generate config with MCP servers", func(t *testing.T) {
		config := map[string]interface{}{
			"mcp_servers": map[string]interface{}{
				"scribe": "http://localhost:8080",
				"locus": map[string]interface{}{
					"url": "http://localhost:8081",
				},
			},
		}

		err := provider.GenerateConfig(configDir, config)
		require.NoError(t, err)

		// Verify config file was created
		settingsPath := filepath.Join(configDir, "settings.local.json")
		data, err := os.ReadFile(settingsPath)
		require.NoError(t, err)

		// Parse and verify
		var settings map[string]interface{}
		err = json.Unmarshal(data, &settings)
		require.NoError(t, err)

		mcpServers, ok := settings["mcpServers"].(map[string]interface{})
		require.True(t, ok)

		// Verify scribe was converted to object
		scribe, ok := mcpServers["scribe"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "http://localhost:8080", scribe["url"])

		// Verify locus is preserved
		locus, ok := mcpServers["locus"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "http://localhost:8081", locus["url"])
	})

	t.Run("generate config with settings", func(t *testing.T) {
		cleanDir := filepath.Join(tmpDir, ".claude2")
		config := map[string]interface{}{
			"settings": map[string]interface{}{
				"auto_memory":        true,
				"max_context_tokens": 200000,
			},
		}

		err := provider.GenerateConfig(cleanDir, config)
		require.NoError(t, err)

		// Verify settings were included
		settingsPath := filepath.Join(cleanDir, "settings.local.json")
		data, err := os.ReadFile(settingsPath)
		require.NoError(t, err)

		var settings map[string]interface{}
		err = json.Unmarshal(data, &settings)
		require.NoError(t, err)

		assert.Equal(t, true, settings["auto_memory"])
		assert.Equal(t, float64(200000), settings["max_context_tokens"])
	})

	t.Run("generate empty config", func(t *testing.T) {
		cleanDir := filepath.Join(tmpDir, ".claude3")
		config := map[string]interface{}{}

		err := provider.GenerateConfig(cleanDir, config)
		require.NoError(t, err)

		// Verify minimal config was created
		settingsPath := filepath.Join(cleanDir, "settings.local.json")
		data, err := os.ReadFile(settingsPath)
		require.NoError(t, err)

		var settings map[string]interface{}
		err = json.Unmarshal(data, &settings)
		require.NoError(t, err)

		// Should have mcpServers even if empty
		_, ok := settings["mcpServers"]
		assert.True(t, ok)
	})
}

func TestClaudeProviderValidateConfig(t *testing.T) {
	provider := &ClaudeProvider{}

	t.Run("valid config with MCP servers", func(t *testing.T) {
		config := map[string]interface{}{
			"mcp_servers": map[string]interface{}{
				"scribe": "http://localhost:8080",
			},
		}

		err := provider.ValidateConfig(config)
		assert.NoError(t, err)
	})

	t.Run("valid empty config", func(t *testing.T) {
		err := provider.ValidateConfig(nil)
		assert.NoError(t, err)
	})

	t.Run("invalid MCP servers (not a map)", func(t *testing.T) {
		config := map[string]interface{}{
			"mcp_servers": "invalid",
		}

		err := provider.ValidateConfig(config)
		assert.Error(t, err)
	})
}

func TestConvertMCPServers(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "string URL converted to object",
			input: map[string]interface{}{
				"scribe": "http://localhost:8080",
			},
			expected: map[string]interface{}{
				"scribe": map[string]interface{}{
					"url": "http://localhost:8080",
				},
			},
		},
		{
			name: "object preserved",
			input: map[string]interface{}{
				"locus": map[string]interface{}{
					"url": "http://localhost:8081",
					"key": "value",
				},
			},
			expected: map[string]interface{}{
				"locus": map[string]interface{}{
					"url": "http://localhost:8081",
					"key": "value",
				},
			},
		},
		{
			name: "mixed types",
			input: map[string]interface{}{
				"scribe": "http://localhost:8080",
				"locus": map[string]interface{}{
					"url": "http://localhost:8081",
				},
			},
			expected: map[string]interface{}{
				"scribe": map[string]interface{}{
					"url": "http://localhost:8080",
				},
				"locus": map[string]interface{}{
					"url": "http://localhost:8081",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertMCPServers(tt.input)

			// Compare each server
			for name, expectedConfig := range tt.expected {
				actualConfig, ok := result[name]
				require.True(t, ok, "Server %s not found in result", name)

				expectedMap := expectedConfig.(map[string]interface{})
				actualMap := actualConfig.(map[string]interface{})

				for key, expectedValue := range expectedMap {
					actualValue, ok := actualMap[key]
					require.True(t, ok, "Key %s not found in server %s", key, name)
					assert.Equal(t, expectedValue, actualValue)
				}
			}
		})
	}
}
