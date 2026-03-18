//go:build e2e && llm

package e2e_test

import (
	"bytes"
	"encoding/json"
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
	modelName     = "qwen2.5-coder:7b-instruct"
)

// TestLLMAgentWorkflow tests misbah with an LLM agent (Qwen2.5-Coder via Ollama)
func TestLLMAgentWorkflow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	if !isOllamaAvailable(t) {
		t.Skip("Ollama not available, skipping LLM tests")
	}

	if !isModelAvailable(t, modelName) {
		t.Skipf("Model %s not available in Ollama", modelName)
	}

	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")
	runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")
	defer os.Remove(misbahBin)

	testDir := t.TempDir()
	containerName := "llm-e2e-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "llm-container.yaml")

	// Test: Ask LLM to create a container spec
	t.Run("llm_create_spec", func(t *testing.T) {
		prompt := `You are helping test the misbah container manager.

Task: Create a YAML container spec with these requirements:
- Version: "1.0"
- Name: "llm-test-container"
- Command: ["/bin/sh", "-c", "echo hello"]
- Cwd: "/container/workspace"
- Namespaces: user: true, mount: true

Output ONLY the YAML, nothing else.`

		yamlSpec := queryLLM(t, prompt)
		t.Logf("LLM-generated spec:\n%s", yamlSpec)

		if err := os.WriteFile(specFile, []byte(yamlSpec), 0644); err != nil {
			t.Fatalf("Failed to write spec: %v", err)
		}
	})

	// Test: Validate the LLM-generated spec
	t.Run("validate_llm_spec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "validate", "--spec", specFile)
		t.Logf("Validation output: %s", output)
		// If LLM generated valid YAML, this should pass
		_ = output
	})

	// Test: Ask LLM to explain the container
	t.Run("llm_explain_container", func(t *testing.T) {
		specContent, err := os.ReadFile(specFile)
		if err != nil {
			t.Fatalf("Failed to read spec: %v", err)
		}

		prompt := "Given this misbah container spec:\n\n" + string(specContent) + "\n\nExplain in one sentence what this container does."

		explanation := queryLLM(t, prompt)
		t.Logf("LLM explanation: %s", explanation)
		_ = containerName
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
