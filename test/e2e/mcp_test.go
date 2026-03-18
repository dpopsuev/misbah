//go:build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	mcpAddr = "http://localhost:18080"
)

// TestMCPWorkflow tests misbah via MCP server
func TestMCPWorkflow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	root := repoRoot(t)
	runInDir(t, root, "go", "build", "-o", "misbah", "./cmd/misbah")

	misbahBin := filepath.Join(root, "misbah")
	server := startMCPServer(t, misbahBin)
	defer server.Process.Kill()

	waitForMCPServer(t, 10*time.Second)

	testDir := t.TempDir()
	specFile := filepath.Join(testDir, "mcp-test.yaml")

	t.Run("mcp_initialize", func(t *testing.T) {
		result := mcpCall(t, "initialize", map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "test-client",
				"version": "1.0",
			},
		})

		serverInfo := result["serverInfo"].(map[string]interface{})
		assert(t, serverInfo["name"] == "misbah", "Server name should be 'misbah'")
		assert(t, serverInfo["version"] == "0.2.0", "Server version should be '0.2.0'")
	})

	t.Run("mcp_list_tools", func(t *testing.T) {
		result := mcpCall(t, "tools/list", nil)

		tools := result["tools"].([]interface{})
		assert(t, len(tools) > 0, "Should have tools")

		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolMap := tool.(map[string]interface{})
			toolNames[toolMap["name"].(string)] = true
		}

		requiredTools := []string{
			"misbah_container_create",
			"misbah_container_validate",
			"misbah_container_inspect",
		}

		for _, toolName := range requiredTools {
			assert(t, toolNames[toolName], fmt.Sprintf("Tool %s should exist", toolName))
		}
	})

	t.Run("mcp_container_create", func(t *testing.T) {
		result := mcpCallTool(t, "misbah_container_create", map[string]interface{}{
			"spec_path": specFile,
			"name":      "mcp-test-container",
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "Container spec created"), "Should report success")
	})

	t.Run("mcp_container_validate", func(t *testing.T) {
		result := mcpCallTool(t, "misbah_container_validate", map[string]interface{}{
			"spec_path": specFile,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "valid"), "Should validate successfully")
	})

	t.Run("mcp_container_inspect", func(t *testing.T) {
		result := mcpCallTool(t, "misbah_container_inspect", map[string]interface{}{
			"spec_path": specFile,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "mcp-test-container"), "Should contain container name")
	})
}

// --- MCP helpers ---

type mcpRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      int                    `json:"id"`
	Result  map[string]interface{} `json:"result"`
	Error   *mcpError              `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func startMCPServer(t *testing.T, misbahBin string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(misbahBin, "serve", "--port", "18080", "--log-level", "debug")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start MCP server: %v", err)
	}

	t.Logf("MCP server started (PID: %d)", cmd.Process.Pid)
	return cmd
}

func waitForMCPServer(t *testing.T, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(mcpAddr)
		if err == nil {
			resp.Body.Close()
			t.Logf("MCP server is ready")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("MCP server did not become ready within %s", timeout)
}

func mcpCall(t *testing.T, method string, params interface{}) map[string]interface{} {
	t.Helper()

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(mcpAddr, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("MCP call failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("MCP call returned status %d: %s", resp.StatusCode, body)
	}

	var result mcpResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Error != nil {
		t.Fatalf("MCP error: %s", result.Error.Message)
	}

	return result.Result
}

func mcpCallTool(t *testing.T, tool string, arguments map[string]interface{}) map[string]interface{} {
	t.Helper()

	params := map[string]interface{}{
		"name":      tool,
		"arguments": arguments,
	}

	return mcpCall(t, "tools/call", params)
}

func getToolContent(t *testing.T, result map[string]interface{}) string {
	t.Helper()

	content := result["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("No content in tool result")
	}

	firstContent := content[0].(map[string]interface{})
	return firstContent["text"].(string)
}

func assert(t *testing.T, condition bool, message string) {
	t.Helper()
	if !condition {
		t.Fatal(message)
	}
}
