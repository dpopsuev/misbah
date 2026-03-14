package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGlobalConfig(t *testing.T) {
	// Save original env and restore after test
	originalConfigDir := os.Getenv("MISBAH_CONFIG_DIR")
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("MISBAH_CONFIG_DIR", originalConfigDir)
		} else {
			os.Unsetenv("MISBAH_CONFIG_DIR")
		}
	}()

	tmpDir := t.TempDir()
	os.Setenv("MISBAH_CONFIG_DIR", tmpDir)

	t.Run("non-existent config returns defaults", func(t *testing.T) {
		config, err := LoadGlobalConfig()
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "claude", config.DefaultProvider)
		assert.Equal(t, "info", config.LogLevel)
		assert.True(t, config.AutoCleanup)
	})

	t.Run("load existing config", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "config.yaml")
		configContent := `default_provider: aider
log_level: debug
auto_cleanup: false
`
		require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

		config, err := LoadGlobalConfig()
		require.NoError(t, err)
		assert.Equal(t, "aider", config.DefaultProvider)
		assert.Equal(t, "debug", config.LogLevel)
		assert.False(t, config.AutoCleanup)
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		configPath := filepath.Join(tmpDir, "config.yaml")
		invalidYAML := `default_provider: aider
invalid yaml here
`
		require.NoError(t, os.WriteFile(configPath, []byte(invalidYAML), 0644))

		_, err := LoadGlobalConfig()
		assert.Error(t, err)
	})
}

func TestSaveGlobalConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Save original env and restore after test
	originalConfigDir := os.Getenv("MISBAH_CONFIG_DIR")
	defer func() {
		if originalConfigDir != "" {
			os.Setenv("MISBAH_CONFIG_DIR", originalConfigDir)
		} else {
			os.Unsetenv("MISBAH_CONFIG_DIR")
		}
	}()

	os.Setenv("MISBAH_CONFIG_DIR", tmpDir)

	config := &GlobalConfig{
		DefaultProvider: "aider",
		LogLevel:        "debug",
		AutoCleanup:     false,
	}

	err := config.SaveGlobalConfig()
	require.NoError(t, err)

	// Verify file was created
	configPath := GetGlobalConfigPath()
	_, err = os.Stat(configPath)
	assert.NoError(t, err)

	// Load and verify
	loaded, err := LoadGlobalConfig()
	require.NoError(t, err)
	assert.Equal(t, "aider", loaded.DefaultProvider)
	assert.Equal(t, "debug", loaded.LogLevel)
	assert.False(t, loaded.AutoCleanup)
}

func TestDefaultGlobalConfig(t *testing.T) {
	config := DefaultGlobalConfig()
	assert.NotNil(t, config)
	assert.Equal(t, "claude", config.DefaultProvider)
	assert.Equal(t, "info", config.LogLevel)
	assert.True(t, config.AutoCleanup)
	assert.NotEmpty(t, config.ConfigDir)
	assert.NotEmpty(t, config.TempDir)
}

func TestApplyDefaults(t *testing.T) {
	config := &GlobalConfig{
		DefaultProvider: "aider",
		// Other fields empty
	}

	applyDefaults(config)

	assert.Equal(t, "aider", config.DefaultProvider)
	assert.NotEmpty(t, config.ConfigDir)
	assert.NotEmpty(t, config.TempDir)
	assert.Equal(t, "info", config.LogLevel)
}
