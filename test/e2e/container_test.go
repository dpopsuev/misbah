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

// TestContainerLifecycle tests the complete lifecycle of container operations:
// create spec, validate, inspect, start, list, stop, destroy.
func TestContainerLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Container tests require Linux")
	}

	// Build misbah binary
	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")
	runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")
	defer os.Remove(misbahBin)

	// Setup
	testDir := t.TempDir()
	containerName := "e2e-container-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "test-container.yaml")

	t.Logf("Test directory: %s", testDir)
	t.Logf("Container name: %s", containerName)
	t.Logf("Spec file: %s", specFile)

	// Test 1: Create container specification
	t.Run("create_container_spec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "create",
			"--spec", specFile,
			"--name", containerName,
			"--command", "/bin/bash,-c,echo 'Hello from container' && sleep 1")

		require.Contains(t, output, "Container specification created")
		require.Contains(t, output, specFile)
		require.FileExists(t, specFile)

		// Verify file contents
		data, err := os.ReadFile(specFile)
		require.NoError(t, err)

		content := string(data)
		require.Contains(t, content, "version: \"1.0\"")
		require.Contains(t, content, "name: "+containerName)
		require.Contains(t, content, "/bin/bash")
		require.Contains(t, content, "echo 'Hello from container'")

		// Modify spec to use tmpfs in /tmp (writable location)
		modifiedSpec := `version: "1.0"
metadata:
  name: ` + containerName + `
  description: Auto-generated container specification
  labels: {}
process:
  command:
  - /bin/bash
  - -c
  - echo 'Hello from container' && sleep 1
  env:
  - MISBAH_CONTAINER=` + containerName + `
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

	// Test 2: Validate container specification
	t.Run("validate_container_spec", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "validate", "--spec", specFile)

		require.Contains(t, output, "Container specification is valid")
		require.Contains(t, output, "Name: "+containerName)
		require.Contains(t, output, "Version: 1.0")
		require.Contains(t, output, "Namespaces: user=true, mount=true, pid=true")
	})

	// Test 3: Inspect container specification (not running)
	t.Run("inspect_container_spec_file", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "inspect", "--spec", specFile)

		require.Contains(t, output, "Container Specification:")
		require.Contains(t, output, "Version: 1.0")
		require.Contains(t, output, "Name: "+containerName)
		require.Contains(t, output, "Process:")
		require.Contains(t, output, "Namespaces:")
		require.Contains(t, output, "Mounts")
	})

	// Test 4: List containers (should be empty initially)
	t.Run("list_containers_empty", func(t *testing.T) {
		output := runOutput(t, misbahBin, "container", "list")
		// Might have other containers running, but our container shouldn't be there yet
		require.NotContains(t, output, containerName)
	})

	// Test 5: Start container (runs and exits immediately)
	t.Run("start_container", func(t *testing.T) {
		// This will execute the container and wait for it to complete
		output := runOutput(t, misbahBin, "container", "start", "--spec", specFile)

		require.Contains(t, output, "Starting container from specification")
		require.Contains(t, output, "Loaded container specification: "+containerName)
		require.Contains(t, output, "Mounting container: "+containerName)
		require.Contains(t, output, "Container "+containerName+" exited successfully")
	})

	// Test 6: Create a long-running container for testing stop/destroy
	t.Run("long_running_container", func(t *testing.T) {
		longContainerName := containerName + "-long"
		longSpecFile := filepath.Join(testDir, "long-container.yaml")

		// Create spec with long-running command and tmpfs mount
		longSpec := `version: "1.0"
metadata:
  name: ` + longContainerName + `
  description: Long-running test container
  labels: {}
process:
  command:
  - /bin/bash
  - -c
  - sleep 30
  env:
  - MISBAH_CONTAINER=` + longContainerName + `
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

		// Start container in background
		cmd := exec.Command(misbahBin, "container", "start", "--spec", longSpecFile)
		cmd.Dir = root
		require.NoError(t, cmd.Start())

		// Give it time to start
		time.Sleep(2 * time.Second)

		// // Test 6a: List should show running container
		output := runOutput(t, misbahBin, "container", "list")
		require.Contains(t, output, longContainerName)
		require.Contains(t, output, "running")

		// // Test 6b: Inspect running container
		output = runOutput(t, misbahBin, "container", "inspect", "--name", longContainerName)
		require.Contains(t, output, "Inspecting running container: "+longContainerName)
		require.Contains(t, output, "Lock Information:")
		require.Contains(t, output, "PID:")
		require.Contains(t, output, "Status: Running")

		// Test 6c: Stop container (use --force for background processes)
		output = runOutput(t, misbahBin, "container", "stop", "--name", longContainerName, "--force")
		require.Contains(t, output, "Stopping container: "+longContainerName)
		require.Contains(t, output, "force stopped")

		// Wait for container to fully stop
		time.Sleep(1 * time.Second)

		// // Test 6d: List should not show stopped container
		output = runOutput(t, misbahBin, "container", "list")
		require.NotContains(t, output, longContainerName)

		// Test 6e: Destroy container (cleanup any remaining resources)
		output = runOutput(t, misbahBin, "container", "destroy", "--name", longContainerName)
		require.Contains(t, output, "destroyed successfully")

		// Clean up background process if still running
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	// Test 7: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		// Ensure our test container is cleaned up
		_ = exec.Command(misbahBin, "container", "destroy", "--name", containerName, "--force").Run()
	})
}

// TestContainerWithResources tests container creation with resource limits.
func TestContainerWithResources(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Container tests require Linux")
	}

	root := repoRoot(t)
	misbahBin := filepath.Join(root, "misbah")

	// Check if binary exists, build if not
	if _, err := os.Stat(misbahBin); os.IsNotExist(err) {
		runInDir(t, root, "go", "build", "-o", misbahBin, "./cmd/misbah")
		defer os.Remove(misbahBin)
	}

	testDir := t.TempDir()
	containerName := "e2e-resources-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "resources-container.yaml")

	// Create container spec with resources and tmpfs mount
	specWithResources := `version: "1.0"
metadata:
  name: ` + containerName + `
  description: Test container with resource limits
  labels: {}
process:
  command:
  - /bin/bash
  - -c
  - echo test && exit 0
  env:
  - MISBAH_CONTAINER=` + containerName + `
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
	output := runOutput(t, misbahBin, "container", "validate", "--spec", specFile)
	require.Contains(t, output, "Container specification is valid")
	require.Contains(t, output, "Resources configured: memory=512MB, cpu_shares=1024")

	// Inspect spec with resources
	output = runOutput(t, misbahBin, "container", "inspect", "--spec", specFile)
	require.Contains(t, output, "Resources:")
	require.Contains(t, output, "Memory: 512MB")
	require.Contains(t, output, "CPU Shares: 1024")
	require.Contains(t, output, "IO Weight: 100")

	// Start container (will apply resource limits if cgroup v2 is available)
	output = runOutput(t, misbahBin, "container", "start", "--spec", specFile)
	require.Contains(t, output, "exited successfully")

	// Cleanup
	_ = exec.Command(misbahBin, "container", "destroy", "--name", containerName, "--force").Run()
}

// TestContainerErrorCases tests error handling for container commands.
func TestContainerErrorCases(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Container tests require Linux")
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
		cmd := exec.Command(misbahBin, "container", "validate", "--spec", "/nonexistent/file.yaml")
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "failed to load container spec")
	})

	t.Run("start_nonexistent_spec", func(t *testing.T) {
		cmd := exec.Command(misbahBin, "container", "start", "--spec", "/nonexistent/file.yaml")
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "failed to load container spec")
	})

	t.Run("stop_nonexistent_container", func(t *testing.T) {
		cmd := exec.Command(misbahBin, "container", "stop", "--name", "nonexistent-container")
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "is not running")
	})

	t.Run("inspect_without_args", func(t *testing.T) {
		cmd := exec.Command(misbahBin, "container", "inspect")
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "must provide either --spec or --name")
	})

	t.Run("invalid_container_spec", func(t *testing.T) {
		invalidSpec := filepath.Join(testDir, "invalid.yaml")
		// Create a spec with invalid version (missing required fields)
		require.NoError(t, os.WriteFile(invalidSpec, []byte(`version: "2.0"
metadata:
  name: invalid
`), 0644))

		cmd := exec.Command(misbahBin, "container", "validate", "--spec", invalidSpec)
		output, err := cmd.CombinedOutput()
		require.Error(t, err)
		require.Contains(t, string(output), "validation failed")
	})
}



