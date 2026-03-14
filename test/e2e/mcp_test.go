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

	// Build misbah
	root := repoRoot(t)
	runInDir(t, root, "go", "build", "-o", "misbah", "./cmd/misbah")

	// Start MCP server
	jabalBin := filepath.Join(root, "misbah")
	server := startMCPServer(t, jabalBin)
	defer server.Process.Kill()

	// Wait for server to be ready
	waitForMCPServer(t, 10*time.Second)

	workspace := "mcp-e2e-" + time.Now().Format("20060102-150405")
	testDir := t.TempDir()

	sourceA := filepath.Join(testDir, "project-a")
	sourceB := filepath.Join(testDir, "project-b")

	if err := os.MkdirAll(sourceA, 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	if err := os.MkdirAll(sourceB, 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}

	defer func() {
		// Cleanup workspace
		workspaceDir := filepath.Join(os.Getenv("HOME"), ".config/misbah/workspaces", workspace)
		os.RemoveAll(workspaceDir)

		// Assert cleanup worked
		if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
			t.Errorf("Cleanup failed: workspace directory still exists at %s", workspaceDir)
		}
	}()

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
		assert(t, serverInfo["version"] == "0.1.0", "Server version should be '0.1.0'")
	})

	t.Run("mcp_list_tools", func(t *testing.T) {
		result := mcpCall(t, "tools/list", nil)

		tools := result["tools"].([]interface{})
		assert(t, len(tools) > 0, "Should have tools")

		// Verify expected tools exist
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolMap := tool.(map[string]interface{})
			toolNames[toolMap["name"].(string)] = true
		}

		requiredTools := []string{
			"jabal_list_workspaces",
			"jabal_create_workspace",
			"jabal_get_workspace",
			"jabal_update_manifest",
			"jabal_validate_workspace",
			"jabal_get_status",
			"jabal_list_providers",
		}

		for _, toolName := range requiredTools {
			assert(t, toolNames[toolName], fmt.Sprintf("Tool %s should exist", toolName))
		}
	})

	t.Run("mcp_create_workspace", func(t *testing.T) {
		result := mcpCallTool(t, "jabal_create_workspace", map[string]interface{}{
			"name":        workspace,
			"description": "MCP E2E test workspace",
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "created successfully"), "Should report success")
	})

	t.Run("mcp_list_workspaces", func(t *testing.T) {
		result := mcpCallTool(t, "jabal_list_workspaces", map[string]interface{}{})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, workspace), "Should list the workspace")
	})

	t.Run("mcp_update_manifest", func(t *testing.T) {
		manifest := map[string]interface{}{
			"name":        workspace,
			"description": "MCP E2E test workspace",
			"sources": []map[string]interface{}{
				{"path": sourceA, "mount": "project-a"},
				{"path": sourceB, "mount": "project-b"},
			},
			"providers": map[string]interface{}{
				"claude": map[string]interface{}{
					"mcp_servers": map[string]interface{}{
						"scribe": "http://localhost:8080",
					},
				},
			},
		}

		result := mcpCallTool(t, "jabal_update_manifest", map[string]interface{}{
			"name":     workspace,
			"manifest": manifest,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "updated successfully"), "Should update manifest")
	})

	t.Run("mcp_validate_workspace", func(t *testing.T) {
		result := mcpCallTool(t, "jabal_validate_workspace", map[string]interface{}{
			"name": workspace,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "is valid"), "Should validate successfully")
	})

	t.Run("mcp_get_workspace", func(t *testing.T) {
		result := mcpCallTool(t, "jabal_get_workspace", map[string]interface{}{
			"name": workspace,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, workspace), "Should return workspace details")
		assert(t, strings.Contains(content, "project-a"), "Should include sources")
	})

	t.Run("mcp_get_status", func(t *testing.T) {
		result := mcpCallTool(t, "jabal_get_status", map[string]interface{}{
			"name": workspace,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "Mounted"), "Should show mount status")
	})

	t.Run("mcp_list_providers", func(t *testing.T) {
		result := mcpCallTool(t, "jabal_list_providers", map[string]interface{}{})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "claude"), "Should list claude provider")
		assert(t, strings.Contains(content, "aider"), "Should list aider provider")
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

func startMCPServer(t *testing.T, jabalBin string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(jabalBin, "serve", "--port", "18080", "--log-level", "debug")
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
