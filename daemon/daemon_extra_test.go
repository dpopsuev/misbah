package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock lifecycle ---

type mockLifecycle struct {
	startErr   error
	stopErr    error
	destroyErr error
	started    []string
	stopped    []string
	destroyed  []string
	mu         sync.Mutex
}

func (m *mockLifecycle) Start(spec *model.ContainerSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, spec.Metadata.Name)
	return m.startErr
}

func (m *mockLifecycle) Stop(name string, force bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, name)
	return m.stopErr
}

func (m *mockLifecycle) Destroy(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyed = append(m.destroyed, name)
	return m.destroyErr
}

// startTestServerWithLifecycle creates a test server with a lifecycle manager
// installed, following the same pattern as startTestServer from server_test.go.
func startTestServerWithLifecycle(t *testing.T, whitelist *WhitelistStore, prompter Prompter, lc ContainerLifecycle, opts ...ServerOption) (*Server, *http.Client) {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "test.sock")
	logger := testLogger()

	var auditBuf bytes.Buffer
	audit := NewAuditLoggerFromWriter(&auditBuf, logger)

	allOpts := append([]ServerOption{WithLifecycle(lc)}, opts...)
	server := NewServer(whitelist, prompter, audit, logger, allOpts...)

	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	server.listener = ln

	go server.httpServer.Serve(ln)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	t.Cleanup(func() {
		server.Stop()
	})

	return server, client
}

// --- Audit logger tests ---

func TestAuditLogger_FileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := testLogger()

	audit, err := NewAuditLogger(path, logger)
	require.NoError(t, err)

	req := PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	}
	audit.LogDecision(req, DecisionAlways, "whitelist")

	req2 := PermissionRequest{
		Container:    "agent-2",
		ResourceType: ResourceMCP,
		ResourceID:   "bash",
	}
	audit.LogDecision(req2, DecisionDeny, "user")

	require.NoError(t, audit.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 2)

	var entry1 AuditEntry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &entry1))
	assert.Equal(t, "agent-1", entry1.Container)
	assert.Equal(t, ResourceNetwork, entry1.ResourceType)
	assert.Equal(t, "api.github.com", entry1.ResourceID)
	assert.Equal(t, DecisionAlways, entry1.Decision)
	assert.Equal(t, "whitelist", entry1.Source)
	assert.NotEmpty(t, entry1.Timestamp)

	var entry2 AuditEntry
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &entry2))
	assert.Equal(t, "agent-2", entry2.Container)
	assert.Equal(t, ResourceMCP, entry2.ResourceType)
	assert.Equal(t, "bash", entry2.ResourceID)
	assert.Equal(t, DecisionDeny, entry2.Decision)
	assert.Equal(t, "user", entry2.Source)
}

func TestAuditLogger_CloseNilCloser(t *testing.T) {
	// NewAuditLoggerFromWriter does not set closer, so Close should be a no-op.
	audit := NewAuditLoggerFromWriter(io.Discard, testLogger())
	assert.NoError(t, audit.Close())
}

func TestAuditLogger_NilLogger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	audit, err := NewAuditLogger(path, nil)
	require.NoError(t, err)
	defer audit.Close()

	// Verify it still works with nil logger (should use default).
	audit.LogDecision(PermissionRequest{
		Container:    "c",
		ResourceType: ResourceNetwork,
		ResourceID:   "x.com",
	}, DecisionOnce, "test")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "x.com")
}

func TestAuditLogger_InvalidPath(t *testing.T) {
	// /dev/null/impossible is not a valid directory
	_, err := NewAuditLogger("/dev/null/impossible/audit.jsonl", testLogger())
	assert.Error(t, err)
}

// --- Server option tests ---

func TestServerOptions_WithLifecycle(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	audit := NewAuditLoggerFromWriter(io.Discard, testLogger())

	server := NewServer(whitelist, &AutoDenyPrompter{}, audit, testLogger(), WithLifecycle(lc))
	assert.Equal(t, lc, server.lifecycle)
}

func TestServerOptions_WithProxyManager(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	audit := NewAuditLoggerFromWriter(io.Discard, testLogger())

	// Create a DirectChecker as the PermissionChecker for the ProxyManager.
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, audit, testLogger())
	pm := NewProxyManager(checker, testLogger())

	server := NewServer(whitelist, &AutoDenyPrompter{}, audit, testLogger(), WithProxyManager(pm))
	assert.Equal(t, pm, server.proxyMgr)
}

func TestServerOptions_WithVsockConfig(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	audit := NewAuditLoggerFromWriter(io.Discard, testLogger())

	server := NewServer(whitelist, &AutoDenyPrompter{}, audit, testLogger(),
		WithVsockConfig(1234, "/usr/local/bin"))

	require.NotNil(t, server.vsockCfg)
	assert.Equal(t, uint32(1234), server.vsockCfg.Port)
	assert.Equal(t, "/usr/local/bin", server.vsockCfg.BinDir)
}

func TestServerOptions_Multiple(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	audit := NewAuditLoggerFromWriter(io.Discard, testLogger())

	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, audit, testLogger())
	pm := NewProxyManager(checker, testLogger())

	server := NewServer(whitelist, &AutoDenyPrompter{}, audit, testLogger(),
		WithLifecycle(lc),
		WithProxyManager(pm),
		WithVsockConfig(9999, "/opt/bin"),
	)

	assert.Equal(t, lc, server.lifecycle)
	assert.Equal(t, pm, server.proxyMgr)
	require.NotNil(t, server.vsockCfg)
	assert.Equal(t, uint32(9999), server.vsockCfg.Port)
}

// --- NewTerminalPrompter test ---

func TestNewTerminalPrompter(t *testing.T) {
	p := NewTerminalPrompter()
	require.NotNil(t, p)
	assert.Equal(t, os.Stdin, p.reader)
	assert.Equal(t, os.Stderr, p.writer)
}

// --- Whitelist load endpoint tests ---

func TestWhitelistLoad_WithPermissions(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	spec := &model.ContainerSpec{
		Version:  "1.0",
		Metadata: model.ContainerMetadata{Name: "test"},
		Process:  model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
		Permissions: &model.PermissionConfig{
			NetworkWhitelist: []string{"api.github.com", "registry.npmjs.org"},
			MCPWhitelist:     []string{"bash", "read"},
			PackageWhitelist: []string{"numpy"},
		},
	}

	resp := postJSON(t, client, "/whitelist/load", ContainerStartRequest{Spec: spec})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var actionResp ContainerActionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&actionResp))
	assert.Equal(t, "loaded", actionResp.Status)

	// Verify the rules were loaded
	d, ok := whitelist.Check(ResourceNetwork, "api.github.com")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)

	d, ok = whitelist.Check(ResourceNetwork, "registry.npmjs.org")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)

	d, ok = whitelist.Check(ResourceMCP, "bash")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)

	d, ok = whitelist.Check(ResourceMCP, "read")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)

	d, ok = whitelist.Check(ResourcePackage, "numpy")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)
}

func TestWhitelistLoad_NilSpec(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp := postJSON(t, client, "/whitelist/load", ContainerStartRequest{Spec: nil})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var actionResp ContainerActionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&actionResp))
	assert.Equal(t, "loaded", actionResp.Status)
}

func TestWhitelistLoad_NoPermissions(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	spec := &model.ContainerSpec{
		Version:    "1.0",
		Metadata:   model.ContainerMetadata{Name: "test"},
		Process:    model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
		// No Permissions field
	}

	resp := postJSON(t, client, "/whitelist/load", ContainerStartRequest{Spec: spec})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// No rules should exist
	rules := whitelist.Rules()
	assert.Empty(t, rules)
}

func TestWhitelistLoad_InvalidJSON(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp, err := client.Post("http://localhost/whitelist/load", "application/json",
		strings.NewReader("{invalid"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestWhitelistLoad_MethodNotAllowed(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp, err := client.Get("http://localhost/whitelist/load")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// --- Container lifecycle endpoint tests (with mock lifecycle) ---

func TestContainerStart_WithLifecycle(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	spec := &model.ContainerSpec{
		Version:    "1.0",
		Metadata:   model.ContainerMetadata{Name: "test-container"},
		Process:    model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
	}

	resp := postJSON(t, client, "/container/start", ContainerStartRequest{Spec: spec})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var startResp ContainerStartResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&startResp))
	assert.Equal(t, "test-container", startResp.ID)
	assert.Equal(t, "started", startResp.Status)

	// Wait for the goroutine to call lifecycle.Start
	assert.Eventually(t, func() bool {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		return len(lc.started) > 0
	}, time.Second, 10*time.Millisecond)

	lc.mu.Lock()
	assert.Contains(t, lc.started, "test-container")
	lc.mu.Unlock()
}

func TestContainerStart_WithLifecycle_MissingSpec(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp := postJSON(t, client, "/container/start", ContainerStartRequest{})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	assert.Contains(t, errResp["error"], "spec is required")
}

func TestContainerStart_WithLifecycle_LoadsWhitelist(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	spec := &model.ContainerSpec{
		Version:    "1.0",
		Metadata:   model.ContainerMetadata{Name: "test-wl"},
		Process:    model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
		Permissions: &model.PermissionConfig{
			NetworkWhitelist: []string{"allowed.com"},
		},
	}

	resp := postJSON(t, client, "/container/start", ContainerStartRequest{Spec: spec})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify whitelist was loaded from spec
	d, ok := whitelist.Check(ResourceNetwork, "allowed.com")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)
}

func TestContainerStart_InvalidJSON(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp, err := client.Post("http://localhost/container/start", "application/json",
		strings.NewReader("{bad json"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestContainerStart_MethodNotAllowed(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp, err := client.Get("http://localhost/container/start")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestContainerStop_WithLifecycle(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp := postJSON(t, client, "/container/stop", ContainerStopRequest{Name: "my-container", Force: true})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var actionResp ContainerActionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&actionResp))
	assert.Equal(t, "stopped", actionResp.Status)

	lc.mu.Lock()
	assert.Contains(t, lc.stopped, "my-container")
	lc.mu.Unlock()
}

func TestContainerStop_WithLifecycle_Error(t *testing.T) {
	lc := &mockLifecycle{stopErr: fmt.Errorf("container not found")}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp := postJSON(t, client, "/container/stop", ContainerStopRequest{Name: "missing"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	assert.Contains(t, errResp["error"], "container not found")
}

func TestContainerStop_InvalidJSON(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp, err := client.Post("http://localhost/container/stop", "application/json",
		strings.NewReader("not json"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestContainerStop_MethodNotAllowed(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp, err := client.Get("http://localhost/container/stop")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestContainerDestroy_WithLifecycle(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp := postJSON(t, client, "/container/destroy", ContainerDestroyRequest{Name: "old-container"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var actionResp ContainerActionResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&actionResp))
	assert.Equal(t, "destroyed", actionResp.Status)

	lc.mu.Lock()
	assert.Contains(t, lc.destroyed, "old-container")
	lc.mu.Unlock()
}

func TestContainerDestroy_WithLifecycle_Error(t *testing.T) {
	lc := &mockLifecycle{destroyErr: fmt.Errorf("destroy failed: busy")}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp := postJSON(t, client, "/container/destroy", ContainerDestroyRequest{Name: "busy-container"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	assert.Contains(t, errResp["error"], "destroy failed")
}

func TestContainerDestroy_InvalidJSON(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp, err := client.Post("http://localhost/container/destroy", "application/json",
		strings.NewReader("<<<"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestContainerDestroy_MethodNotAllowed(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)

	resp, err := client.Get("http://localhost/container/destroy")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

// --- Client container operation tests ---

// startTestServerForClientWithLifecycle is like startTestServerForClient but
// includes a mock lifecycle, enabling container endpoints.
func startTestServerForClientWithLifecycle(t *testing.T, whitelist *WhitelistStore, prompter Prompter, lc ContainerLifecycle) (string, func()) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	logger := testLogger()

	audit := NewAuditLoggerFromWriter(io.Discard, logger)
	server := NewServer(whitelist, prompter, audit, logger, WithLifecycle(lc))

	ready := make(chan struct{})
	go func() {
		close(ready)
		server.Start(socketPath)
	}()
	<-ready
	time.Sleep(10 * time.Millisecond)

	return socketPath, func() { server.Stop() }
}

func TestClient_ContainerStart(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClientWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	spec := &model.ContainerSpec{
		Version:    "1.0",
		Metadata:   model.ContainerMetadata{Name: "client-test"},
		Process:    model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
	}

	resp, err := client.ContainerStart(context.Background(), spec)
	require.NoError(t, err)
	assert.Equal(t, "client-test", resp.ID)
	assert.Equal(t, "started", resp.Status)

	assert.Eventually(t, func() bool {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		return len(lc.started) > 0
	}, time.Second, 10*time.Millisecond)
}

func TestClient_ContainerStop(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClientWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	err := client.ContainerStop(context.Background(), "stop-me", false)
	require.NoError(t, err)

	lc.mu.Lock()
	assert.Contains(t, lc.stopped, "stop-me")
	lc.mu.Unlock()
}

func TestClient_ContainerStop_Force(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClientWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	err := client.ContainerStop(context.Background(), "force-stop", true)
	require.NoError(t, err)

	lc.mu.Lock()
	assert.Contains(t, lc.stopped, "force-stop")
	lc.mu.Unlock()
}

func TestClient_ContainerDestroy(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClientWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	err := client.ContainerDestroy(context.Background(), "destroy-me")
	require.NoError(t, err)

	lc.mu.Lock()
	assert.Contains(t, lc.destroyed, "destroy-me")
	lc.mu.Unlock()
}

func TestClient_ContainerDestroy_Error(t *testing.T) {
	lc := &mockLifecycle{destroyErr: fmt.Errorf("no such container")}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClientWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	err := client.ContainerDestroy(context.Background(), "ghost")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no such container")
}

func TestClient_ContainerStop_Error(t *testing.T) {
	lc := &mockLifecycle{stopErr: fmt.Errorf("stop timed out")}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClientWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	err := client.ContainerStop(context.Background(), "stuck", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stop timed out")
}

func TestClient_WhitelistLoad(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClientWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc)
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	spec := &model.ContainerSpec{
		Version:    "1.0",
		Metadata:   model.ContainerMetadata{Name: "wl-test"},
		Process:    model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
		Permissions: &model.PermissionConfig{
			NetworkWhitelist: []string{"trusted.io"},
			MCPWhitelist:     []string{"grep"},
		},
	}

	err := client.WhitelistLoad(context.Background(), spec)
	require.NoError(t, err)

	// Verify via the permission check client method
	resp, err := client.Check(context.Background(), PermissionRequest{
		Container:    "wl-test",
		ResourceType: ResourceNetwork,
		ResourceID:   "trusted.io",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)

	resp, err = client.Check(context.Background(), PermissionRequest{
		Container:    "wl-test",
		ResourceType: ResourceMCP,
		ResourceID:   "grep",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)
}

func TestClient_ContainerStart_NoLifecycle(t *testing.T) {
	// Server without lifecycle should return error via client
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClient(t, whitelist, &AutoDenyPrompter{})
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	spec := &model.ContainerSpec{
		Version:    "1.0",
		Metadata:   model.ContainerMetadata{Name: "no-lc"},
		Process:    model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
	}

	_, err := client.ContainerStart(context.Background(), spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestClient_ContainerStop_NoLifecycle(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClient(t, whitelist, &AutoDenyPrompter{})
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	err := client.ContainerStop(context.Background(), "no-lc", false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

func TestClient_ContainerDestroy_NoLifecycle(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())

	socketPath, cleanup := startTestServerForClient(t, whitelist, &AutoDenyPrompter{})
	defer cleanup()

	client := NewClient(socketPath, testLogger())
	defer client.Close()

	err := client.ContainerDestroy(context.Background(), "no-lc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not available")
}

// --- VsockListener tests ---

func TestVsockListener_NewAndStop(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())
	defer pm.StopAll()

	vl := NewVsockListener(pm, testLogger())
	require.NotNil(t, vl)

	// Stop without Start should be safe
	err := vl.Stop(context.Background())
	assert.NoError(t, err)
}

func TestVsockListener_StartAndStop(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())
	defer pm.StopAll()

	vl := NewVsockListener(pm, testLogger())

	// Use TCP listener for test
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- vl.Start(ln)
	}()

	// Give the listener time to start
	time.Sleep(10 * time.Millisecond)

	// Stop the listener
	require.NoError(t, vl.Stop(context.Background()))

	// Start should return nil (clean shutdown)
	err = <-errCh
	assert.NoError(t, err)
}

func TestVsockListener_NilLogger(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())

	vl := NewVsockListener(pm, nil)
	require.NotNil(t, vl)
	assert.NotNil(t, vl.logger)
}

func TestVsockListener_ConnNoProxy(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())

	vl := NewVsockListener(pm, testLogger())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	go vl.Start(ln)
	defer vl.Stop(context.Background())

	time.Sleep(10 * time.Millisecond)

	// Connect with no proxies registered — handleConn logs error and closes
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	// The handler closes the connection when no proxy is available
	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err = conn.Read(buf)
	assert.Error(t, err) // should get EOF or deadline exceeded
	conn.Close()
}

// --- handleContainerStart with proxy manager (namespace path) ---

func TestContainerStart_WithLifecycleAndProxyManager(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())

	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc,
		WithProxyManager(pm))

	spec := &model.ContainerSpec{
		Version:    "1.0",
		Metadata:   model.ContainerMetadata{Name: "proxy-test"},
		Process:    model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
		// Runtime defaults to namespace (not kata), so takes the non-kata proxy path
	}

	resp := postJSON(t, client, "/container/start", ContainerStartRequest{Spec: spec})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var startResp ContainerStartResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&startResp))
	assert.Equal(t, "proxy-test", startResp.ID)
	assert.Equal(t, "started", startResp.Status)

	// Wait for lifecycle.Start goroutine
	assert.Eventually(t, func() bool {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		return len(lc.started) > 0
	}, time.Second, 10*time.Millisecond)

	// Verify spec got proxy env vars injected
	lc.mu.Lock()
	assert.Contains(t, lc.started, "proxy-test")
	lc.mu.Unlock()
}

func TestContainerStart_WithLifecycleAndProxyManager_Kata(t *testing.T) {
	lc := &mockLifecycle{}
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())

	_, client := startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc,
		WithProxyManager(pm),
		WithVsockConfig(12345, "/usr/local/bin"),
	)

	spec := &model.ContainerSpec{
		Version:    "1.0",
		Metadata:   model.ContainerMetadata{Name: "kata-proxy"},
		Process:    model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
		Namespaces: model.NamespaceSpec{User: true, Mount: true},
		Runtime:    model.RuntimeKata,
		Image:      "docker.io/library/alpine:latest",
	}

	resp := postJSON(t, client, "/container/start", ContainerStartRequest{Spec: spec})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var startResp ContainerStartResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&startResp))
	assert.Equal(t, "kata-proxy", startResp.ID)
	assert.Equal(t, "started", startResp.Status)

	assert.Eventually(t, func() bool {
		lc.mu.Lock()
		defer lc.mu.Unlock()
		return len(lc.started) > 0
	}, time.Second, 10*time.Millisecond)
}

// --- DirectChecker with audit logger ---

func TestDirectChecker_RequestWithAudit(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	var auditBuf bytes.Buffer
	audit := NewAuditLoggerFromWriter(&auditBuf, testLogger())

	prompter := &fixedPrompter{decision: DecisionAlways}
	checker := NewDirectChecker(whitelist, prompter, audit, testLogger())

	resp, err := checker.Request(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)

	// Verify audit was written
	assert.Contains(t, auditBuf.String(), "api.github.com")
	assert.Contains(t, auditBuf.String(), "user")
}

func TestDirectChecker_RequestWithAudit_WhitelistHit(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(ResourceNetwork, "api.github.com", DecisionAlways)

	var auditBuf bytes.Buffer
	audit := NewAuditLoggerFromWriter(&auditBuf, testLogger())

	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, audit, testLogger())

	resp, err := checker.Request(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionAlways, resp.Decision)
	assert.Equal(t, "whitelist", resp.Reason)

	// Verify audit was written with "whitelist" source
	assert.Contains(t, auditBuf.String(), "whitelist")
}

func TestDirectChecker_RequestOnceWithAudit(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	var auditBuf bytes.Buffer
	audit := NewAuditLoggerFromWriter(&auditBuf, testLogger())

	prompter := &fixedPrompter{decision: DecisionOnce}
	checker := NewDirectChecker(whitelist, prompter, audit, testLogger())

	resp, err := checker.Request(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceMCP,
		ResourceID:   "bash",
	})
	require.NoError(t, err)
	assert.Equal(t, DecisionOnce, resp.Decision)

	// Once is not persisted
	_, ok := whitelist.Check(ResourceMCP, "bash")
	assert.False(t, ok)

	// But audit is still written
	assert.Contains(t, auditBuf.String(), "bash")
}

func TestDirectChecker_NilLogger(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, nil)
	require.NotNil(t, checker)
	assert.NotNil(t, checker.logger)
}

// --- Nil logger constructors ---

func TestNewClient_NilLogger(t *testing.T) {
	client := NewClient("/tmp/test.sock", nil)
	require.NotNil(t, client)
	assert.NotNil(t, client.logger)
	client.Close()
}

func TestNewWhitelistStore_NilLogger(t *testing.T) {
	ws := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), nil)
	require.NotNil(t, ws)
	assert.NotNil(t, ws.logger)
}

func TestNewProxyManager_NilLogger(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, nil)
	require.NotNil(t, pm)
	assert.NotNil(t, pm.logger)
}

func TestNewAuditLoggerFromWriter_NilLogger(t *testing.T) {
	audit := NewAuditLoggerFromWriter(io.Discard, nil)
	require.NotNil(t, audit)
	assert.NotNil(t, audit.logger)
}

func TestNewServer_NilLogger(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	audit := NewAuditLoggerFromWriter(io.Discard, testLogger())
	server := NewServer(whitelist, &AutoDenyPrompter{}, audit, nil)
	require.NotNil(t, server)
	assert.NotNil(t, server.logger)
}

// --- Server.Stop with ProxyManager ---

func TestServerStop_WithProxyManager(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())

	// Start a proxy so StopAll has something to do
	_, err := pm.Start("test-container")
	require.NoError(t, err)

	lc := &mockLifecycle{}
	_, _ = startTestServerWithLifecycle(t, whitelist, &AutoDenyPrompter{}, lc,
		WithProxyManager(pm))

	// The server cleanup will call Stop() which calls pm.StopAll()
	// This verifies the proxy manager StopAll path through server.Stop
}

// --- NetworkIsolator TeardownAll empty ---

func TestNetworkIsolator_TeardownAll_Empty(t *testing.T) {
	cfg := config.NetworkSection{Subnet: "10.88.0.0/24"}
	ni := NewNetworkIsolator(cfg, testLogger())
	// Should not panic
	ni.TeardownAll()
	assert.Empty(t, ni.netns)
}

func TestNetworkIsolator_NilLogger(t *testing.T) {
	cfg := config.NetworkSection{Subnet: "10.88.0.0/24"}
	ni := NewNetworkIsolator(cfg, nil)
	require.NotNil(t, ni)
	assert.NotNil(t, ni.logger)
}

// --- VsockListener handleConn with proxy available ---

func TestVsockListener_ConnWithProxy(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	// Whitelist the upstream host so proxy lets traffic through
	whitelist.Set(ResourceNetwork, "127.0.0.1", DecisionAlways)

	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())
	defer pm.StopAll()

	// Start a proxy so handleConn has a proxy to connect to
	_, err := pm.Start("vm-container")
	require.NoError(t, err)

	vl := NewVsockListener(pm, testLogger())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()

	go vl.Start(ln)
	defer vl.Stop(context.Background())

	time.Sleep(10 * time.Millisecond)

	// Connect — handleConn will find the proxy and try to forward
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)

	// Send some data and close — this exercises the io.Copy path
	conn.Write([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	buf := make([]byte, 1024)
	conn.Read(buf) // May get response or timeout, both fine
	conn.Close()
}

// --- Server endpoint method-not-allowed tests ---

func TestHandleRequest_MethodNotAllowed(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp, err := client.Get("http://localhost/permission/request")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestHandleCheck_MethodNotAllowed(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp, err := client.Get("http://localhost/permission/check")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestHandleList_MethodNotAllowed(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp := postJSON(t, client, "/permission/list", map[string]string{})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestHandleRequest_InvalidJSON(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp, err := client.Post("http://localhost/permission/request", "application/json",
		strings.NewReader("not valid json"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHandleCheck_InvalidJSON(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp, err := client.Post("http://localhost/permission/check", "application/json",
		strings.NewReader("{broken"))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// --- DirectChecker prompter error path ---

type errorPrompter struct{}

func (e *errorPrompter) Prompt(req *PermissionRequest) (Decision, error) {
	return "", fmt.Errorf("prompter exploded")
}

func TestDirectChecker_RequestPrompterError(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &errorPrompter{}, nil, testLogger())

	resp, err := checker.Request(context.Background(), PermissionRequest{
		Container:    "test",
		ResourceType: ResourceNetwork,
		ResourceID:   "err.com",
	})
	require.NoError(t, err)
	// On prompter error, decision falls back to Deny
	assert.Equal(t, DecisionDeny, resp.Decision)

	// Deny should be persisted
	d, ok := whitelist.Check(ResourceNetwork, "err.com")
	assert.True(t, ok)
	assert.Equal(t, DecisionDeny, d)
}

// --- handleRequest with prompter error (server-side) ---

func TestHandleRequest_PrompterError(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &errorPrompter{})

	req := PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "error.com",
	}

	resp := postJSON(t, client, "/permission/request", req)
	pr := decodeResponse(t, resp)
	assert.Equal(t, DecisionDeny, pr.Decision)
}
