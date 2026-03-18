package daemon

import (
	"context"
	"io"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startTestServerForClient(t *testing.T, whitelist *WhitelistStore, prompter Prompter) (string, func()) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	logger := testLogger()

	audit := NewAuditLoggerFromWriter(io.Discard, logger)
	server := NewServer(whitelist, prompter, audit, logger)

	ready := make(chan struct{})
	go func() {
		close(ready)
		server.Start(socketPath)
	}()
	<-ready
	// Give the server a moment to start listening
	time.Sleep(10 * time.Millisecond)

	return socketPath, func() { server.Stop() }
}

func TestClientCheck_WhitelistHit(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(ResourceNetwork, "api.github.com", DecisionAlways)

	socketPath, cleanup := startTestServerForClient(t, whitelist, &AutoDenyPrompter{})
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	resp, err := client.Check(context.Background(), PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)
}

func TestClientCheck_WhitelistMiss(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClient(t, whitelist, &AutoDenyPrompter{})
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	resp, err := client.Check(context.Background(), PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "unknown.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionUnknown, resp.Decision)
}

func TestClientRequest_UserDeny(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClient(t, whitelist, &AutoDenyPrompter{})
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	resp, err := client.Request(context.Background(), PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "evil.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionDeny, resp.Decision)
}

func TestClientRequest_UserAlways(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	prompter := &fixedPrompter{decision: DecisionAlways}

	socketPath, cleanup := startTestServerForClient(t, whitelist, prompter)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	resp, err := client.Request(context.Background(), PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)

	// Subsequent check should return always (persisted)
	resp2, err := client.Check(context.Background(), PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp2.Decision)
}

func TestClientRequest_UserOnce(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	prompter := &fixedPrompter{decision: DecisionOnce}

	socketPath, cleanup := startTestServerForClient(t, whitelist, prompter)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	resp, err := client.Request(context.Background(), PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "temp.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionOnce, resp.Decision)

	// Subsequent check should return unknown (once is not persisted)
	resp2, err := client.Check(context.Background(), PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "temp.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionUnknown, resp2.Decision)
}

func TestClientRequest_ContextCancellation(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	// Blocking prompter that waits until unblocked
	blocker := &blockingPrompter{ch: make(chan struct{})}

	socketPath, cleanup := startTestServerForClient(t, whitelist, blocker)

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Request(ctx, PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "slow.com",
	})
	assert.Error(t, err)

	// Unblock the prompter so the server can shut down cleanly
	close(blocker.ch)
	cleanup()
}

func TestClientCheck_InvalidSocket(t *testing.T) {
	client := NewClient("/tmp/nonexistent-socket-path.sock", testLogger())
	defer client.Close()

	_, err := client.Check(context.Background(), PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "example.com",
	})
	assert.Error(t, err)
}

func TestClientConcurrent(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(ResourceNetwork, "api.github.com", DecisionAlways)

	socketPath, cleanup := startTestServerForClient(t, whitelist, &AutoDenyPrompter{})
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Check(context.Background(), PermissionRequest{
				Container:    "agent-1",
				ResourceType: ResourceNetwork,
				ResourceID:   "api.github.com",
			})
			assert.NoError(t, err)
			assert.Equal(t, DecisionAlways, resp.Decision)
		}()
	}
	wg.Wait()
}

// blockingPrompter blocks until the channel is closed.
type blockingPrompter struct {
	ch chan struct{}
}

func (b *blockingPrompter) Prompt(req *PermissionRequest) (Decision, error) {
	<-b.ch
	return DecisionDeny, nil
}
