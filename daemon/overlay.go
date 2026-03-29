package daemon

import (
	"fmt"
	"sync"

	"github.com/dpopsuev/mirage"
)

// DiffEntry re-exports mirage.Change for backward compatibility.
type DiffEntry = mirage.Change

// OverlayStore tracks overlay directories per container for diff/commit.
// Wraps mirage.Space instances for each container.
type OverlayStore struct {
	mu     sync.RWMutex
	spaces map[string]mirage.Space
	dirs   map[string]overlayDirs // lower/upper paths for registration
}

type overlayDirs struct {
	lower string
	upper string
}

// NewOverlayStore creates an overlay store.
func NewOverlayStore() *OverlayStore {
	return &OverlayStore{
		spaces: make(map[string]mirage.Space),
		dirs:   make(map[string]overlayDirs),
	}
}

// Register records overlay paths for a container.
// Creates a plain mirage space (no fuse mount — Misbah manages the mount externally).
func (s *OverlayStore) Register(name, lower, upper string) {
	s.mu.Lock()
	s.dirs[name] = overlayDirs{lower: lower, upper: upper}
	// Create a plain space wrapping the existing lower/upper dirs.
	s.spaces[name] = &plainMirageSpace{lower: lower, upper: upper}
	s.mu.Unlock()
}

// Remove deletes tracking for a container.
func (s *OverlayStore) Remove(name string) {
	s.mu.Lock()
	delete(s.spaces, name)
	delete(s.dirs, name)
	s.mu.Unlock()
}

// Diff returns files changed by the agent in the overlay.
func (s *OverlayStore) Diff(name string) ([]DiffEntry, error) {
	s.mu.RLock()
	space, ok := s.spaces[name]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no overlay for container %q", name)
	}
	return space.Diff()
}

// Commit copies selected files from overlay upper to real workspace (lower).
func (s *OverlayStore) Commit(name string, paths []string) error {
	s.mu.RLock()
	space, ok := s.spaces[name]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no overlay for container %q", name)
	}
	return space.Commit(paths)
}

// plainMirageSpace wraps existing lower/upper directories as a mirage.Space.
// Misbah manages the fuse mount externally — this space only does diff/commit.
type plainMirageSpace struct {
	lower string
	upper string
}

func (s *plainMirageSpace) Diff() ([]mirage.Change, error) {
	return mirage.DiffDirs(s.lower, s.upper)
}

func (s *plainMirageSpace) Commit(paths []string) error {
	return mirage.CommitFiles(s.upper, s.lower, paths)
}

func (s *plainMirageSpace) Reset() error   { return nil } // Misbah manages lifecycle
func (s *plainMirageSpace) Destroy() error { return nil } // Misbah manages lifecycle
func (s *plainMirageSpace) WorkDir() string { return s.upper }
