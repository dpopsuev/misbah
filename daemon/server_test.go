package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *metrics.Logger {
	return metrics.NewLogger(metrics.LogLevelDebug, os.Stderr)
}

func startTestServer(t *testing.T, whitelist *WhitelistStore, prompter Prompter) (*Server, *http.Client) {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "test.sock")
	logger := testLogger()

	var auditBuf bytes.Buffer
	audit := NewAuditLoggerFromWriter(&auditBuf, logger)

	server := NewServer(whitelist, prompter, audit, logger)

	ready := make(chan struct{})
	go func() {
		// Remove socket if exists
		os.Remove(socketPath)
		ln, err := net.Listen("unix", socketPath)
		require.NoError(t, err)
		server.listener = ln

		mux := http.NewServeMux()
		mux.HandleFunc("/permission/request", server.handleRequest)
		mux.HandleFunc("/permission/check", server.handleCheck)
		mux.HandleFunc("/permission/list", server.handleList)
		mux.HandleFunc("/container/start", server.handleContainerStart)
		mux.HandleFunc("/container/stop", server.handleContainerStop)
		mux.HandleFunc("/container/destroy", server.handleContainerDestroy)
		mux.HandleFunc("/whitelist/load", server.handleWhitelistLoad)

		server.httpServer = &http.Server{Handler: mux}
		close(ready)
		server.httpServer.Serve(ln)
	}()
	<-ready

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

func postJSON(t *testing.T, client *http.Client, path string, body interface{}) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := client.Post("http://localhost"+path, "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	return resp
}

func decodeResponse(t *testing.T, resp *http.Response) PermissionResponse {
	t.Helper()
	defer resp.Body.Close()
	var pr PermissionResponse
	err := json.NewDecoder(resp.Body).Decode(&pr)
	require.NoError(t, err)
	return pr
}

func TestServerAutoDeny(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	req := PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "evil.com",
	}

	resp := postJSON(t, client, "/permission/request", req)
	pr := decodeResponse(t, resp)

	assert.Equal(t, DecisionDeny, pr.Decision)
}

func TestServerWhitelistAlways(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(ResourceNetwork, "api.github.com", DecisionAlways)

	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	req := PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "api.github.com",
	}

	resp := postJSON(t, client, "/permission/request", req)
	pr := decodeResponse(t, resp)

	assert.Equal(t, DecisionAlways, pr.Decision)
	assert.Equal(t, "whitelist", pr.Reason)
}

func TestServerOnceNotPersisted(t *testing.T) {
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.yaml")
	whitelist := NewWhitelistStore(wlPath, testLogger())

	// Use a prompter that returns Once
	prompter := &fixedPrompter{decision: DecisionOnce}
	_, client := startTestServer(t, whitelist, prompter)

	req := PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceMCP,
		ResourceID:   "bash",
	}

	resp := postJSON(t, client, "/permission/request", req)
	pr := decodeResponse(t, resp)
	assert.Equal(t, DecisionOnce, pr.Decision)

	// Check should return unknown (once is not persisted)
	resp2 := postJSON(t, client, "/permission/check", req)
	pr2 := decodeResponse(t, resp2)
	assert.Equal(t, DecisionUnknown, pr2.Decision)
}

func TestServerAlwaysPersisted(t *testing.T) {
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.yaml")
	whitelist := NewWhitelistStore(wlPath, testLogger())

	prompter := &fixedPrompter{decision: DecisionAlways}
	_, client := startTestServer(t, whitelist, prompter)

	req := PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourcePackage,
		ResourceID:   "numpy",
	}

	resp := postJSON(t, client, "/permission/request", req)
	pr := decodeResponse(t, resp)
	assert.Equal(t, DecisionAlways, pr.Decision)

	// Check should return always (persisted)
	resp2 := postJSON(t, client, "/permission/check", req)
	pr2 := decodeResponse(t, resp2)
	assert.Equal(t, DecisionAlways, pr2.Decision)

	// Verify survives reload
	whitelist2 := NewWhitelistStore(wlPath, testLogger())
	require.NoError(t, whitelist2.Load())
	d, ok := whitelist2.Check(ResourcePackage, "numpy")
	assert.True(t, ok)
	assert.Equal(t, DecisionAlways, d)
}

func TestServerDenyPersisted(t *testing.T) {
	tmpDir := t.TempDir()
	wlPath := filepath.Join(tmpDir, "wl.yaml")
	whitelist := NewWhitelistStore(wlPath, testLogger())

	prompter := &fixedPrompter{decision: DecisionDeny}
	_, client := startTestServer(t, whitelist, prompter)

	req := PermissionRequest{
		Container:    "agent-1",
		ResourceType: ResourceNetwork,
		ResourceID:   "bad.com",
	}

	// First request goes through prompter
	resp := postJSON(t, client, "/permission/request", req)
	pr := decodeResponse(t, resp)
	assert.Equal(t, DecisionDeny, pr.Decision)

	// Subsequent check fast-paths via whitelist
	resp2 := postJSON(t, client, "/permission/check", req)
	pr2 := decodeResponse(t, resp2)
	assert.Equal(t, DecisionDeny, pr2.Decision)
}

func TestServerConcurrentRequests(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := PermissionRequest{
				Container:    "agent-1",
				ResourceType: ResourceNetwork,
				ResourceID:   "example.com",
			}
			resp := postJSON(t, client, "/permission/check", req)
			resp.Body.Close()
		}(i)
	}
	wg.Wait()
}

func TestServerList(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(ResourceNetwork, "api.github.com", DecisionAlways)
	whitelist.Set(ResourceMCP, "bash", DecisionDeny)

	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp, err := client.Get("http://localhost/permission/list")
	require.NoError(t, err)
	defer resp.Body.Close()

	var result map[string]map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	rules := result["rules"]
	assert.Equal(t, "always", rules["network:api.github.com"])
	assert.Equal(t, "deny", rules["mcp:bash"])
}

// fixedPrompter always returns the same decision (for testing).
type fixedPrompter struct {
	decision Decision
}

func (f *fixedPrompter) Prompt(req *PermissionRequest) (Decision, error) {
	return f.decision, nil
}

// --- Container endpoint tests ---

func TestContainerStart_NoLifecycle(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp := postJSON(t, client, "/container/start", ContainerStartRequest{
		Spec: &model.ContainerSpec{
			Version:  "1.0",
			Metadata: model.ContainerMetadata{Name: "test"},
			Process:  model.ProcessSpec{Command: []string{"/bin/true"}, Cwd: "/"},
			Namespaces: model.NamespaceSpec{User: true, Mount: true},
		},
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var errResp map[string]string
	json.NewDecoder(resp.Body).Decode(&errResp)
	assert.Contains(t, errResp["error"], "not available")
}

func TestContainerStop_NoLifecycle(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp := postJSON(t, client, "/container/stop", ContainerStopRequest{Name: "test"})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestContainerDestroy_NoLifecycle(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp := postJSON(t, client, "/container/destroy", ContainerDestroyRequest{Name: "test"})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestContainerStart_MissingSpec(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client := startTestServer(t, whitelist, &AutoDenyPrompter{})

	resp := postJSON(t, client, "/container/start", ContainerStartRequest{})
	defer resp.Body.Close()

	// No lifecycle, so service unavailable before spec check
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
