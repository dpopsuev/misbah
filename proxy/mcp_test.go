package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/daemon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mcpTestSetup creates daemon server + MCP proxy + upstream MCP server.
// Returns the proxy client URL and cleanup.
func mcpTestSetup(t *testing.T, whitelist *daemon.WhitelistStore, prompter daemon.Prompter, upstreamHandler http.Handler) (*http.Client, string) {
	t.Helper()

	// Start upstream MCP server
	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)

	// Start daemon
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	logger := testLogger()
	audit := daemon.NewAuditLoggerFromWriter(io.Discard, logger)
	daemonServer := daemon.NewServer(whitelist, prompter, audit, logger)

	daemonReady := make(chan struct{})
	go func() {
		close(daemonReady)
		daemonServer.Start(socketPath)
	}()
	<-daemonReady
	time.Sleep(10 * time.Millisecond)
	t.Cleanup(func() { daemonServer.Stop() })

	daemonClient := daemon.NewClient(socketPath, logger)
	t.Cleanup(func() { daemonClient.Close() })

	// Start MCP proxy
	upstreamURL, _ := url.Parse(upstream.URL)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyAddr := ln.Addr().String()
	mcpProxy := NewMCPProxy(daemonClient, "test-container", proxyAddr, upstreamURL, logger)

	go func() { mcpProxy.StartOnListener(ln) }()
	t.Cleanup(func() { mcpProxy.Stop(context.Background()) })

	client := &http.Client{}
	return client, "http://" + proxyAddr
}

func postMCP(t *testing.T, client *http.Client, baseURL string, req interface{}) *http.Response {
	t.Helper()
	data, err := json.Marshal(req)
	require.NoError(t, err)
	resp, err := client.Post(baseURL, "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	return resp
}

func TestMCPProxy_AllowedToolCall(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "success"}},
			},
		})
	})

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(daemon.ResourceMCP, "scribe_list", daemon.DecisionAlways)

	client, proxyURL := mcpTestSetup(t, whitelist, &daemon.AutoDenyPrompter{}, upstream)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"scribe_list","arguments":{}}`),
	}

	resp := postMCP(t, client, proxyURL, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	assert.NotNil(t, result["result"])
}

func TestMCPProxy_DeniedToolCall(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach upstream")
	})

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	client, proxyURL := mcpTestSetup(t, whitelist, &daemon.AutoDenyPrompter{}, upstream)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"admin_vacuum","arguments":{}}`),
	}

	resp := postMCP(t, client, proxyURL, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	var errResp mcpErrorResponse
	json.NewDecoder(resp.Body).Decode(&errResp)
	assert.Contains(t, errResp.Error.Message, "access denied")
	assert.Contains(t, errResp.Error.Message, "admin_vacuum")
}

func TestMCPProxy_NonToolCallPassesThrough(t *testing.T) {
	var receivedMethod string
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req mcpRequest
		json.Unmarshal(body, &req)
		receivedMethod = req.Method
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{"tools": []interface{}{}},
		})
	})

	// Empty whitelist — doesn't matter because tools/list isn't intercepted
	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	client, proxyURL := mcpTestSetup(t, whitelist, &daemon.AutoDenyPrompter{}, upstream)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	resp := postMCP(t, client, proxyURL, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "tools/list", receivedMethod)
}

func TestMCPProxy_InitializePassesThrough(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"serverInfo":      map[string]string{"name": "test", "version": "0.1.0"},
			},
		})
	})

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	client, proxyURL := mcpTestSetup(t, whitelist, &daemon.AutoDenyPrompter{}, upstream)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	}

	resp := postMCP(t, client, proxyURL, req)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMCPProxy_PermissionCaching(t *testing.T) {
	var count atomic.Int32
	prompter := &countingFixedPrompter{decision: daemon.DecisionAlways, count: &count}

	callCount := 0
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{},
		})
	})

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	client, proxyURL := mcpTestSetup(t, whitelist, prompter, upstream)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"scribe_list","arguments":{}}`),
	}

	// First call triggers prompt
	resp := postMCP(t, client, proxyURL, req)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Second call cached
	resp2 := postMCP(t, client, proxyURL, req)
	resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	assert.Equal(t, int32(1), count.Load())
	assert.Equal(t, 2, callCount) // Both forwarded to upstream
}

func TestMCPProxy_OnceNotCached(t *testing.T) {
	var count atomic.Int32
	prompter := &countingFixedPrompter{decision: daemon.DecisionOnce, count: &count}

	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{},
		})
	})

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	client, proxyURL := mcpTestSetup(t, whitelist, prompter, upstream)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"scribe_list","arguments":{}}`),
	}

	resp := postMCP(t, client, proxyURL, req)
	resp.Body.Close()

	resp2 := postMCP(t, client, proxyURL, req)
	resp2.Body.Close()

	// Prompted twice since Once is not cached
	assert.Equal(t, int32(2), count.Load())
}

func TestMCPProxy_DifferentToolsSeparatePermissions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result":  map[string]interface{}{},
		})
	})

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(daemon.ResourceMCP, "scribe_list", daemon.DecisionAlways)
	// scribe_archive is NOT whitelisted

	client, proxyURL := mcpTestSetup(t, whitelist, &daemon.AutoDenyPrompter{}, upstream)

	// Allowed tool
	req1 := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"scribe_list","arguments":{}}`),
	}
	resp1 := postMCP(t, client, proxyURL, req1)
	resp1.Body.Close()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	// Denied tool
	req2 := mcpRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"scribe_archive","arguments":{}}`),
	}
	resp2 := postMCP(t, client, proxyURL, req2)
	resp2.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp2.StatusCode)
}

func TestMCPProxy_InvalidJSON(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach upstream")
	})

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	client, proxyURL := mcpTestSetup(t, whitelist, &daemon.AutoDenyPrompter{}, upstream)

	resp, err := client.Post(proxyURL, "application/json", bytes.NewReader([]byte("not json")))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMCPProxy_GETRejected(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach upstream")
	})

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	client, proxyURL := mcpTestSetup(t, whitelist, &daemon.AutoDenyPrompter{}, upstream)

	resp, err := client.Get(proxyURL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}
