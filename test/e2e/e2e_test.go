//go:build e2e

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	testImage     = "misbah-e2e-test"
	testContainer = "misbah-e2e-test"
)

// TestBasicWorkflow tests the complete workspace lifecycle without LLM
func TestBasicWorkflow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	// Build misbah binary
	root := repoRoot(t)
	runInDir(t, root, "go", "build", "-o", "misbah", "./cmd/misbah")

	// Setup
	workspace := "e2e-test-" + time.Now().Format("20060102-150405")
	testDir := t.TempDir()

	sourceA := filepath.Join(testDir, "source-a")
	sourceB := filepath.Join(testDir, "source-b")

	if err := os.MkdirAll(sourceA, 0755); err != nil {
		t.Fatalf("Failed to create source-a: %v", err)
	}
	if err := os.MkdirAll(sourceB, 0755); err != nil {
		t.Fatalf("Failed to create source-b: %v", err)
	}

	// Write test files
	if err := os.WriteFile(filepath.Join(sourceA, "test.txt"), []byte("test-a"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceB, "test.txt"), []byte("test-b"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	jabalBin := filepath.Join(root, "misbah")

	defer func() {
		// Cleanup workspace
		// Try unmount, ignore errors (may not be mounted)
		exec.Command(jabalBin, "unmount", "-w", workspace, "--force").Run()

		// Always clean workspace directory
		workspaceDir := filepath.Join(os.Getenv("HOME"), ".config/misbah/workspaces", workspace)
		os.RemoveAll(workspaceDir)

		// Assert cleanup worked
		if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
			t.Errorf("Cleanup failed: workspace directory still exists at %s", workspaceDir)
		}
	}()

	t.Run("create_workspace", func(t *testing.T) {
		run(t, jabalBin, "create", "-w", workspace, "--description", "E2E test workspace")
	})

	t.Run("edit_manifest", func(t *testing.T) {
		manifestPath := filepath.Join(os.Getenv("HOME"), ".config/misbah/workspaces", workspace, "manifest.yaml")
		manifest := `name: ` + workspace + `
description: E2E test workspace
sources:
  - path: ` + sourceA + `
    mount: source-a
  - path: ` + sourceB + `
    mount: source-b
providers:
  claude:
    mcp_servers:
      scribe: http://localhost:8080
`
		if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
			t.Fatalf("Failed to write manifest: %v", err)
		}
	})

	t.Run("validate_manifest", func(t *testing.T) {
		run(t, jabalBin, "validate", "-w", workspace)
	})

	t.Run("list_workspaces", func(t *testing.T) {
		out := runOutput(t, jabalBin, "peaks")
		if !strings.Contains(out, workspace) {
			t.Fatalf("Workspace not found in peaks output")
		}
	})

	t.Run("check_status", func(t *testing.T) {
		out := runOutput(t, jabalBin, "summit", "-w", workspace)
		if !strings.Contains(out, "Not mounted") {
			t.Fatalf("Unexpected status: %s", out)
		}
	})
}

// TestContainerizedWorkflow tests misbah inside a container
func TestContainerizedWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping container tests in short mode")
	}

	t.Run("build_image", func(t *testing.T) {
		buildImage(t)
	})

	defer stopContainer(t)

	t.Run("version", func(t *testing.T) {
		out, err := exec.Command("podman", "run", "--rm", testImage, "version").CombinedOutput()
		if err != nil {
			t.Fatalf("version command failed: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "misbah version") {
			t.Fatalf("Unexpected version output: %s", out)
		}
	})
}

// --- helpers ---

func buildImage(t *testing.T) {
	t.Helper()
	root := repoRoot(t)
	start := time.Now()
	run(t, "podman", "build", "-t", testImage, "-f", filepath.Join(root, "Dockerfile.test"), root)
	t.Logf("image built in %s", time.Since(start).Round(time.Millisecond))
}

func stopContainer(t *testing.T) {
	t.Helper()
	exec.Command("podman", "rm", "-f", testContainer).Run()
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v", err)
	}
	mod := strings.TrimSpace(string(out))
	if mod == "" {
		t.Fatal("not inside a Go module")
	}
	return filepath.Dir(mod)
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func runOutput(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func runInDir(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v failed in %s: %v\n%s", name, args, dir, err, out)
	}
}
