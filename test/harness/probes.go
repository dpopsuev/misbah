package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/daemon"
	"github.com/dpopsuev/misbah/model"
)

// LockProbe reads lock state directly from the filesystem.
type LockProbe struct {
	locksDir string
}

// NewLockProbe creates a lock probe for the given directory.
func NewLockProbe(locksDir string) *LockProbe {
	return &LockProbe{locksDir: locksDir}
}

// Exists checks if a lock file exists for the given container name.
func (lp *LockProbe) Exists(name string) bool {
	_, err := os.Stat(filepath.Join(lp.locksDir, name+".lock"))
	return err == nil
}

// Read reads and parses a lock file.
func (lp *LockProbe) Read(name string) (*model.Lock, error) {
	data, err := os.ReadFile(filepath.Join(lp.locksDir, name+".lock"))
	if err != nil {
		return nil, err
	}
	return model.UnmarshalLock(data)
}

// IsStale checks if a lock exists and is stale (process no longer running).
func (lp *LockProbe) IsStale(name string) bool {
	lock, err := lp.Read(name)
	if err != nil {
		return false
	}
	return lock.IsStale()
}

// WaitForLock polls until a lock file appears, or fails the test on timeout.
func (lp *LockProbe) WaitForLock(t *testing.T, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if lp.Exists(name) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for lock %q (dir: %s)", name, lp.locksDir)
}

// WaitForRelease polls until a lock file disappears, or fails the test on timeout.
func (lp *LockProbe) WaitForRelease(t *testing.T, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !lp.Exists(name) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for lock release %q (dir: %s)", name, lp.locksDir)
}

// Count returns the number of lock files in the directory.
func (lp *LockProbe) Count() int {
	entries, err := os.ReadDir(lp.locksDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".lock") {
			count++
		}
	}
	return count
}

// All returns all lock file names (without .lock extension).
func (lp *LockProbe) All() []string {
	entries, err := os.ReadDir(lp.locksDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".lock") {
			names = append(names, strings.TrimSuffix(e.Name(), ".lock"))
		}
	}
	return names
}

// ContainerProbe checks process state via /proc.
type ContainerProbe struct {
	pid int
}

// NewContainerProbe creates a probe for the given PID.
func NewContainerProbe(pid int) *ContainerProbe {
	return &ContainerProbe{pid: pid}
}

// IsAlive checks if the process is still running.
func (cp *ContainerProbe) IsAlive() bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d/status", cp.pid))
	return err == nil
}

// Namespaces returns the namespace inode links for the process.
func (cp *ContainerProbe) Namespaces() map[string]string {
	nsDir := fmt.Sprintf("/proc/%d/ns", cp.pid)
	entries, err := os.ReadDir(nsDir)
	if err != nil {
		return nil
	}
	result := make(map[string]string)
	for _, e := range entries {
		link, err := os.Readlink(filepath.Join(nsDir, e.Name()))
		if err == nil {
			result[e.Name()] = link
		}
	}
	return result
}

// DaemonProbe checks permission daemon health via its client.
type DaemonProbe struct {
	client *daemon.Client
}

// NewDaemonProbe creates a probe using the given daemon client.
func NewDaemonProbe(client *daemon.Client) *DaemonProbe {
	return &DaemonProbe{client: client}
}

// IsReady checks if the daemon is reachable.
func (dp *DaemonProbe) IsReady() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := dp.client.Check(ctx, daemon.PermissionRequest{
		ResourceType: daemon.ResourceNetwork,
		ResourceID:   "__probe__",
	})
	return err == nil
}

// WhitelistRules returns current whitelist state from the daemon.
func (dp *DaemonProbe) WhitelistRules() map[string]string {
	// The daemon's /permission/list endpoint returns rules
	// For now, use Check to verify specific entries
	return nil
}
