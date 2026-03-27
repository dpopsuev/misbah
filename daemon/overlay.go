package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// OverlayStore tracks overlay directories per container for diff/commit.
type OverlayStore struct {
	mu       sync.RWMutex
	overlays map[string]*OverlayInfo
}

// OverlayInfo holds the overlay paths for a container.
type OverlayInfo struct {
	Lower string // real workspace (read-only)
	Upper string // agent's changes
}

// NewOverlayStore creates an overlay store.
func NewOverlayStore() *OverlayStore {
	return &OverlayStore{overlays: make(map[string]*OverlayInfo)}
}

// Register records overlay paths for a container.
func (s *OverlayStore) Register(name string, lower, upper string) {
	s.mu.Lock()
	s.overlays[name] = &OverlayInfo{Lower: lower, Upper: upper}
	s.mu.Unlock()
}

// Remove deletes tracking for a container.
func (s *OverlayStore) Remove(name string) {
	s.mu.Lock()
	delete(s.overlays, name)
	s.mu.Unlock()
}

// DiffEntry represents a changed file in the overlay.
type DiffEntry struct {
	Path   string `json:"path"`
	Change string `json:"change"` // "modified", "created", "deleted"
}

// Diff returns files changed by the agent in the overlay.
func (s *OverlayStore) Diff(name string) ([]DiffEntry, error) {
	s.mu.RLock()
	ov, ok := s.overlays[name]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no overlay for container %q", name)
	}

	var entries []DiffEntry
	err := filepath.Walk(ov.Upper, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == ov.Upper {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(ov.Upper, path)

		// Check if file exists in lower to classify change type.
		change := "created"
		if _, err := os.Stat(filepath.Join(ov.Lower, rel)); err == nil {
			change = "modified"
		}
		entries = append(entries, DiffEntry{Path: rel, Change: change})
		return nil
	})
	return entries, err
}

// Commit copies selected files from overlay upper to real workspace (lower).
func (s *OverlayStore) Commit(name string, paths []string) error {
	s.mu.RLock()
	ov, ok := s.overlays[name]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no overlay for container %q", name)
	}

	for _, p := range paths {
		src := filepath.Join(ov.Upper, p)
		dst := filepath.Join(ov.Lower, p)

		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("commit read %s: %w", p, err)
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("commit mkdir: %w", err)
		}

		info, _ := os.Stat(src)
		if err := os.WriteFile(dst, data, info.Mode()); err != nil {
			return fmt.Errorf("commit write %s: %w", p, err)
		}
	}
	return nil
}
