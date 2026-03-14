//go:build e2e && llm

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestLLMWithMCP tests LLM agent interacting with jabal via MCP
func TestLLMWithMCP(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	// Check Ollama availability
	if !isOllamaAvailable(t) {
		t.Skip("Ollama not available")
	}

	// Use available model
	model := detectAvailableModel(t)
	if model == "" {
		t.Skip("No suitable LLM model available in Ollama")
	}

	t.Logf("Using LLM model: %s", model)

	// Build and start MCP server
	root := repoRoot(t)
	runInDir(t, root, "go", "build", "-o", "jabal", "./cmd/jabal")

	jabalBin := filepath.Join(root, "jabal")
	server := startMCPServer(t, jabalBin)
	defer server.Process.Kill()

	waitForMCPServer(t, 10*time.Second)

	workspace := "llm-mcp-" + time.Now().Format("20060102-150405")
	testDir := t.TempDir()

	sourceA := filepath.Join(testDir, "api-service")
	sourceB := filepath.Join(testDir, "web-frontend")

	if err := os.MkdirAll(sourceA, 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	if err := os.MkdirAll(sourceB, 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}

	defer func() {
		// Cleanup workspace
		workspaceDir := filepath.Join(os.Getenv("HOME"), ".config/jabal/workspaces", workspace)
		os.RemoveAll(workspaceDir)

		// Assert cleanup worked
		if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
			t.Errorf("Cleanup failed: workspace directory still exists at %s", workspaceDir)
		}
	}()

	// Test: LLM discovers MCP tools
	t.Run("llm_discover_tools", func(t *testing.T) {
		prompt := `You are testing the jabal workspace manager via MCP protocol.

The MCP server is running at http://localhost:18080

Task: List all available MCP tools by calling the tools/list method.

Respond with a JSON-RPC request to list the tools. Output ONLY the JSON request, nothing else.`

		response := queryLLM(t, model, prompt)
		t.Logf("LLM generated request:\n%s", response)

		// Verify LLM generated valid JSON-RPC
		var req map[string]interface{}
		if err := json.Unmarshal([]byte(response), &req); err != nil {
			t.Logf("LLM response was not pure JSON, extracting...")
			// Try to extract JSON from response
			response = extractJSON(response)
			if err := json.Unmarshal([]byte(response), &req); err != nil {
				t.Fatalf("LLM did not generate valid JSON-RPC: %v", err)
			}
		}

		assert(t, req["method"] == "tools/list", "LLM should generate tools/list request")
	})

	// Test: LLM creates workspace via MCP
	t.Run("llm_create_workspace", func(t *testing.T) {
		prompt := fmt.Sprintf(`You are testing the jabal workspace manager via MCP protocol.

Task: Create a workspace named "%s" with description "LLM-managed test workspace" using the jabal_create_workspace tool.

Generate a JSON-RPC request to call this tool. Output ONLY the JSON request, nothing else.`, workspace)

		response := queryLLM(t, model, prompt)
		response = extractJSON(response)
		t.Logf("LLM generated:\n%s", response)

		// Execute the LLM-generated request
		var req mcpRequest
		if err := json.Unmarshal([]byte(response), &req); err != nil {
			t.Fatalf("Failed to parse LLM response: %v", err)
		}

		// Actually execute it
		result := executeMCPRequest(t, &req)
		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "created successfully"), "Workspace should be created")
	})

	// Test: LLM generates manifest
	t.Run("llm_generate_manifest", func(t *testing.T) {
		prompt := fmt.Sprintf(`You are testing the jabal workspace manager.

Task: Generate a workspace manifest JSON object with:
- name: %s
- description: "Full-stack application workspace"
- Two sources:
  1. Path: %s, Mount: api
  2. Path: %s, Mount: web
- Provider: claude with MCP server "scribe" at http://localhost:8080

Output ONLY the manifest JSON object (not the full MCP request), nothing else.`, workspace, sourceA, sourceB)

		response := queryLLM(t, model, prompt)
		response = extractJSON(response)
		t.Logf("LLM generated manifest:\n%s", response)

		// Parse manifest
		var manifest map[string]interface{}
		if err := json.Unmarshal([]byte(response), &manifest); err != nil {
			t.Fatalf("Failed to parse LLM manifest: %v", err)
		}

		// Update via MCP
		result := mcpCallTool(t, "jabal_update_manifest", map[string]interface{}{
			"name":     workspace,
			"manifest": manifest,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "updated successfully"), "Manifest should update")
	})

	// Test: LLM validates workspace
	t.Run("llm_validate_and_explain", func(t *testing.T) {
		// Get workspace details
		result := mcpCallTool(t, "jabal_get_workspace", map[string]interface{}{
			"name": workspace,
		})
		manifestJSON := getToolContent(t, result)

		prompt := fmt.Sprintf(`You are reviewing a jabal workspace configuration.

Workspace manifest:
%s

Tasks:
1. Is this manifest valid? Check if it has required fields (name, sources).
2. Summarize what this workspace is for in one sentence.

Respond in this format:
Valid: [yes/no]
Purpose: [one sentence summary]`, manifestJSON)

		response := queryLLM(t, model, prompt)
		t.Logf("LLM analysis:\n%s", response)

		// Verify LLM understood the workspace
		assert(t, strings.Contains(strings.ToLower(response), "valid"), "LLM should check validity")
		assert(t, strings.Contains(strings.ToLower(response), "workspace") ||
			     strings.Contains(strings.ToLower(response), "api") ||
			     strings.Contains(strings.ToLower(response), "web"), "LLM should explain purpose")
	})

	// Test: LLM troubleshoots an issue
	t.Run("llm_troubleshoot", func(t *testing.T) {
		// Create workspace with invalid manifest
		badWorkspace := "llm-bad-" + time.Now().Format("20060102-150405")

		mcpCallTool(t, "jabal_create_workspace", map[string]interface{}{
			"name": badWorkspace,
		})

		// Try to update with invalid manifest (duplicate mount names)
		badManifest := map[string]interface{}{
			"name": badWorkspace,
			"sources": []map[string]interface{}{
				{"path": sourceA, "mount": "duplicate"},
				{"path": sourceB, "mount": "duplicate"}, // Same mount name!
			},
		}

		result := mcpCallTool(t, "jabal_update_manifest", map[string]interface{}{
			"name":     badWorkspace,
			"manifest": badManifest,
		})

		errorMsg := getToolContent(t, result)
		t.Logf("Error from jabal: %s", errorMsg)

		prompt := fmt.Sprintf(`You are debugging a jabal workspace error.

Error message:
%s

Task: Explain what the problem is and how to fix it.`, errorMsg)

		response := queryLLM(t, model, prompt)
		t.Logf("LLM troubleshooting:\n%s", response)

		// Verify LLM understood the error
		assert(t, strings.Contains(strings.ToLower(response), "duplicate") ||
			     strings.Contains(strings.ToLower(response), "mount") ||
			     strings.Contains(strings.ToLower(response), "unique"), "LLM should identify duplicate issue")

		// Cleanup
		os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".config/jabal/workspaces", badWorkspace))
	})
}

// --- Helpers ---

func detectAvailableModel(t *testing.T) string {
	t.Helper()

	// Preferred models in order
	candidates := []string{
		"qwen2.5-coder:7b-instruct",
		"qwen2.5-coder:1.5b-instruct",
		"qwen2.5:32b",
		"qwen2.5:14b",
		"qwen2.5:7b",
	}

	for _, model := range candidates {
		if isModelAvailable(t, model) {
			return model
		}
	}

	return ""
}

func extractJSON(text string) string {
	// Try to extract JSON from markdown code blocks or mixed text
	start := strings.Index(text, "{")
	if start == -1 {
		return text
	}

	// Find matching closing brace
	depth := 0
	for i := start; i < len(text); i++ {
		if text[i] == '{' {
			depth++
		} else if text[i] == '}' {
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}

	return text
}

func executeMCPRequest(t *testing.T, req *mcpRequest) map[string]interface{} {
	t.Helper()

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(mcpAddr, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("MCP request failed: %v", err)
	}
	defer resp.Body.Close()

	var result mcpResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Error != nil {
		t.Fatalf("MCP error: %s", result.Error.Message)
	}

	return result.Result
}

func queryLLM(t *testing.T, model string, prompt string) string {
	t.Helper()

	req := ollamaRequest{
		Model:  model,
		Prompt: prompt,
		Stream: false,
	}

	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(ollamaBaseURL+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to query LLM: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("LLM query failed with status %d: %s", resp.StatusCode, body)
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	return strings.TrimSpace(result.Response)
}
