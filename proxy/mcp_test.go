package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mcpTestSetup(t *testing.T, checker PermissionChecker, upstreamHandler http.Handler) (*http.Client, string) {
	t.Helper()

	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)

	upstreamURL, _ := url.Parse(upstream.URL)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyAddr := ln.Addr().String()
	logger := metrics.NewLogger(metrics.LogLevelDebug, os.Stderr)
	mcpProxy := NewMCPProxy(checker, "test-container", proxyAddr, upstreamURL, logger)

	go func() { mcpProxy.StartOnListener(ln) }()
	t.Cleanup(func() { mcpProxy.Stop(context.Background()) })

	return &http.Client{}, "http://" + proxyAddr
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
			"jsonrpc": "2.0", "id": 1,
			"result": map[string]interface{}{"content": []map[string]string{{"type": "text", "text": "success"}}},
		})
	})

	checker := newMockChecker().withWhitelist(ResourceMCP, "scribe_list", DecisionAlways)
	client, proxyURL := mcpTestSetup(t, checker, upstream)

	resp := postMCP(t, client, proxyURL, mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: json.RawMessage(`{"name":"scribe_list","arguments":{}}`)})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMCPProxy_DeniedToolCall(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach") })

	checker := newMockChecker()
	client, proxyURL := mcpTestSetup(t, checker, upstream)

	resp := postMCP(t, client, proxyURL, mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: json.RawMessage(`{"name":"admin_vacuum","arguments":{}}`)})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestMCPProxy_NonToolCallPassesThrough(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": map[string]interface{}{"tools": []interface{}{}}})
	})

	checker := newMockChecker()
	client, proxyURL := mcpTestSetup(t, checker, upstream)

	resp := postMCP(t, client, proxyURL, mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestMCPProxy_PermissionCaching(t *testing.T) {
	cp := newCountingPrompter(DecisionAlways)
	checker := newMockChecker().withPrompter(cp.prompt)

	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": map[string]interface{}{}})
	})

	client, proxyURL := mcpTestSetup(t, checker, upstream)

	postMCP(t, client, proxyURL, mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: json.RawMessage(`{"name":"scribe_list","arguments":{}}`)}).Body.Close()
	postMCP(t, client, proxyURL, mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: json.RawMessage(`{"name":"scribe_list","arguments":{}}`)}).Body.Close()

	assert.Equal(t, int32(1), cp.count.Load())
}

func TestMCPProxy_DifferentToolsSeparatePermissions(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": map[string]interface{}{}})
	})

	checker := newMockChecker().withWhitelist(ResourceMCP, "scribe_list", DecisionAlways)
	client, proxyURL := mcpTestSetup(t, checker, upstream)

	resp1 := postMCP(t, client, proxyURL, mcpRequest{JSONRPC: "2.0", ID: 1, Method: "tools/call", Params: json.RawMessage(`{"name":"scribe_list","arguments":{}}`)})
	resp1.Body.Close()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	resp2 := postMCP(t, client, proxyURL, mcpRequest{JSONRPC: "2.0", ID: 2, Method: "tools/call", Params: json.RawMessage(`{"name":"scribe_archive","arguments":{}}`)})
	resp2.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp2.StatusCode)
}

func TestMCPProxy_InvalidJSON(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach") })
	checker := newMockChecker()
	client, proxyURL := mcpTestSetup(t, checker, upstream)

	resp, err := client.Post(proxyURL, "application/json", bytes.NewReader([]byte("not json")))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestMCPProxy_GETRejected(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("should not reach") })
	checker := newMockChecker()
	client, proxyURL := mcpTestSetup(t, checker, upstream)

	resp, err := client.Get(proxyURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}
