package harness

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/daemon"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLabAccessors(t *testing.T) {
	lab := NewLab(t)

	t.Run("SocketPath", func(t *testing.T) {
		sp := lab.SocketPath()
		assert.NotEmpty(t, sp)
		assert.Contains(t, sp, "daemon.sock")
	})

	t.Run("Guard", func(t *testing.T) {
		g := lab.Guard()
		assert.NotNil(t, g)
	})

	t.Run("Logger", func(t *testing.T) {
		l := lab.Logger()
		assert.NotNil(t, l)
	})
}

func TestLockProbeRead(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	lockData, err := json.Marshal(map[string]interface{}{
		"workspace":  "myworkspace",
		"provider":   "testprovider",
		"pid":        os.Getpid(),
		"user":       "testuser",
		"started_at": now,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lab.LocksDir(), "readtest.lock"), lockData, 0644))

	lock, err := probe.Read("readtest")
	require.NoError(t, err)
	assert.Equal(t, "myworkspace", lock.Workspace)
	assert.Equal(t, "testprovider", lock.Provider)
	assert.Equal(t, os.Getpid(), lock.PID)
	assert.Equal(t, "testuser", lock.User)
	assert.True(t, lock.StartedAt.Equal(now))
}

func TestLockProbeRead_Invalid(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	require.NoError(t, os.WriteFile(filepath.Join(lab.LocksDir(), "bad.lock"), []byte("not json{{{"), 0644))

	lock, err := probe.Read("bad")
	assert.Error(t, err)
	assert.Nil(t, lock)
}

func TestLockProbeIsStale_LiveProcess(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	lockData, err := json.Marshal(map[string]interface{}{
		"workspace":  "live",
		"provider":   "test",
		"pid":        os.Getpid(),
		"user":       "test",
		"started_at": time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lab.LocksDir(), "live.lock"), lockData, 0644))

	// Our own PID is alive, so IsStale should return false
	assert.False(t, probe.IsStale("live"))
}

func TestLockProbeIsStale_DeadProcess(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	lockData, err := json.Marshal(map[string]interface{}{
		"workspace":  "dead",
		"provider":   "test",
		"pid":        999999999,
		"user":       "test",
		"started_at": time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lab.LocksDir(), "dead.lock"), lockData, 0644))

	// PID 999999999 does not exist, so IsStale should return true
	assert.True(t, probe.IsStale("dead"))
}

func TestLockProbeIsStale_NoFile(t *testing.T) {
	lab := NewLab(t)
	probe := lab.LockProbe()

	// No lock file exists — Read will fail, IsStale returns false
	assert.False(t, probe.IsStale("nonexistent"))
}

func TestAssertLockHeld(t *testing.T) {
	lab := NewLab(t)

	// Start a long-running process so its PID is alive
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	lab.Guard().TrackProcess(cmd.Process)

	lockData, err := json.Marshal(map[string]interface{}{
		"workspace":  "held",
		"provider":   "test",
		"pid":        cmd.Process.Pid,
		"user":       "test",
		"started_at": time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lab.LocksDir(), "held.lock"), lockData, 0644))

	// Should not fail — lock exists and process is alive
	AssertLockHeld(t, lab, "held")
}

func TestAssertContainerExited_DeadProcess(t *testing.T) {
	lab := NewLab(t)

	lockData, err := json.Marshal(map[string]interface{}{
		"workspace":  "exited",
		"provider":   "test",
		"pid":        999999999,
		"user":       "test",
		"started_at": time.Now(),
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lab.LocksDir(), "exited.lock"), lockData, 0644))

	// PID 999999999 is not alive, so this should pass
	AssertContainerExited(t, lab, "exited")
}

func TestAssertContainerExited_NoLock(t *testing.T) {
	lab := NewLab(t)

	// No lock file — container exited and cleaned up, should pass
	AssertContainerExited(t, lab, "gone")
}

func TestDaemonProbe(t *testing.T) {
	// Create a client pointing to a non-existent socket
	logger := metrics.NewLogger(metrics.LogLevelDebug, os.Stderr)
	client := daemon.NewClient("/tmp/nonexistent-misbah-test.sock", logger)
	defer client.Close()

	probe := NewDaemonProbe(client)

	t.Run("IsReady_false", func(t *testing.T) {
		// No daemon running — IsReady should return false
		assert.False(t, probe.IsReady())
	})

	t.Run("WhitelistRules_nil", func(t *testing.T) {
		// Stub returns nil
		assert.Nil(t, probe.WhitelistRules())
	})
}

func TestCleanupGuard_NilProcess(t *testing.T) {
	guard := NewCleanupGuard()

	// TrackProcess(nil) should not panic
	assert.NotPanics(t, func() {
		guard.TrackProcess(nil)
	})

	// Cleanup should also not panic with no tracked processes
	assert.NotPanics(t, func() {
		guard.Cleanup()
	})
}

func TestLabMisbahBin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping MisbahBin test in short mode (requires go build)")
	}

	lab := NewLab(t)
	bin := lab.MisbahBin()

	assert.NotEmpty(t, bin)

	// Binary should exist on disk
	info, err := os.Stat(bin)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
	assert.True(t, info.Size() > 0)

	// Calling MisbahBin again should return the same cached path
	bin2 := lab.MisbahBin()
	assert.Equal(t, bin, bin2)
}
