//go:build e2e && claude

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClaudeCodeIntegration tests Misbah's integration with Claude Code CLI.
//
// Requirements:
// - claude binary in PATH
// - MISBAH_E2E_CLAUDE=true environment variable
//
// Run with:
//
//	MISBAH_E2E_CLAUDE=true go test -v -tags=e2e,claude ./test/e2e/
func TestClaudeCodeIntegration(t *testing.T) {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude binary not found in PATH")
	}
	t.Logf("Found claude at: %s", claudePath)

	if os.Getenv("MISBAH_E2E_CLAUDE") != "true" {
		t.Skip("MISBAH_E2E_CLAUDE not set to 'true'")
	}

	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")
	runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")
	defer os.Remove(misbahBin)

	testDir := t.TempDir()
	containerName := "claude-e2e-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "claude-container.yaml")

	t.Run("CreateContainerSpec", func(t *testing.T) {
		run(t, misbahBin, "container", "create",
			"--spec", specFile,
			"--name", containerName)
	})

	t.Run("ValidateSpec", func(t *testing.T) {
		run(t, misbahBin, "container", "validate", "--spec", specFile)
	})

	t.Run("InspectSpec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "inspect", "--spec", specFile)
		if !strings.Contains(output, containerName) {
			t.Fatalf("Inspect doesn't contain container name: %s", output)
		}
	})
}

// TestClaudeCodeWithMCP tests Claude Code integration via MCP server.
func TestClaudeCodeWithMCP(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found in PATH")
	}
	if os.Getenv("MISBAH_E2E_CLAUDE") != "true" {
		t.Skip("MISBAH_E2E_CLAUDE not set to 'true'")
	}

	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")
	runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")

	server := startMCPServer(t, misbahBin)
	defer server.Process.Kill()

	waitForMCPServer(t, 10*time.Second)

	testDir := t.TempDir()
	specFile := filepath.Join(testDir, "claude-mcp-container.yaml")

	t.Run("CreateViaMCP", func(t *testing.T) {
		result := mcpCallTool(t, "misbah_container_create", map[string]interface{}{
			"spec_path": specFile,
			"name":      "claude-mcp-test",
		})
		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "Container spec created"), "Should create spec")
	})

	t.Run("VerifyClaudeCanDiscoverMCP", func(t *testing.T) {
		result := mcpCall(t, "tools/list", nil)

		tools := result["tools"].([]interface{})

		foundCreate := false
		for _, tool := range tools {
			toolMap := tool.(map[string]interface{})
			if toolMap["name"] == "misbah_container_create" {
				foundCreate = true
				break
			}
		}

		assert(t, foundCreate, "MCP tools not properly exposed for Claude discovery")
	})

	t.Run("ClaudeCanInspectContainer", func(t *testing.T) {
		result := mcpCallTool(t, "misbah_container_inspect", map[string]interface{}{
			"spec_path": specFile,
		})

		content := getToolContent(t, result)
		assert(t, strings.Contains(content, "claude-mcp-test"), "Should contain container name")
	})
}
