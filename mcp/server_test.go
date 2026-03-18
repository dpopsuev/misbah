package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer() *httptest.Server {
	logger := metrics.NewLogger(metrics.LogLevelDebug, os.Stderr)
	recorder := metrics.NewMetricsRecorder()
	server := NewServer(logger, recorder)
	return httptest.NewServer(server)
}

func mcpPost(t *testing.T, url string, method string, params interface{}) *http.Response {
	t.Helper()
	req := MCPRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
	}
	if params != nil {
		data, _ := json.Marshal(params)
		req.Params = data
	}
	body, _ := json.Marshal(req)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	return resp
}

func decodeResult(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	defer resp.Body.Close()
	var result struct {
		Result map[string]interface{} `json:"result"`
		Error  *ErrorDetail           `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result.Result
}

func TestInitialize(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp := mcpPost(t, ts.URL, MethodInitialize, nil)
	result := decodeResult(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	serverInfo := result["serverInfo"].(map[string]interface{})
	assert.Equal(t, "misbah", serverInfo["name"])
	assert.Equal(t, "0.2.0", serverInfo["version"])

	assert.Equal(t, "2024-11-05", result["protocolVersion"])
}

func TestListTools(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp := mcpPost(t, ts.URL, MethodListTools, nil)
	result := decodeResult(t, resp)

	tools := result["tools"].([]interface{})
	assert.Len(t, tools, 3)

	names := make(map[string]bool)
	for _, tool := range tools {
		toolMap := tool.(map[string]interface{})
		names[toolMap["name"].(string)] = true
	}

	assert.True(t, names[ToolContainerCreate])
	assert.True(t, names[ToolContainerValidate])
	assert.True(t, names[ToolContainerInspect])
}

func TestContainerCreate(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	specPath := filepath.Join(t.TempDir(), "test.yaml")

	resp := mcpPost(t, ts.URL, MethodCallTool, map[string]interface{}{
		"name": ToolContainerCreate,
		"arguments": map[string]interface{}{
			"spec_path": specPath,
			"name":      "test-container",
		},
	})
	result := decodeResult(t, resp)

	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	assert.Contains(t, text, "Container spec created")
	assert.FileExists(t, specPath)
}

func TestContainerValidate_Valid(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	// Create a valid spec first
	specPath := filepath.Join(t.TempDir(), "valid.yaml")
	mcpPost(t, ts.URL, MethodCallTool, map[string]interface{}{
		"name": ToolContainerCreate,
		"arguments": map[string]interface{}{
			"spec_path": specPath,
			"name":      "valid-container",
		},
	}).Body.Close()

	resp := mcpPost(t, ts.URL, MethodCallTool, map[string]interface{}{
		"name": ToolContainerValidate,
		"arguments": map[string]interface{}{
			"spec_path": specPath,
		},
	})
	result := decodeResult(t, resp)

	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	assert.Contains(t, text, "valid")
}

func TestContainerValidate_Invalid(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	specPath := filepath.Join(t.TempDir(), "invalid.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte("version: \"2.0\"\nmetadata:\n  name: x\n"), 0644))

	resp := mcpPost(t, ts.URL, MethodCallTool, map[string]interface{}{
		"name": ToolContainerValidate,
		"arguments": map[string]interface{}{
			"spec_path": specPath,
		},
	})
	result := decodeResult(t, resp)

	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	assert.Contains(t, text, "Validation failed")

	isError := result["isError"].(bool)
	assert.True(t, isError)
}

func TestContainerInspect(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	specPath := filepath.Join(t.TempDir(), "inspect.yaml")
	mcpPost(t, ts.URL, MethodCallTool, map[string]interface{}{
		"name": ToolContainerCreate,
		"arguments": map[string]interface{}{
			"spec_path": specPath,
			"name":      "inspect-me",
		},
	}).Body.Close()

	resp := mcpPost(t, ts.URL, MethodCallTool, map[string]interface{}{
		"name": ToolContainerInspect,
		"arguments": map[string]interface{}{
			"spec_path": specPath,
		},
	})
	result := decodeResult(t, resp)

	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	assert.Contains(t, text, "inspect-me")
}

func TestUnknownMethod(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp := mcpPost(t, ts.URL, "nonexistent/method", nil)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestInvalidJSON(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Post(ts.URL, "application/json", bytes.NewReader([]byte("not json")))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGETRejected(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp, err := http.Get(ts.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestUnknownTool(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	resp := mcpPost(t, ts.URL, MethodCallTool, map[string]interface{}{
		"name":      "nonexistent_tool",
		"arguments": map[string]interface{}{},
	})
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "unknown tool")
}
