package harness

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLabIsolation(t *testing.T) {
	lab1 := NewLab(t)
	lab2 := NewLab(t)

	// Each lab has its own root dir
	assert.NotEqual(t, lab1.RootDir(), lab2.RootDir())
	assert.NotEqual(t, lab1.LocksDir(), lab2.LocksDir())

	// Creating a lock in lab1 doesn't appear in lab2
	lockPath := filepath.Join(lab1.LocksDir(), "test.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte(`{"workspace":"test","pid":1}`), 0644))

	assert.True(t, lab1.LockProbe().Exists("test"))
	assert.False(t, lab2.LockProbe().Exists("test"))
}

func TestLockProbeWaitForLock(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	// Write lock file after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		lockData, _ := json.Marshal(map[string]interface{}{
			"workspace":  "delayed",
			"provider":   "test",
			"pid":        os.Getpid(),
			"started_at": time.Now(),
			"user":       "test",
		})
		os.WriteFile(filepath.Join(lab.LocksDir(), "delayed.lock"), lockData, 0644)
	}()

	// Should wait and succeed
	probe.WaitForLock(t, "delayed", 5*time.Second)
	assert.True(t, probe.Exists("delayed"))
}

func TestLockProbeWaitForRelease(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	// Create lock first
	lockPath := filepath.Join(lab.LocksDir(), "ephemeral.lock")
	lockData, _ := json.Marshal(map[string]interface{}{
		"workspace":  "ephemeral",
		"provider":   "test",
		"pid":        os.Getpid(),
		"started_at": time.Now(),
		"user":       "test",
	})
	require.NoError(t, os.WriteFile(lockPath, lockData, 0644))
	require.True(t, probe.Exists("ephemeral"))

	// Remove after delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		os.Remove(lockPath)
	}()

	probe.WaitForRelease(t, "ephemeral", 5*time.Second)
	assert.False(t, probe.Exists("ephemeral"))
}

func TestLockProbeCount(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	assert.Equal(t, 0, probe.Count())

	os.WriteFile(filepath.Join(lab.LocksDir(), "a.lock"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(lab.LocksDir(), "b.lock"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(lab.LocksDir(), "not-a-lock.txt"), []byte("{}"), 0644)

	assert.Equal(t, 2, probe.Count())
}

func TestLockProbeAll(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	os.WriteFile(filepath.Join(lab.LocksDir(), "alpha.lock"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(lab.LocksDir(), "beta.lock"), []byte("{}"), 0644)

	names := probe.All()
	assert.Contains(t, names, "alpha")
	assert.Contains(t, names, "beta")
	assert.Len(t, names, 2)
}

func TestCleanupGuardKillsProcesses(t *testing.T) {
	guard := NewCleanupGuard()

	// Start a sleep process
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	guard.TrackProcess(cmd.Process)

	pid := cmd.Process.Pid
	probe := NewContainerProbe(pid)
	assert.True(t, probe.IsAlive())

	// Cleanup should kill it
	guard.Cleanup()

	// Give OS a moment to reap
	time.Sleep(50 * time.Millisecond)
	assert.False(t, probe.IsAlive())
}

func TestContainerProbeAlive(t *testing.T) {
	// Our own PID should be alive
	probe := NewContainerProbe(os.Getpid())
	assert.True(t, probe.IsAlive())

	// A nonsense PID should not be alive
	probe2 := NewContainerProbe(999999999)
	assert.False(t, probe2.IsAlive())
}

func TestContainerProbeNamespaces(t *testing.T) {
	probe := NewContainerProbe(os.Getpid())
	ns := probe.Namespaces()

	// Should have at least user, mnt, pid namespaces
	assert.NotEmpty(t, ns)
	assert.Contains(t, ns, "user")
	assert.Contains(t, ns, "mnt")
}

func TestAssertLockReleased(t *testing.T) {
	lab := NewLab(t)
	// No lock exists — should pass
	AssertLockReleased(t, lab, "nonexistent")
}

func TestAssertNoStaleState(t *testing.T) {
	lab := NewLab(t)
	// Empty dir — should pass
	AssertNoStaleState(t, lab)
}
