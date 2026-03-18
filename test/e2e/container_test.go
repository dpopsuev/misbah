//go:build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/test/harness"
	"github.com/stretchr/testify/require"
)

// TestContainerLifecycle tests the complete lifecycle of container operations:
// create spec, validate, inspect, start, list, stop, destroy.
func TestContainerLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Container tests require Linux")
	}

	lab := harness.NewLab(t)

	testDir := t.TempDir()
	containerName := "e2e-container-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "test-container.yaml")

	t.Logf("Test directory: %s", testDir)
	t.Logf("Container name: %s", containerName)
	t.Logf("Spec file: %s", specFile)
	t.Logf("Lab locks dir: %s", lab.LocksDir())

	// Test 1: Create container specification
	t.Run("create_container_spec", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "create",
			"--spec", specFile,
			"--name", containerName,
			"--command", "/bin/bash,-c,echo 'Hello from container' && sleep 1")
		require.NoError(t, err)

		require.Contains(t, output, "Container specification created")
		require.FileExists(t, specFile)

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

	// Test 2: Validate
	t.Run("validate_container_spec", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "validate", "--spec", specFile)
		require.NoError(t, err)
		require.Contains(t, output, "Container specification is valid")
		require.Contains(t, output, "Name: "+containerName)
	})

	// Test 3: Inspect
	t.Run("inspect_container_spec_file", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "inspect", "--spec", specFile)
		require.NoError(t, err)
		require.Contains(t, output, "Container Specification:")
		require.Contains(t, output, "Name: "+containerName)
	})

	// Test 4: List (should be empty)
	t.Run("list_containers_empty", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "list")
		require.NoError(t, err)
		require.NotContains(t, output, containerName)
	})

	// Test 5: Start container (runs and exits)
	t.Run("start_container", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "start", "--spec", specFile)
		require.NoError(t, err)
		require.Contains(t, output, "Starting container from specification")
		require.Contains(t, output, "Container "+containerName+" exited successfully")
		harness.AssertNoStaleState(t, lab)
	})

	// Test 6: Long-running container lifecycle
	t.Run("long_running_container", func(t *testing.T) {
		longName := containerName + "-long"
		longSpecFile := filepath.Join(testDir, "long-container.yaml")

		longSpec := `version: "1.0"
metadata:
  name: ` + longName + `
  description: Long-running test container
  labels: {}
process:
  command:
  - sleep
  - "30"
  env:
  - MISBAH_CONTAINER=` + longName + `
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

		// Start container in background using Lab (tracked by CleanupGuard)
		cmd := lab.StartMisbah("container", "start", "--spec", longSpecFile)

		// Wait for lock to appear (instead of time.Sleep)
		probe := lab.LockProbe()
		probe.WaitForLock(t, longName, 10*time.Second)

		// 6a: Verify lock is held
		harness.AssertLockHeld(t, lab, longName)

		// 6b: List should show running container
		output, err := lab.RunMisbah("container", "list")
		require.NoError(t, err)
		require.Contains(t, output, longName)
		require.Contains(t, output, "running")

		// 6c: Inspect running container
		output, err = lab.RunMisbah("container", "inspect", "--name", longName)
		require.NoError(t, err)
		require.Contains(t, output, "Inspecting running container: "+longName)
		require.Contains(t, output, "Status: Running")

		// 6d: Stop container
		output, err = lab.RunMisbah("container", "stop", "--name", longName, "--force")
		require.NoError(t, err)
		require.Contains(t, output, "force stopped")

		// Wait for lock release (instead of time.Sleep)
		probe.WaitForRelease(t, longName, 10*time.Second)

		// 6e: List should not show stopped container
		output, err = lab.RunMisbah("container", "list")
		require.NoError(t, err)
		require.NotContains(t, output, longName)

		// 6f: Destroy
		output, err = lab.RunMisbah("container", "destroy", "--name", longName)
		require.NoError(t, err)
		require.Contains(t, output, "destroyed successfully")

		// Clean up background process
		if cmd.Process != nil {
			cmd.Process.Kill()
		}

		harness.AssertNoStaleState(t, lab)
	})
}

// TestContainerWithResources tests container creation with resource limits.
func TestContainerWithResources(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Container tests require Linux")
	}

	lab := harness.NewLab(t)

	testDir := t.TempDir()
	containerName := "e2e-resources-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "resources-container.yaml")

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

	output, err := lab.RunMisbah("container", "validate", "--spec", specFile)
	require.NoError(t, err)
	require.Contains(t, output, "Container specification is valid")
	require.Contains(t, output, "Resources configured: memory=512MB, cpu_shares=1024")

	output, err = lab.RunMisbah("container", "inspect", "--spec", specFile)
	require.NoError(t, err)
	require.Contains(t, output, "Resources:")
	require.Contains(t, output, "Memory: 512MB")

	output, err = lab.RunMisbah("container", "start", "--spec", specFile)
	require.NoError(t, err)
	require.Contains(t, output, "exited successfully")

	harness.AssertNoStaleState(t, lab)
}

// TestContainerErrorCases tests error handling for container commands.
func TestContainerErrorCases(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Container tests require Linux")
	}

	lab := harness.NewLab(t)
	testDir := t.TempDir()

	t.Run("validate_nonexistent_spec", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "validate", "--spec", "/nonexistent/file.yaml")
		require.Error(t, err)
		require.Contains(t, output, "failed to load container spec")
	})

	t.Run("start_nonexistent_spec", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "start", "--spec", "/nonexistent/file.yaml")
		require.Error(t, err)
		require.Contains(t, output, "failed to load container spec")
	})

	t.Run("stop_nonexistent_container", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "stop", "--name", "nonexistent-container")
		require.Error(t, err)
		require.Contains(t, output, "is not running")
	})

	t.Run("inspect_without_args", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "inspect")
		require.Error(t, err)
		require.Contains(t, output, "must provide either --spec or --name")
	})

	t.Run("invalid_container_spec", func(t *testing.T) {
		invalidSpec := filepath.Join(testDir, "invalid.yaml")
		require.NoError(t, os.WriteFile(invalidSpec, []byte(`version: "2.0"
metadata:
  name: invalid
`), 0644))

		output, err := lab.RunMisbah("container", "validate", "--spec", invalidSpec)
		require.Error(t, err)
		require.Contains(t, output, "validation failed")
	})
}
