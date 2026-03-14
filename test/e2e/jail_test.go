//go:build e2e

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestJailHappyPath tests the complete end-user workflow for jail commands.
func TestJailHappyPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Jail tests require Linux")
	}

	// Build misbah binary
	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")
	runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")
	defer os.Remove(misbahBin)

	// Setup
	testDir := t.TempDir()
	jailName := "e2e-jail-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "test-jail.yaml")

	t.Logf("Test directory: %s", testDir)
	t.Logf("Jail name: %s", jailName)
	t.Logf("Spec file: %s", specFile)

	// Test 1: Create jail specification
	t.Run("create_jail_spec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "jail", "create",
			"--spec", specFile,
			"--name", jailName,
			"--command", "/bin/bash,-c,echo 'Hello from jail' && sleep 1")

		require.Contains(t, output, "Jail specification created")
		require.Contains(t, output, specFile)
		require.FileExists(t, specFile)

		// Verify file contents
		data, err := os.ReadFile(specFile)
		require.NoError(t, err)

		content := string(data)
		require.Contains(t, content, "version: \"1.0\"")
		require.Contains(t, content, "name: "+jailName)
		require.Contains(t, content, "/bin/bash")
		require.Contains(t, content, "echo 'Hello from jail'")

		// Modify spec to use tmpfs in /tmp (writable location)
		modifiedSpec := `version: "1.0"
metadata:
  name: ` + jailName + `
  description: Auto-generated jail specification
  labels: {}
process:
  command:
  - /bin/bash
  - -c
  - echo 'Hello from jail' && sleep 1
  env:
  - MISBAH_JAIL=` + jailName + `
  cwd: /tmp/workspace
namespaces:
  user: true
  mount: true
  pid: true
mounts:
- type: tmpfs
  destination: /tmp/workspace
  options:
  - rw
`
		require.NoError(t, os.WriteFile(specFile, []byte(modifiedSpec), 0644))
	})

	// Test 2: Validate jail specification
	t.Run("validate_jail_spec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "jail", "validate", "--spec", specFile)

		require.Contains(t, output, "Jail specification is valid")
		require.Contains(t, output, "Name: "+jailName)
		require.Contains(t, output, "Version: 1.0")
		require.Contains(t, output, "Namespaces: user=true, mount=true, pid=true")
	})

	// Test 3: Inspect jail specification (not running)
	t.Run("inspect_jail_spec_file", func(t *testing.T) {
		output := runOutput(t, misbahBin, "jail", "inspect", "--spec", specFile)

		require.Contains(t, output, "Jail Specification:")
		require.Contains(t, output, "Version: 1.0")
		require.Contains(t, output, "Name: "+jailName)
		require.Contains(t, output, "Process:")
		require.Contains(t, output, "Namespaces:")
		require.Contains(t, output, "Mounts")
	})

	// Test 4: List jails (should be empty initially)
	t.Run("list_jails_empty", func(t *testing.T) {
		output := runOutput(t, misbahBin, "jail", "list")
		// Might have other jails running, but our jail shouldn't be there yet
		require.NotContains(t, output, jailName)
	})

	// Test 5: Start jail (runs and exits immediately)
	t.Run("start_jail", func(t *testing.T) {
		// This will execute the jail and wait for it to complete
		output := runOutput(t, misbahBin, "jail", "start", "--spec", specFile)

		require.Contains(t, output, "Starting jail from specification")
		require.Contains(t, output, "Loaded jail specification: "+jailName)
		require.Contains(t, output, "Mounting jail: "+jailName)
		require.Contains(t, output, "Jail "+jailName+" exited successfully")
	})

	// Test 6: Create a long-running jail for testing stop/destroy
	t.Run("long_running_jail", func(t *testing.T) {
		longJailName := jailName + "-long"
		longSpecFile := filepath.Join(testDir, "long-jail.yaml")

		// Create spec with long-running command and tmpfs mount
		longSpec := `version: "1.0"
metadata:
  name: ` + longJailName + `
  description: Long-running test jail
  labels: {}
process:
  command:
  - /bin/bash
  - -c
  - sleep 30
  env:
  - MISBAH_JAIL=` + longJailName + `
  cwd: /tmp/workspace
namespaces:
  user: true
  mount: true
  pid: true
mounts:
- type: tmpfs
  destination: /tmp/workspace
  options:
  - rw
`
		require.NoError(t, os.WriteFile(longSpecFile, []byte(longSpec), 0644))

		// Start jail in background
		cmd := exec.Command(misbahBin, "jail", "start", "--spec", longSpecFile)
		cmd.Dir = root
		require.NoError(t, cmd.Start())

		// Give it time to start
		time.Sleep(2 * time.Second)

		// Test 6a: List should show running jail
		output := runOutput(t, misbahBin, "jail", "list")
		require.Contains(t, output, longJailName)
		require.Contains(t, output, "running")

		// Test 6b: Inspect running jail
		output = runOutput(t, misbahBin, "jail", "inspect", "--name", longJailName)
		require.Contains(t, output, "Inspecting running jail: "+longJailName)
		require.Contains(t, output, "Lock Information:")
		require.Contains(t, output, "PID:")
		require.Contains(t, output, "Status: Running")

		// Test 6c: Stop jail (use --force for background processes)
		output = runOutput(t, misbahBin, "jail", "stop", "--name", longJailName, "--force")
		require.Contains(t, output, "Stopping jail: "+longJailName)
		require.Contains(t, output, "force stopped")

		// Wait for jail to fully stop
		time.Sleep(1 * time.Second)

		// Test 6d: List should not show stopped jail
		output = runOutput(t, misbahBin, "jail", "list")
		require.NotContains(t, output, longJailName)

		// Test 6e: Destroy jail (cleanup any remaining resources)
		output = runOutput(t, misbahBin, "jail", "destroy", "--name", longJailName)
		require.Contains(t, output, "destroyed successfully")

		// Clean up background process if still running
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	// Test 7: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		// Ensure our test jail is cleaned up
		_ = exec.Command(misbahBin, "jail", "destroy", "--name", jailName, "--force").Run()
	})
}

// TestJailWithResources tests jail creation with resource limits.
func TestJailWithResources(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Jail tests require Linux")
	}

	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")

	// Check if binary exists, build if not
	if _, err := os.Stat(misbahBin); os.IsNotExist(err) {
		runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")
		defer os.Remove(misbahBin)
	}

	testDir := t.TempDir()
	jailName := "e2e-resources-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "resources-jail.yaml")

	// Create jail spec with resources and tmpfs mount
	specWithResources := `version: "1.0"
metadata:
  name: ` + jailName + `
  description: Test jail with resource limits
  labels: {}
process:
  command:
  - /bin/bash
  - -c
  - echo test && exit 0
  env:
  - MISBAH_JAIL=` + jailName + `
  cwd: /tmp/workspace
namespaces:
  user: true
  mount: true
  pid: true
mounts:
- type: tmpfs
  destination: /tmp/workspace
  options:
  - rw
resources:
  memory: 512MB
  cpu_shares: 1024
  io_weight: 100
`
	require.NoError(t, os.WriteFile(specFile, []byte(specWithResources), 0644))

	// Validate with resources
	output := runOutput(t, misbahBin, "jail", "validate", "--spec", specFile)
	require.Contains(t, output, "Jail specification is valid")
	require.Contains(t, output, "Resources configured: memory=512MB, cpu_shares=1024")

	// Inspect spec with resources
	output = runOutput(t, misbahBin, "jail", "inspect", "--spec", specFile)
	require.Contains(t, output, "Resources:")
	require.Contains(t, output, "Memory: 512MB")
	require.Contains(t, output, "CPU Shares: 1024")
	require.Contains(t, output, "IO Weight: 100")

	// Start jail (will apply resource limits if cgroup v2 is available)
	output = runOutput(t, misbahBin, "jail", "start", "--spec", specFile)
	require.Contains(t, output, "exited successfully")

	// Cleanup
	_ = exec.Command(misbahBin, "jail", "destroy", "--name", jailName, "--force").Run()
}

// TestJailErrorCases tests error handling for jail commands.
func TestJailErrorCases(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Jail tests require Linux")
	}

	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")

	// Check if binary exists
	if _, err := os.Stat(misbahBin); os.IsNotExist(err) {
		runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")
		defer os.Remove(misbahBin)
	}

	testDir := t.TempDir()

	t.Run("validate_nonexistent_spec", func(t *testing.T) {
		cmd := exec.Command(misbahBin, "jail", "validate", "--spec", "/nonexistent/file.yaml")
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "failed to load jail spec")
	})

	t.Run("start_nonexistent_spec", func(t *testing.T) {
		cmd := exec.Command(misbahBin, "jail", "start", "--spec", "/nonexistent/file.yaml")
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "failed to load jail spec")
	})

	t.Run("stop_nonexistent_jail", func(t *testing.T) {
		cmd := exec.Command(misbahBin, "jail", "stop", "--name", "nonexistent-jail")
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "is not running")
	})

	t.Run("inspect_without_args", func(t *testing.T) {
		cmd := exec.Command(misbahBin, "jail", "inspect")
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "must provide either --spec or --name")
	})

	t.Run("invalid_jail_spec", func(t *testing.T) {
		invalidSpec := filepath.Join(testDir, "invalid.yaml")
		// Create a spec with invalid version (missing required fields)
		require.NoError(t, os.WriteFile(invalidSpec, []byte(`version: "2.0"
metadata:
  name: invalid
`), 0644))

		cmd := exec.Command(misbahBin, "jail", "validate", "--spec", invalidSpec)
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "validation failed")
	})
}



