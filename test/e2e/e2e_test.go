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

// TestBasicWorkflow tests the container spec lifecycle via CLI
func TestBasicWorkflow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")
	runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")
	defer os.Remove(misbahBin)

	testDir := t.TempDir()
	containerName := "e2e-basic-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "test-container.yaml")

	t.Run("create_spec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "create",
			"--spec", specFile,
			"--name", containerName)
		if !strings.Contains(output, "Container specification created") {
			t.Fatalf("Unexpected output: %s", output)
		}
		if _, err := os.Stat(specFile); os.IsNotExist(err) {
			t.Fatal("Spec file not created")
		}
	})

	t.Run("validate_spec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "validate", "--spec", specFile)
		if !strings.Contains(output, "valid") {
			t.Fatalf("Validation failed: %s", output)
		}
	})

	t.Run("inspect_spec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "inspect", "--spec", specFile)
		if !strings.Contains(output, containerName) {
			t.Fatalf("Inspect doesn't contain container name: %s", output)
		}
	})

	t.Run("version", func(t *testing.T) {
		output := runOutput(t, misbahBin, "version")
		if !strings.Contains(output, "misbah version") {
			t.Fatalf("Unexpected version output: %s", output)
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
