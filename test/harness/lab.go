package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
)

// Lab provides an isolated test environment for Misbah E2E tests.
// Each Lab gets its own temp directory for locks, sockets, and state.
// Use t.Cleanup() guarantees resource cleanup even on panic.
type Lab struct {
	t         *testing.T
	rootDir   string
	locksDir  string
	logger    *metrics.Logger
	guard     *CleanupGuard
	lockProbe *LockProbe

	buildOnce sync.Once
	binPath   string
	buildErr  error
}

// NewLab creates a new isolated test environment.
func NewLab(t *testing.T) *Lab {
	t.Helper()

	rootDir := t.TempDir()
	locksDir := filepath.Join(rootDir, ".locks")

	if err := os.MkdirAll(locksDir, 0755); err != nil {
		t.Fatalf("failed to create locks dir: %v", err)
	}

	// Set MISBAH_TEMP_DIR so all config.GetLocksDir() / config.GetTempDir() calls
	// use our isolated directory. t.Setenv restores the original value on cleanup.
	t.Setenv("MISBAH_TEMP_DIR", rootDir)

	guard := NewCleanupGuard()
	t.Cleanup(guard.Cleanup)

	logger := metrics.NewLogger(metrics.LogLevelDebug, os.Stderr)

	return &Lab{
		t:         t,
		rootDir:   rootDir,
		locksDir:  locksDir,
		logger:    logger,
		guard:     guard,
		lockProbe: NewLockProbe(locksDir),
	}
}

// MisbahBin returns the path to a freshly built misbah binary.
// The binary is built once per Lab and cached.
func (l *Lab) MisbahBin() string {
	l.t.Helper()
	l.buildOnce.Do(func() {
		root := repoRoot(l.t)
		l.binPath = filepath.Join(l.rootDir, "misbah")
		cmd := exec.Command("go", "build", "-o", l.binPath, "./cmd/misbah")
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			l.buildErr = fmt.Errorf("build failed: %v\n%s", err, out)
		}
	})
	if l.buildErr != nil {
		l.t.Fatalf("MisbahBin: %v", l.buildErr)
	}
	return l.binPath
}

// RootDir returns the lab's isolated root directory.
func (l *Lab) RootDir() string {
	return l.rootDir
}

// LocksDir returns the lab's isolated locks directory.
func (l *Lab) LocksDir() string {
	return l.locksDir
}

// SocketPath returns an isolated Unix socket path for the permission daemon.
func (l *Lab) SocketPath() string {
	return filepath.Join(l.rootDir, "daemon.sock")
}

// LockProbe returns the lock probe for this lab.
func (l *Lab) LockProbe() *LockProbe {
	return l.lockProbe
}

// Guard returns the cleanup guard for tracking processes.
func (l *Lab) Guard() *CleanupGuard {
	return l.guard
}

// Logger returns the lab's logger.
func (l *Lab) Logger() *metrics.Logger {
	return l.logger
}

// RunMisbah executes the misbah binary with the lab's isolated environment.
// Returns combined stdout+stderr and any error.
func (l *Lab) RunMisbah(args ...string) (string, error) {
	l.t.Helper()
	bin := l.MisbahBin()
	cmd := exec.Command(bin, args...)
	cmd.Env = l.env()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// StartMisbah starts the misbah binary in the background.
// The process is tracked by the cleanup guard.
func (l *Lab) StartMisbah(args ...string) *exec.Cmd {
	l.t.Helper()
	bin := l.MisbahBin()
	cmd := exec.Command(bin, args...)
	cmd.Env = l.env()
	// Don't connect stdout/stderr — avoids "WaitDelay expired before I/O complete"
	// when the test kills the background process.
	if err := cmd.Start(); err != nil {
		l.t.Fatalf("StartMisbah failed: %v", err)
	}
	l.guard.TrackProcess(cmd.Process)
	return cmd
}

// env returns the environment for misbah subprocesses.
func (l *Lab) env() []string {
	env := os.Environ()
	// Override MISBAH_TEMP_DIR to point to our isolated root
	var filtered []string
	for _, e := range env {
		if !strings.HasPrefix(e, "MISBAH_TEMP_DIR=") {
			filtered = append(filtered, e)
		}
	}
	filtered = append(filtered, "MISBAH_TEMP_DIR="+l.rootDir)
	return filtered
}

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
