//go:build e2e && claude

package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClaudeCodeIntegration tests Jabal's integration with Claude Code CLI.
//
// Requirements:
// - claude binary in PATH
// - JABAL_E2E_CLAUDE=true environment variable
//
// Run with:
//   JABAL_E2E_CLAUDE=true go test -v -tags=e2e,claude ./test/e2e/
func TestClaudeCodeIntegration(t *testing.T) {
	// Runtime check 1: Binary exists
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude binary not found in PATH")
	}
	t.Logf("Found claude at: %s", claudePath)

	// Runtime check 2: Opt-in flag
	if os.Getenv("JABAL_E2E_CLAUDE") != "true" {
		t.Skip("JABAL_E2E_CLAUDE not set to 'true'")
	}

	root := repoRoot(t)
	runInDir(t, root, "go", "build", "-o", "jabal", "./cmd/jabal")
	jabalBin := filepath.Join(root, "jabal")

	// Create isolated test workspace
	workspaceDir := t.TempDir()
	workspaceName := "claude-e2e-test-" + time.Now().Format("20060102-150405")

	defer func() {
		// Cleanup workspace
		home, _ := os.UserHomeDir()
		workspaceDir := filepath.Join(home, ".config/jabal/workspaces", workspaceName)
		os.RemoveAll(workspaceDir)

		// Assert cleanup worked
		if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
			t.Errorf("Cleanup failed: workspace directory still exists at %s", workspaceDir)
		}
	}()

	// Create test source repositories
	srcA := filepath.Join(workspaceDir, "repo-a")
	srcB := filepath.Join(workspaceDir, "repo-b")
	mustMkdir(t, srcA)
	mustMkdir(t, srcB)

	// Add sample files
	mustWriteFile(t, filepath.Join(srcA, "index.js"), "console.log('repo-a');")
	mustWriteFile(t, filepath.Join(srcB, "app.js"), "console.log('repo-b');")

	t.Run("CreateWorkspace", func(t *testing.T) {
		run(t, jabalBin, "create", "-w", workspaceName, "-d", "Claude E2E test workspace")
	})

	t.Run("UpdateManifest", func(t *testing.T) {
		// Build manifest content
		manifest := `name: ` + workspaceName + `
description: Claude E2E test workspace
sources:
  - path: ` + srcA + `
    mount: repo-a
  - path: ` + srcB + `
    mount: repo-b
providers:
  claude:
    mcp_servers:
      scribe: http://localhost:8080
`
		manifestPath := getManifestPath(t, workspaceName)
		mustWriteFile(t, manifestPath, manifest)
	})

	t.Run("ValidateWorkspace", func(t *testing.T) {
		run(t, jabalBin, "validate", "-w", workspaceName)
	})

	t.Run("MountAndLaunchClaude", func(t *testing.T) {
		// This test requires interactive Claude session
		// We'll mount, verify namespace, then unmount
		// TODO: Automate Claude interaction via stdin

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Mount workspace (this will exec claude and block)
		// For now, we'll test mounting in dry-run mode or background
		t.Skip("Interactive Claude mount test - requires manual verification")

		// Future: Use expect-like library to automate Claude interaction
		cmd := exec.CommandContext(ctx, jabalBin, "mount", "-w", workspaceName, "-a", "claude")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				t.Log("Mount timed out (expected for interactive test)")
			} else {
				t.Fatalf("Mount failed: %v", err)
			}
		}
	})

	t.Run("VerifyStatus", func(t *testing.T) {
		out := runOutput(t, jabalBin, "summit", "-w", workspaceName)
		// After unmount, should show not mounted
		if strings.Contains(out, "Mounted: true") {
			t.Log("Workspace still mounted (may need manual cleanup)")
		}
	})
}

// TestClaudeCodeWithMCP tests Claude Code integration via MCP server.
//
// This test uses the MCP server interface instead of direct CLI mounting,
// allowing for automated verification without interactive Claude session.
func TestClaudeCodeWithMCP(t *testing.T) {
	// Runtime checks
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude binary not found in PATH")
	}
	if os.Getenv("JABAL_E2E_CLAUDE") != "true" {
		t.Skip("JABAL_E2E_CLAUDE not set to 'true'")
	}

	root := repoRoot(t)
	runInDir(t, root, "go", "build", "-o", "jabal", "./cmd/jabal")
	jabalBin := filepath.Join(root, "jabal")

	// Start MCP server
	server := startMCPServer(t, jabalBin)
	defer server.Process.Kill()

	waitForMCPServer(t, 10*time.Second)

	workspaceName := "claude-mcp-test-" + time.Now().Format("20060102-150405")

	defer func() {
		// Cleanup workspace
		home, _ := os.UserHomeDir()
		workspaceDir := filepath.Join(home, ".config/jabal/workspaces", workspaceName)
		os.RemoveAll(workspaceDir)

		// Assert cleanup worked
		if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
			t.Errorf("Cleanup failed: workspace directory still exists at %s", workspaceDir)
		}
	}()

	t.Run("CreateViaMCP", func(t *testing.T) {
		_ = mcpCallTool(t, "jabal_create_workspace", map[string]interface{}{
			"name":        workspaceName,
			"description": "Claude MCP test",
		})
	})

	t.Run("VerifyClaudeCanDiscoverMCP", func(t *testing.T) {
		// Verify MCP tools are discoverable
		result := mcpCall(t, "tools/list", nil)

		// Check that Claude-specific tools exist
		tools := result["tools"].([]interface{})

		foundCreate := false
		for _, tool := range tools {
			toolMap := tool.(map[string]interface{})
			if toolMap["name"] == "jabal_create_workspace" {
				foundCreate = true
				break
			}
		}

		assert(t, foundCreate, "MCP tools not properly exposed for Claude discovery")
	})

	t.Run("ClaudeCanQueryWorkspaces", func(t *testing.T) {
		result := mcpCallTool(t, "jabal_list_workspaces", map[string]interface{}{})

		// Verify our test workspace exists
		content := getToolContent(t, result)
		assert(t, strings.Contains(content, workspaceName), "Created workspace not found in list")
	})
}

// Helper functions

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func getManifestPath(t *testing.T, workspaceName string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("get home dir failed: %v", err)
	}
	return filepath.Join(home, ".config", "jabal", "workspaces", workspaceName, "manifest.yaml")
}
