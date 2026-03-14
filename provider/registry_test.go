package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	// Create a test provider
	testProvider := &ClaudeProvider{}

	// Register it (should not panic)
	Register(testProvider)

	// Verify it's registered
	assert.True(t, Exists("claude"))
}

func TestGet(t *testing.T) {
	t.Run("get existing provider", func(t *testing.T) {
		provider, err := Get("claude")
		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.Equal(t, "claude", provider.Name())
	})

	t.Run("get non-existent provider", func(t *testing.T) {
		_, err := Get("nonexistent")
		assert.Error(t, err)
	})
}

func TestList(t *testing.T) {
	providers := List()
	assert.NotEmpty(t, providers)

	// Should contain built-in providers
	assert.Contains(t, providers, "claude")
	assert.Contains(t, providers, "aider")
	assert.Contains(t, providers, "cursor")
}

func TestExists(t *testing.T) {
	assert.True(t, Exists("claude"))
	assert.True(t, Exists("aider"))
	assert.True(t, Exists("cursor"))
	assert.False(t, Exists("nonexistent"))
}

func TestGetProviderInfo(t *testing.T) {
	infos := GetProviderInfo()
	assert.NotEmpty(t, infos)

	// Verify at least one provider info
	found := false
	for _, info := range infos {
		if info.Name == "claude" {
			found = true
			assert.Equal(t, "claude", info.BinaryName)
			assert.Equal(t, ".claude", info.ConfigDir)
			assert.True(t, info.Supported)
		}
	}
	assert.True(t, found, "Should find claude provider info")
}
