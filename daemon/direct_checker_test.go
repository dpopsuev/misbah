package daemon

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectChecker_CheckWhitelistHit(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(ResourceNetwork, "api.github.com", DecisionAlways)

	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())

	resp, err := checker.Check(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)
}

func TestDirectChecker_CheckWhitelistMiss(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())

	resp, err := checker.Check(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceNetwork,
		ResourceID:   "unknown.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionUnknown, resp.Decision)
}

func TestDirectChecker_RequestPromptsUser(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	prompter := &fixedPrompter{decision: DecisionAlways}
	checker := NewDirectChecker(whitelist, prompter, nil, testLogger())

	resp, err := checker.Request(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)

	// Should be persisted in whitelist
	d, ok := whitelist.Check(ResourceNetwork, "api.github.com")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)
}

func TestDirectChecker_RequestWhitelistShortCircuits(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(ResourceNetwork, "api.github.com", DecisionAlways)

	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())

	resp, err := checker.Request(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)
	assert.Equal(t, "whitelist", resp.Reason)
}

func TestDirectChecker_RequestDenyPersisted(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())

	resp, err := checker.Request(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceNetwork,
		ResourceID:   "evil.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionDeny, resp.Decision)

	d, ok := whitelist.Check(ResourceNetwork, "evil.com")
	assert.True(t, ok)
	assert.Equal(t, DecisionDeny, d)
}

func TestDirectChecker_RequestOnceNotPersisted(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	prompter := &fixedPrompter{decision: DecisionOnce}
	checker := NewDirectChecker(whitelist, prompter, nil, testLogger())

	resp, err := checker.Request(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceMCP,
		ResourceID:   "bash",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionOnce, resp.Decision)

	_, ok := whitelist.Check(ResourceMCP, "bash")
	assert.False(t, ok)
}
