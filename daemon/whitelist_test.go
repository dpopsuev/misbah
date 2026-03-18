package daemon

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhitelistCheckNonExistent(t *testing.T) {
	w := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	_, ok := w.Check(ResourceNetwork, "nonexistent.com")
	assert.False(t, ok)
}

func TestWhitelistSetAndCheck(t *testing.T) {
	w := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	w.Set(ResourceNetwork, "api.github.com", DecisionAlways)
	d, ok := w.Check(ResourceNetwork, "api.github.com")

	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)
}

func TestWhitelistLoadSaveRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.yaml")

	// Create and save
	w1 := NewWhitelistStore(wlPath, testLogger())
	w1.Set(ResourceNetwork, "api.github.com", DecisionAlways)
	w1.Set(ResourceMCP, "bash", DecisionDeny)
	w1.Set(ResourcePackage, "numpy", DecisionAlways)
	require.NoError(t, w1.Save())

	// Verify file exists
	assert.FileExists(t, wlPath)

	// Load into new store
	w2 := NewWhitelistStore(wlPath, testLogger())
	require.NoError(t, w2.Load())

	d, ok := w2.Check(ResourceNetwork, "api.github.com")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)

	d, ok = w2.Check(ResourceMCP, "bash")
	assert.True(t, ok)
	assert.Equal(t, DecisionDeny, d)

	d, ok = w2.Check(ResourcePackage, "numpy")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)
}

func TestWhitelistLoadNonExistent(t *testing.T) {
	w := NewWhitelistStore(filepath.Join(t.TempDir(), "nonexistent.yaml"), testLogger())
	err := w.Load()
	assert.NoError(t, err)
}

func TestWhitelistLoadFromSpec(t *testing.T) {
	w := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	spec := &model.ContainerSpec{
		Permissions: &model.PermissionConfig{
			NetworkWhitelist: []string{"api.github.com", "pypi.org"},
			MCPWhitelist:     []string{"bash", "read"},
			PackageWhitelist: []string{"numpy"},
		},
	}

	w.LoadFromSpec(spec)

	d, ok := w.Check(ResourceNetwork, "api.github.com")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)

	d, ok = w.Check(ResourceNetwork, "pypi.org")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)

	d, ok = w.Check(ResourceMCP, "bash")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)

	d, ok = w.Check(ResourcePackage, "numpy")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)
}

func TestWhitelistLoadFromSpecNil(t *testing.T) {
	w := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	spec := &model.ContainerSpec{}
	w.LoadFromSpec(spec) // Should not panic

	rules := w.Rules()
	assert.Empty(t, rules)
}

func TestWhitelistConcurrentAccess(t *testing.T) {
	w := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			w.Set(ResourceNetwork, "example.com", DecisionAlways)
		}(i)
		go func(i int) {
			defer wg.Done()
			w.Check(ResourceNetwork, "example.com")
		}(i)
	}
	wg.Wait()
}

func TestWhitelistRulesSnapshot(t *testing.T) {
	w := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	w.Set(ResourceNetwork, "a.com", DecisionAlways)
	w.Set(ResourceMCP, "bash", DecisionDeny)

	rules := w.Rules()
	assert.Len(t, rules, 2)

	// Modifying snapshot doesn't affect store
	rules["network:b.com"] = DecisionAlways
	_, ok := w.Check(ResourceNetwork, "b.com")
	assert.False(t, ok)
}

func TestWhitelistSaveCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "subdir", "nested", "wl.yaml")

	w := NewWhitelistStore(wlPath, testLogger())
	w.Set(ResourceNetwork, "example.com", DecisionAlways)
	require.NoError(t, w.Save())

	_, err := os.Stat(wlPath)
	assert.NoError(t, err)
}
