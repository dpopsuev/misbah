package harness

import (
	"testing"
)

// AssertLockHeld fails the test if no active lock exists for the container.
func AssertLockHeld(t *testing.T, lab *Lab, name string) {
	t.Helper()
	probe := lab.LockProbe()
	if !probe.Exists(name) {
		t.Fatalf("expected lock for %q to exist, but it does not (dir: %s)", name, lab.LocksDir())
	}
	if probe.IsStale(name) {
		t.Fatalf("lock for %q exists but is stale (process not running)", name)
	}
}

// AssertLockReleased fails the test if a lock still exists for the container.
func AssertLockReleased(t *testing.T, lab *Lab, name string) {
	t.Helper()
	probe := lab.LockProbe()
	if probe.Exists(name) {
		lock, _ := probe.Read(name)
		t.Fatalf("expected lock for %q to be released, but it still exists (PID: %d, stale: %v)",
			name, lock.PID, lock.IsStale())
	}
}

// AssertNoStaleState fails the test if any lock files remain in the lab.
func AssertNoStaleState(t *testing.T, lab *Lab) {
	t.Helper()
	probe := lab.LockProbe()
	locks := probe.All()
	if len(locks) > 0 {
		t.Fatalf("expected no remaining locks, but found %d: %v", len(locks), locks)
	}
}

// AssertContainerExited fails if the process behind a lock is still alive.
func AssertContainerExited(t *testing.T, lab *Lab, name string) {
	t.Helper()
	probe := lab.LockProbe()
	lock, err := probe.Read(name)
	if err != nil {
		// Lock file gone — container exited and cleaned up
		return
	}
	cp := NewContainerProbe(lock.PID)
	if cp.IsAlive() {
		t.Fatalf("container %q process (PID %d) is still alive", name, lock.PID)
	}
}
