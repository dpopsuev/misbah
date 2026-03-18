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

// TestLLMWithMCP tests LLM agent interacting with misbah via MCP
func TestLLMWithMCP(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	if !isOllamaAvailable(t) {
		t.Skip("Ollama not available")
	}

	model := detectAvailableModel(t)
	if model == "" {
		t.Skip("No suitable LLM model available in Ollama")
	}

	t.Logf("Using LLM model: %s", model)

	root := repoRoot(t)
	runInDir(t, root, "go", "build", "-o", "misbah", "./cmd/misbah")

	misbahBin := filepath.Join(root, "misbah")
	server := startMCPServer(t, misbahBin)
	defer server.Process.Kill()

	waitForMCPServer(t, 10*time.Second)

	testDir := t.TempDir()
	specFile := filepath.Join(testDir, "llm-mcp-container.yaml")

	// Test: LLM discovers MCP tools
	t.Run("llm_discover_tools", func(t *testing.T) {
		prompt := `You are testing the misbah container manager via MCP protocol.

The MCP server is running at http://localhost:18080

Task: List all available MCP tools by calling the tools/list method.

Respond with a JSON-RPC request to list the tools. Output ONLY the JSON request, nothing else.`

		response := queryLLMWithModel(t, model, prompt)
		t.Logf("LLM generated request:\n%s", response)

		var req map[string]interface{}
		if err := json.Unmarshal([]byte(response), &req); err != nil {
			response = extractJSON(response)
			if err := json.Unmarshal([]byte(response), &req); err != nil {
				t.Fatalf("LLM did not generate valid JSON-RPC: %v", err)
			}
		}

		assert(t, req["method"] == "tools/list", "LLM should generate tools/list request")
	})

	// Test: LLM creates container spec via MCP
	t.Run("llm_create_container", func(t *testing.T) {
		prompt := fmt.Sprintf(`You are testing the misbah container manager via MCP protocol.

Task: Create a container spec at path "%s" with name "llm-mcp-container" using the misbah_container_create tool.

Generate a JSON-RPC request to call this tool. Output ONLY the JSON request, nothing else.`, specFile)

		response := queryLLMWithModel(t, model, prompt)
		response = extractJSON(response)
		t.Logf("LLM generated:\n%s", response)

		var req mcpRequest
		if err := json.Unmarshal([]byte(response), &req); err != nil {
			t.Fatalf("Failed to parse LLM response: %v", err)
		}

		result := executeMCPRequest(t, &req)
		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "Container spec created"), "Spec should be created")
	})

	// Test: LLM validates container spec
	t.Run("llm_validate_container", func(t *testing.T) {
		result := mcpCallTool(t, "misbah_container_validate", map[string]interface{}{
			"spec_path": specFile,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "valid"), "Spec should validate")
	})

	// Test: LLM inspects and explains
	t.Run("llm_inspect_and_explain", func(t *testing.T) {
		result := mcpCallTool(t, "misbah_container_inspect", map[string]interface{}{
			"spec_path": specFile,
		})
		specJSON := getToolContent(t, result)

		prompt := fmt.Sprintf(`You are reviewing a misbah container specification.

Container spec:
%s

Is this spec valid? Summarize what this container will do in one sentence.

Respond in this format:
Valid: [yes/no]
Purpose: [one sentence summary]`, specJSON)

		response := queryLLMWithModel(t, model, prompt)
		t.Logf("LLM analysis:\n%s", response)

		assert(t, strings.Contains(strings.ToLower(response), "valid"), "LLM should check validity")
	})
}

// --- Helpers ---

func detectAvailableModel(t *testing.T) string {
	t.Helper()

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
	start := strings.Index(text, "{")
	if start == -1 {
		return text
	}

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

func queryLLMWithModel(t *testing.T, model string, prompt string) string {
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
