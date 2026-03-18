package harness

import (
	"os"
	"sync"
	"syscall"
)

// CleanupGuard tracks processes and directories for panic-safe cleanup.
// Register via t.Cleanup() to ensure resources are released even on t.Fatal() or panic.
type CleanupGuard struct {
	mu        sync.Mutex
	processes []*os.Process
}

// NewCleanupGuard creates a new cleanup guard.
func NewCleanupGuard() *CleanupGuard {
	return &CleanupGuard{}
}

// TrackProcess adds a process to be killed during cleanup.
func (cg *CleanupGuard) TrackProcess(p *os.Process) {
	if p == nil {
		return
	}
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.processes = append(cg.processes, p)
}

// Cleanup kills all tracked processes.
func (cg *CleanupGuard) Cleanup() {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	for _, p := range cg.processes {
		// Check if process is still alive before killing
		if err := p.Signal(syscall.Signal(0)); err == nil {
			p.Kill()
			p.Wait()
		}
	}
	cg.processes = nil
}
