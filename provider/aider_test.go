package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAiderProviderName(t *testing.T) {
	provider := &AiderProvider{}
	assert.Equal(t, "aider", provider.Name())
}

func TestAiderProviderBinaryName(t *testing.T) {
	provider := &AiderProvider{}
	assert.Equal(t, "aider", provider.BinaryName())
}

func TestAiderProviderConfigDir(t *testing.T) {
	provider := &AiderProvider{}
	assert.Equal(t, ".aider", provider.ConfigDir())
}

func TestAiderProviderGenerateConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".aider")
	provider := &AiderProvider{}

	t.Run("generate config with options", func(t *testing.T) {
		config := map[string]interface{}{
			"model":        "gpt-4",
			"auto-commits": true,
			"edit-format":  "diff",
		}

		err := provider.GenerateConfig(configDir, config)
		require.NoError(t, err)

		// Verify config file was created
		configPath := filepath.Join(configDir, ".aider.conf.yml")
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)

		// Parse and verify
		var settings map[string]interface{}
		err = yaml.Unmarshal(data, &settings)
		require.NoError(t, err)

		assert.Equal(t, "gpt-4", settings["model"])
		assert.Equal(t, true, settings["auto-commits"])
		assert.Equal(t, "diff", settings["edit-format"])
	})

	t.Run("generate empty config", func(t *testing.T) {
		cleanDir := filepath.Join(tmpDir, ".aider2")
		config := map[string]interface{}{}

		err := provider.GenerateConfig(cleanDir, config)
		require.NoError(t, err)

		// Verify minimal config was created
		configPath := filepath.Join(cleanDir, ".aider.conf.yml")
		data, err := os.ReadFile(configPath)
		require.NoError(t, err)

		var settings map[string]interface{}
		err = yaml.Unmarshal(data, &settings)
		require.NoError(t, err)

		// Should have auto-commits as default
		assert.Equal(t, false, settings["auto-commits"])
	})
}

func TestAiderProviderValidateConfig(t *testing.T) {
	provider := &AiderProvider{}

	t.Run("valid config", func(t *testing.T) {
		config := map[string]interface{}{
			"model":        "gpt-4",
			"auto-commits": true,
			"edit-format":  "diff",
		}

		err := provider.ValidateConfig(config)
		assert.NoError(t, err)
	})

	t.Run("valid empty config", func(t *testing.T) {
		err := provider.ValidateConfig(nil)
		assert.NoError(t, err)
	})

	t.Run("invalid model type", func(t *testing.T) {
		config := map[string]interface{}{
			"model": 123,
		}

		err := provider.ValidateConfig(config)
		assert.Error(t, err)
	})

	t.Run("invalid auto-commits type", func(t *testing.T) {
		config := map[string]interface{}{
			"auto-commits": "yes",
		}

		err := provider.ValidateConfig(config)
		assert.Error(t, err)
	})

	t.Run("invalid edit-format type", func(t *testing.T) {
		config := map[string]interface{}{
			"edit-format": 123,
		}

		err := provider.ValidateConfig(config)
		assert.Error(t, err)
	})
}
