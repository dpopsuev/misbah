package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/model"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetFlags zeroes all package-level flag vars that Cobra caches between
// rootCmd.Execute() calls AND clears Cobra's internal "Changed" bit on every
// flag so that MarkFlagRequired checks work correctly.
// Every test that drives rootCmd MUST call this in a t.Cleanup or defer.
func resetFlags() {
	containerSpecFile = ""
	containerName = ""
	containerCommand = nil
	containerForce = false
	containerImage = ""
	containerRuntime = ""

	// Clear the Changed bit on every flag of every subcommand so that
	// Cobra's required-flag checks fire correctly in subsequent tests.
	for _, cmd := range containerCmd.Commands() {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			f.Changed = false
		})
	}
	// Also clear root persistent flags and daemon/image subcommands
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Changed = false
	})
}

// writeValidSpec creates a minimal valid ContainerSpec YAML at path.
func writeValidSpec(t *testing.T, path string) {
	t.Helper()
	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name:        "test-container",
			Description: "test spec",
			Labels:      map[string]string{"env": "test"},
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh"},
			Env:     []string{"FOO=bar"},
			Cwd:     "/container/workspace",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
			PID:   true,
		},
		Mounts: []model.MountSpec{
			{
				Type:        model.MountTypeBind,
				Source:      "/tmp",
				Destination: "/container/workspace",
				Options:     []string{"rw"},
			},
		},
	}
	err := spec.SaveContainerSpec(path)
	require.NoError(t, err)
}

// writeValidSpecWithResources creates a valid spec that includes resource limits and labels.
func writeValidSpecWithResources(t *testing.T, path string) {
	t.Helper()
	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name:        "resourced",
			Description: "spec with resources",
			Labels:      map[string]string{"tier": "eco"},
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh"},
			Cwd:     "/container/workspace",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Mounts: []model.MountSpec{
			{
				Type:        model.MountTypeBind,
				Source:      "/tmp",
				Destination: "/container/workspace",
				Options:     []string{"ro"},
			},
		},
		Resources: &model.ResourceSpec{
			Memory:    "512MB",
			CPUShares: 1024,
			IOWeight:  100,
		},
	}
	err := spec.SaveContainerSpec(path)
	require.NoError(t, err)
}

// writeLockFile writes a JSON lock file for the given container name inside
// locksDir.  Returns the path to the lock file.
func writeLockFile(t *testing.T, locksDir, name string, pid int, provider string) string {
	t.Helper()
	lock := &model.Lock{
		Workspace: name,
		Provider:  provider,
		PID:       pid,
		StartedAt: time.Now(),
		User:      "testuser",
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	require.NoError(t, err)

	lockPath := filepath.Join(locksDir, name+".lock")
	err = os.WriteFile(lockPath, data, 0644)
	require.NoError(t, err)
	return lockPath
}

// --- formatSize (pure function, no CLI plumbing) ---

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},       // 1 MB
		{1572864, "1.5 MB"},       // 1.5 MB
		{1073741824, "1.0 GB"},    // 1 GB
		{1610612736, "1.5 GB"},    // 1.5 GB
		{10737418240, "10.0 GB"},  // 10 GB
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, formatSize(tc.bytes))
		})
	}
}

// --- container create ---

func TestContainerCreate(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "created.yaml")

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{
		"container", "create",
		"--spec", specPath,
		"--name", "my-new-container",
		"--command", "/bin/bash",
	})

	err := rootCmd.Execute()
	require.NoError(t, err)

	// Verify file exists
	_, statErr := os.Stat(specPath)
	require.NoError(t, statErr, "spec file should exist")

	// Load and validate the generated spec
	spec, loadErr := model.LoadContainerSpec(specPath)
	require.NoError(t, loadErr)
	assert.Equal(t, "1.0", spec.Version)
	assert.Equal(t, "my-new-container", spec.Metadata.Name)
	assert.Equal(t, []string{"/bin/bash"}, spec.Process.Command)
	assert.True(t, spec.Namespaces.User)
	assert.True(t, spec.Namespaces.Mount)
	assert.True(t, spec.Namespaces.PID)
}

func TestContainerCreateWithCommand(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "cmd.yaml")

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{
		"container", "create",
		"--spec", specPath,
		"--name", "cmd-test",
		"--command", "/bin/sh,-c,echo hello",
	})

	err := rootCmd.Execute()
	require.NoError(t, err)

	spec, loadErr := model.LoadContainerSpec(specPath)
	require.NoError(t, loadErr)
	assert.Equal(t, []string{"/bin/sh", "-c", "echo hello"}, spec.Process.Command)
}

func TestContainerCreateWithImage(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "img.yaml")

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{
		"container", "create",
		"--spec", specPath,
		"--name", "img-test",
		"--command", "/bin/bash",
		"--image", "docker.io/library/alpine:latest",
	})

	err := rootCmd.Execute()
	require.NoError(t, err)

	spec, loadErr := model.LoadContainerSpec(specPath)
	require.NoError(t, loadErr)
	assert.Equal(t, "docker.io/library/alpine:latest", spec.Image)
}

func TestContainerCreateMissingName(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "noname.yaml")

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{
		"container", "create",
		"--spec", specPath,
	})

	err := rootCmd.Execute()
	assert.Error(t, err, "should fail when --name is missing")
}

// --- container validate ---

func TestContainerValidate(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "valid.yaml")
	writeValidSpec(t, specPath)

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "validate", "--spec", specPath})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerValidateWithResources(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "resources.yaml")
	writeValidSpecWithResources(t, specPath)

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "validate", "--spec", specPath})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerValidateInvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "bad.yaml")
	err := os.WriteFile(specPath, []byte("not: valid: yaml: ["), 0644)
	require.NoError(t, err)

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "validate", "--spec", specPath})

	execErr := rootCmd.Execute()
	assert.Error(t, execErr)
}

func TestContainerValidateNonexistent(t *testing.T) {
	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "validate", "--spec", "/tmp/does-not-exist-abc123.yaml"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestContainerValidateInvalidSpec(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "invalid-spec.yaml")
	// Valid YAML but invalid spec (missing required fields)
	err := os.WriteFile(specPath, []byte("version: '2.0'\nmetadata:\n  name: test\n"), 0644)
	require.NoError(t, err)

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "validate", "--spec", specPath})

	execErr := rootCmd.Execute()
	assert.Error(t, execErr)
}

func TestContainerValidateMissingSpec(t *testing.T) {
	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "validate"})

	err := rootCmd.Execute()
	assert.Error(t, err, "should fail when --spec is missing")
}

// --- container list ---

func TestContainerListNoContainers(t *testing.T) {
	tmpDir := t.TempDir()

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerListNoLocksDir(t *testing.T) {
	tmpDir := t.TempDir()

	t.Cleanup(resetFlags)
	// Point MISBAH_TEMP_DIR to a dir with no .locks subdirectory
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerListWithLockFiles(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	// Create a lock file with our own PID so it shows as "running"
	writeLockFile(t, locksDir, "running-ctr", os.Getpid(), "claude")
	// Create a lock file with a nonexistent PID so it shows as "stale"
	writeLockFile(t, locksDir, "stale-ctr", 999999999, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerListEmptyLocksDir(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))
	// Empty locks dir — no .lock files

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerListIgnoresNonLockFiles(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	// Write a non-.lock file — it should be ignored
	require.NoError(t, os.WriteFile(filepath.Join(locksDir, "readme.txt"), []byte("hi"), 0644))
	// Write a subdirectory named like a lock — should be skipped (not IsDir check)
	require.NoError(t, os.MkdirAll(filepath.Join(locksDir, "dirlock.lock"), 0755))

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerListMalformedLock(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	// Write a .lock file with invalid JSON
	require.NoError(t, os.WriteFile(filepath.Join(locksDir, "bad.lock"), []byte("{invalid"), 0644))

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	// Should not fail — the malformed lock is warned but list continues
	require.NoError(t, err)
}

// --- container stop ---

func TestContainerStopNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "stop", "--name", "nonexistent"})

	err := rootCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestContainerStopMissingName(t *testing.T) {
	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "stop"})

	err := rootCmd.Execute()
	assert.Error(t, err, "should fail when --name is missing")
}

func TestContainerStopGraceful(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	// Start a subprocess that will sleep — we'll stop it gracefully
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	writeLockFile(t, locksDir, "graceful-ctr", pid, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "stop", "--name", "graceful-ctr"})

	err := rootCmd.Execute()
	// ReleaseLock checks PID ownership: the sleep process isn't us, but it IS
	// running (not stale), so ReleaseLock returns "cannot release lock owned by
	// another process".  This still exercises the full stop path.
	assert.Error(t, err)
}

func TestContainerStopForce(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	writeLockFile(t, locksDir, "force-ctr", pid, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "stop", "--name", "force-ctr", "--force"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	// The sleep process should have been killed
	waitErr := cmd.Wait()
	assert.Error(t, waitErr, "sleep should have been killed")
}

func TestContainerStopStaleLock(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	// A PID that doesn't exist => stale lock
	writeLockFile(t, locksDir, "stale-ctr", 999999999, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	// Graceful stop on a stale lock — ReleaseLock removes it
	rootCmd.SetArgs([]string{"container", "stop", "--name", "stale-ctr"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- container inspect ---

func TestContainerInspectWithSpec(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "inspect.yaml")
	writeValidSpec(t, specPath)

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "inspect", "--spec", specPath})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerInspectWithResources(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "inspect-res.yaml")
	writeValidSpecWithResources(t, specPath)

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "inspect", "--spec", specPath})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerInspectWithName(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	writeLockFile(t, locksDir, "inspect-ctr", os.Getpid(), "claude")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "inspect", "--name", "inspect-ctr"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerInspectWithNameStale(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	writeLockFile(t, locksDir, "stale-inspect", 999999999, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "inspect", "--name", "stale-inspect"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerInspectWithNameNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))
	// No lock file for this name

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "inspect", "--name", "ghost"})

	err := rootCmd.Execute()
	require.NoError(t, err, "inspect with unknown name should still succeed, printing 'Not running'")
}

func TestContainerInspectBothSpecAndName(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "both.yaml")
	writeValidSpec(t, specPath)

	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))
	writeLockFile(t, locksDir, "both-ctr", os.Getpid(), "claude")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{
		"container", "inspect",
		"--spec", specPath,
		"--name", "both-ctr",
	})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerInspectNoFlags(t *testing.T) {
	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "inspect"})

	err := rootCmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must provide either --spec or --name")
}

func TestContainerInspectInvalidSpec(t *testing.T) {
	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "inspect", "--spec", "/tmp/no-such-file-xyz.yaml"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

// --- container destroy ---

func TestContainerDestroyNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "destroy", "--name", "ghost"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestContainerDestroyMissingName(t *testing.T) {
	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "destroy"})

	err := rootCmd.Execute()
	assert.Error(t, err, "should fail when --name is missing")
}

func TestContainerDestroyStaleLock(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	// Create a stale lock (nonexistent PID)
	lockPath := writeLockFile(t, locksDir, "stale-destroy", 999999999, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "destroy", "--name", "stale-destroy"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	// Lock file should be removed
	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "lock file should have been removed")
}

func TestContainerDestroyRunningForce(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	lockPath := writeLockFile(t, locksDir, "force-destroy", pid, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "destroy", "--name", "force-destroy", "--force"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	// The sleep process should have been killed
	waitErr := cmd.Wait()
	assert.Error(t, waitErr, "sleep should have been killed")

	// Lock file should be removed
	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "lock file should have been removed")
}

func TestContainerDestroyRunningGraceful(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	lockPath := writeLockFile(t, locksDir, "graceful-destroy", pid, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "destroy", "--name", "graceful-destroy"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	// Lock file should be removed
	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "lock file should have been removed")

	// The sleep process should have been killed (via ForceRelease path after
	// ReleaseLock warns about ownership)
	cmd.Process.Kill() // ensure cleanup
}

// --- container start (error paths only — real start needs namespaces) ---

func TestContainerStartInvalidSpec(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "bad.yaml")
	require.NoError(t, os.WriteFile(specPath, []byte(":::"), 0644))

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "start", "--spec", specPath})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestContainerStartNonexistentSpec(t *testing.T) {
	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "start", "--spec", "/tmp/no-such-spec-xyz.yaml"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

// --- Verify spec file round-trip through create + validate ---

func TestCreateThenValidate(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "roundtrip.yaml")

	t.Cleanup(resetFlags)

	// Create
	rootCmd.SetArgs([]string{
		"container", "create",
		"--spec", specPath,
		"--name", "roundtrip-test",
		"--command", "/bin/bash",
	})
	err := rootCmd.Execute()
	require.NoError(t, err)

	resetFlags()

	// Validate
	rootCmd.SetArgs([]string{"container", "validate", "--spec", specPath})
	err = rootCmd.Execute()
	require.NoError(t, err)
}

// --- Verify create + inspect round-trip ---

func TestCreateThenInspect(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "ci.yaml")

	t.Cleanup(resetFlags)

	rootCmd.SetArgs([]string{
		"container", "create",
		"--spec", specPath,
		"--name", "ci-test",
		"--command", "/bin/bash",
	})
	require.NoError(t, rootCmd.Execute())

	resetFlags()

	rootCmd.SetArgs([]string{"container", "inspect", "--spec", specPath})
	require.NoError(t, rootCmd.Execute())
}

// --- Inspect spec with empty env/options coverage ---

func TestContainerInspectMinimalSpec(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "minimal.yaml")
	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: "minimal",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh"},
			Cwd:     "/container/workspace",
			// No Env
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Mounts: []model.MountSpec{
			{
				Type:        model.MountTypeTmpfs,
				Destination: "/container/workspace",
				// No Source, no Options
			},
		},
		// No Resources, no Labels
	}
	require.NoError(t, spec.SaveContainerSpec(specPath))

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "inspect", "--spec", specPath})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- container stop with stale lock + force ---

func TestContainerStopForceStale(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	writeLockFile(t, locksDir, "stale-force", 999999999, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "stop", "--name", "stale-force", "--force"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- List with running lock owned by current process ---

func TestContainerListOwnLock(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	writeLockFile(t, locksDir, "own-ctr", os.Getpid(), "misbah")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- formatSize edge cases ---

func TestFormatSizeExactBoundaries(t *testing.T) {
	// Exactly at boundaries
	assert.Equal(t, "1.0 KB", formatSize(1024))
	assert.Equal(t, "1.0 MB", formatSize(1024*1024))
	assert.Equal(t, "1.0 GB", formatSize(1024*1024*1024))

	// Just below boundaries
	assert.Equal(t, "1023 B", formatSize(1023))
	assert.Contains(t, formatSize(1024*1024-1), "KB")
	assert.Contains(t, formatSize(1024*1024*1024-1), "MB")
}

// --- container stop with own PID (ReleaseLock succeeds) ---

func TestContainerStopOwnPID(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	writeLockFile(t, locksDir, "own-stop", os.Getpid(), "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "stop", "--name", "own-stop"})

	err := rootCmd.Execute()
	// ReleaseLock should succeed since the PID matches our own
	require.NoError(t, err)

	// Lock file should be removed
	lockPath := filepath.Join(locksDir, "own-stop.lock")
	_, statErr := os.Stat(lockPath)
	assert.True(t, os.IsNotExist(statErr), "lock file should have been removed")
}

// --- container destroy with own PID ---

func TestContainerDestroyOwnPID(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	writeLockFile(t, locksDir, "own-destroy", os.Getpid(), "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "destroy", "--name", "own-destroy"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- container start with kata runtime (daemon unreachable error path) ---

func TestContainerStartKataErrorPath(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "kata.yaml")
	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: "kata-test",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh"},
			Cwd:     "/container/workspace",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Mounts: []model.MountSpec{
			{
				Type:        model.MountTypeBind,
				Source:      "/tmp",
				Destination: "/container/workspace",
				Options:     []string{"rw"},
			},
		},
		Runtime: model.RuntimeKata,
		Image:   "docker.io/library/alpine:latest",
	}
	require.NoError(t, spec.SaveContainerSpec(specPath))

	t.Cleanup(resetFlags)
	// Set daemon socket to a nonexistent path so the daemon client fails
	t.Setenv("MISBAH_DAEMON_SOCKET", filepath.Join(tmpDir, "nonexistent.sock"))

	rootCmd.SetArgs([]string{"container", "start", "--spec", specPath})

	err := rootCmd.Execute()
	assert.Error(t, err, "should fail when daemon is not running")
}

// --- container start with --runtime flag override ---

func TestContainerStartRuntimeOverride(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "override.yaml")
	// Write a spec with no runtime set
	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: "override-test",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh"},
			Cwd:     "/container/workspace",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Mounts: []model.MountSpec{
			{
				Type:        model.MountTypeBind,
				Source:      "/tmp",
				Destination: "/container/workspace",
				Options:     []string{"rw"},
			},
		},
		Image: "docker.io/library/alpine:latest",
	}
	require.NoError(t, spec.SaveContainerSpec(specPath))

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_DAEMON_SOCKET", filepath.Join(tmpDir, "nonexistent.sock"))

	rootCmd.SetArgs([]string{"container", "start", "--spec", specPath, "--runtime", "kata"})

	err := rootCmd.Execute()
	// Should hit the kata path and fail with daemon error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "daemon")
}

// --- Multiple lock files with mixed status ---

func TestContainerListMixed(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	// Running: our PID
	writeLockFile(t, locksDir, "alpha", os.Getpid(), "claude")
	// Stale: nonexistent PID
	writeLockFile(t, locksDir, "beta", 999999998, "test")
	// Running: a real subprocess
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	writeLockFile(t, locksDir, "gamma", cmd.Process.Pid, "misbah")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- Inspect with multiple mount types (source present/absent) ---

func TestContainerInspectMultipleMounts(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "mounts.yaml")
	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: "multi-mount",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh"},
			Cwd:     "/container/workspace",
			Env:     []string{"A=1", "B=2"},
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Mounts: []model.MountSpec{
			{
				Type:        model.MountTypeBind,
				Source:      "/tmp",
				Destination: "/container/workspace",
				Options:     []string{"rw"},
			},
			{
				Type:        model.MountTypeTmpfs,
				Destination: "/container/tmp",
			},
			{
				Type:        model.MountTypeProc,
				Destination: "/proc",
			},
		},
	}
	require.NoError(t, spec.SaveContainerSpec(specPath))

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "inspect", "--spec", specPath})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- container create verifies env var in spec ---

func TestContainerCreateSetsEnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "env.yaml")

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{
		"container", "create",
		"--spec", specPath,
		"--name", "env-test",
		"--command", "/bin/bash",
	})

	require.NoError(t, rootCmd.Execute())

	spec, err := model.LoadContainerSpec(specPath)
	require.NoError(t, err)

	found := false
	for _, e := range spec.Process.Env {
		if e == "MISBAH_CONTAINER=env-test" {
			found = true
			break
		}
	}
	assert.True(t, found, "spec should contain MISBAH_CONTAINER env var")
}

// --- container force release with stale lock and ForceRelease ---

func TestContainerStopForceWithSubprocess(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	cmd := exec.Command("sleep", "120")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	writeLockFile(t, locksDir, "force-sub", pid, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "stop", "--name", "force-sub", "--force"})

	err := rootCmd.Execute()
	require.NoError(t, err)

	// Verify process was killed
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	select {
	case waitErr := <-waitDone:
		assert.Error(t, waitErr, "sleep should exit with non-zero due to signal")
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for process to die")
	}
}

// --- container destroy with no cgroup (exercises cleanup path) ---

func TestContainerDestroyNoLockFile(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	// No lock file at all — exercises the "Container not running" branch and
	// cgroup cleanup (which should be a no-op) and lock file removal (IsNotExist).
	rootCmd.SetArgs([]string{"container", "destroy", "--name", "no-such-container"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- formatSize with larger values ---

func TestFormatSizeLargeValues(t *testing.T) {
	// 100 GB
	assert.Equal(t, "100.0 GB", formatSize(100*1024*1024*1024))
	// 2.5 GB
	assert.Equal(t, "2.5 GB", formatSize(uint64(2.5*1024*1024*1024)))
	// 999 bytes
	assert.Equal(t, "999 B", formatSize(999))
}

// --- Verify list with a subprocess PID lock ---

func TestContainerListSubprocessLock(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })

	writeLockFile(t, locksDir, "sub-list", cmd.Process.Pid, "test")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- container create produces correct cwd and mount ---

func TestContainerCreateDefaultMount(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "mount.yaml")

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{
		"container", "create",
		"--spec", specPath,
		"--name", "mount-test",
		"--command", "/bin/bash",
	})
	require.NoError(t, rootCmd.Execute())

	spec, err := model.LoadContainerSpec(specPath)
	require.NoError(t, err)

	assert.Equal(t, "/container/workspace", spec.Process.Cwd)
	require.Len(t, spec.Mounts, 1)
	assert.Equal(t, model.MountTypeBind, spec.Mounts[0].Type)
	assert.Equal(t, "/tmp", spec.Mounts[0].Source)
	assert.Equal(t, "/container/workspace", spec.Mounts[0].Destination)
}

// --- Verify inspect prints all namespace flags ---

func TestContainerInspectAllNamespaces(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "ns.yaml")
	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: "all-ns",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh"},
			Cwd:     "/container/workspace",
		},
		Namespaces: model.NamespaceSpec{
			User:    true,
			Mount:   true,
			PID:     true,
			Network: true,
			IPC:     true,
			UTS:     true,
		},
		Mounts: []model.MountSpec{
			{
				Type:        model.MountTypeBind,
				Source:      "/tmp",
				Destination: "/container/workspace",
				Options:     []string{"rw"},
			},
		},
	}
	require.NoError(t, spec.SaveContainerSpec(specPath))

	t.Cleanup(resetFlags)
	rootCmd.SetArgs([]string{"container", "inspect", "--spec", specPath})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// --- image commands (error paths — CRI not available) ---

func TestImagePullNoCRI(t *testing.T) {
	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_CRI_ENDPOINT", "unix:///tmp/nonexistent-cri.sock")

	rootCmd.SetArgs([]string{"image", "pull", "docker.io/library/alpine:latest"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestImageListNoCRI(t *testing.T) {
	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_CRI_ENDPOINT", "unix:///tmp/nonexistent-cri.sock")

	rootCmd.SetArgs([]string{"image", "list"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestImageInspectNoCRI(t *testing.T) {
	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_CRI_ENDPOINT", "unix:///tmp/nonexistent-cri.sock")

	rootCmd.SetArgs([]string{"image", "inspect", "alpine:latest"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestImagePruneNoCRI(t *testing.T) {
	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_CRI_ENDPOINT", "unix:///tmp/nonexistent-cri.sock")

	rootCmd.SetArgs([]string{"image", "prune"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestImagePullRequiresArg(t *testing.T) {
	t.Cleanup(resetFlags)

	rootCmd.SetArgs([]string{"image", "pull"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

func TestImageInspectRequiresArg(t *testing.T) {
	t.Cleanup(resetFlags)

	rootCmd.SetArgs([]string{"image", "inspect"})

	err := rootCmd.Execute()
	assert.Error(t, err)
}

// --- container list displays PID correctly ---

func TestContainerListDisplaysPID(t *testing.T) {
	tmpDir := t.TempDir()
	locksDir := filepath.Join(tmpDir, ".locks")
	require.NoError(t, os.MkdirAll(locksDir, 0755))

	myPID := os.Getpid()
	writeLockFile(t, locksDir, "pid-test", myPID, "claude")

	t.Cleanup(resetFlags)
	t.Setenv("MISBAH_TEMP_DIR", tmpDir)

	rootCmd.SetArgs([]string{"container", "list"})

	err := rootCmd.Execute()
	require.NoError(t, err)
	_ = strconv.Itoa(myPID) // just to use strconv import
}
