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

const (
	ollamaBaseURL = "http://localhost:11434"
	modelName     = "qwen2.5-coder:7b-instruct" // Can use qwen2.5:32b if available
)

// TestLLMAgentWorkflow tests jabal with an LLM agent (Qwen2.5-Coder via Ollama)
func TestLLMAgentWorkflow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	// Check if Ollama is available
	if !isOllamaAvailable(t) {
		t.Skip("Ollama not available, skipping LLM tests")
	}

	// Check if model is available
	if !isModelAvailable(t, modelName) {
		t.Skipf("Model %s not available in Ollama", modelName)
	}

	workspace := "llm-e2e-" + time.Now().Format("20060102-150405")
	testDir := t.TempDir()

	sourceA := filepath.Join(testDir, "project-a")
	sourceB := filepath.Join(testDir, "project-b")

	// Create test sources
	if err := os.MkdirAll(sourceA, 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	if err := os.MkdirAll(sourceB, 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}

	defer func() {
		// Cleanup workspace
		// Try unmount, ignore errors (may not be mounted)
		exec.Command("./jabal", "unmount", "-w", workspace, "--force").Run()

		// Always clean workspace directory
		workspaceDir := filepath.Join(os.Getenv("HOME"), ".config/jabal/workspaces", workspace)
		os.RemoveAll(workspaceDir)

		// Assert cleanup worked
		if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
			t.Errorf("Cleanup failed: workspace directory still exists at %s", workspaceDir)
		}
	}()

	// Test: Ask LLM to create a workspace manifest
	t.Run("llm_create_manifest", func(t *testing.T) {
		prompt := fmt.Sprintf(`You are helping test the jabal workspace manager.

Task: Create a YAML manifest for a workspace with these requirements:
- Name: %s
- Description: "LLM-generated test workspace"
- Two sources:
  1. Path: %s, Mount: project-a
  2. Path: %s, Mount: project-b
- Provider: claude with MCP server "scribe" at http://localhost:8080

Output ONLY the YAML manifest, nothing else.`, workspace, sourceA, sourceB)

		manifest := queryLLM(t, prompt)

		// Save the generated manifest
		manifestPath := filepath.Join(os.Getenv("HOME"), ".config/jabal/workspaces", workspace)
		if err := os.MkdirAll(manifestPath, 0755); err != nil {
			t.Fatalf("Failed to create workspace dir: %v", err)
		}

		manifestFile := filepath.Join(manifestPath, "manifest.yaml")
		if err := os.WriteFile(manifestFile, []byte(manifest), 0644); err != nil {
			t.Fatalf("Failed to write manifest: %v", err)
		}

		t.Logf("LLM-generated manifest:\n%s", manifest)
	})

	// Test: Validate the LLM-generated manifest
	t.Run("validate_llm_manifest", func(t *testing.T) {
		run(t, "./jabal", "validate", "-w", workspace)
	})

	// Test: Ask LLM to explain what the workspace does
	t.Run("llm_explain_workspace", func(t *testing.T) {
		manifestPath := filepath.Join(os.Getenv("HOME"), ".config/jabal/workspaces", workspace, "manifest.yaml")
		manifestContent, err := os.ReadFile(manifestPath)
		if err != nil {
			t.Fatalf("Failed to read manifest: %v", err)
		}

		prompt := fmt.Sprintf(`Given this jabal workspace manifest:

%s

Explain in one sentence what this workspace does.`, manifestContent)

		explanation := queryLLM(t, prompt)
		t.Logf("LLM explanation: %s", explanation)

		// Basic check that explanation mentions workspace
		if !strings.Contains(strings.ToLower(explanation), "workspace") {
			t.Fatalf("LLM explanation doesn't mention workspace: %s", explanation)
		}
	})
}

// --- LLM helpers ---

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func isOllamaAvailable(t *testing.T) bool {
	t.Helper()
	resp, err := http.Get(ollamaBaseURL + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func isModelAvailable(t *testing.T, model string) bool {
	t.Helper()
	resp, err := http.Get(ollamaBaseURL + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}

	for _, m := range result.Models {
		if strings.HasPrefix(m.Name, model) || strings.HasPrefix(m.Name, strings.Split(model, ":")[0]) {
			return true
		}
	}

	return false
}

func queryLLM(t *testing.T, prompt string) string {
	t.Helper()

	req := ollamaRequest{
		Model:  modelName,
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
