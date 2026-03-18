//go:build e2e

package e2e_test

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/test/harness"
)

// TestBasicWorkflow tests the container spec lifecycle via CLI
func TestBasicWorkflow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	lab := harness.NewLab(t)

	testDir := t.TempDir()
	containerName := "e2e-basic-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "test-container.yaml")

	t.Run("create_spec", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "create",
			"--spec", specFile,
			"--name", containerName)
		if err != nil {
			t.Fatalf("create failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, "Container specification created") {
			t.Fatalf("Unexpected output: %s", output)
		}
	})

	t.Run("validate_spec", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "validate", "--spec", specFile)
		if err != nil {
			t.Fatalf("validate failed: %v\n%s", err, output)
		}
	})

	t.Run("inspect_spec", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "inspect", "--spec", specFile)
		if err != nil {
			t.Fatalf("inspect failed: %v\n%s", err, output)
		}
		if !strings.Contains(output, containerName) {
			t.Fatalf("Inspect doesn't contain container name: %s", output)
		}
	})

	t.Run("version", func(t *testing.T) {
		output, err := lab.RunMisbah("version")
		if err != nil {
			t.Fatalf("version failed: %v\n%s", err, output)
		}
	})

	harness.AssertNoStaleState(t, lab)
}

// --- helpers ---

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
