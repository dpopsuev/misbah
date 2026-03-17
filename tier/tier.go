package tier

import (
	"fmt"
	"path/filepath"
)

// Tier represents an agent tier level in the recursive hierarchy.
type Tier string

const (
	TierEco Tier = "eco" // Ecosystem: all read-only
	TierSys Tier = "sys" // System: src/ writable
	TierCom Tier = "com" // Component: pkg/ writable
	TierMod Tier = "mod" // Module: specific package writable
)

// TierSpec defines the tier-scoped mount configuration.
type TierSpec struct {
	Tier          Tier     `yaml:"tier"`
	Repos         []string `yaml:"repos"`                    // absolute host repo paths
	WritablePaths []string `yaml:"writable_paths,omitempty"` // relative paths within workspace to make RW
	WorkspacePath string   `yaml:"workspace_path,omitempty"` // container mount point (default /workspace)
}

// DefaultWorkspacePath is the default mount point for repos inside the container.
const DefaultWorkspacePath = "/workspace"

// ValidTier checks if a tier value is valid.
func ValidTier(t Tier) bool {
	switch t {
	case TierEco, TierSys, TierCom, TierMod:
		return true
	default:
		return false
	}
}

// DefaultWritablePaths returns the conventional writable paths for a tier.
// Eco gets nothing (read-only). Mod returns empty (caller must specify explicitly).
func DefaultWritablePaths(t Tier) []string {
	switch t {
	case TierEco:
		return nil
	case TierSys:
		return []string{"src/"}
	case TierCom:
		return []string{"pkg/"}
	case TierMod:
		return nil // caller must specify explicit package path
	default:
		return nil
	}
}

// Validate validates the tier specification.
func (s *TierSpec) Validate() error {
	if !ValidTier(s.Tier) {
		return fmt.Errorf("invalid tier: %q (must be eco, sys, com, or mod)", s.Tier)
	}

	if len(s.Repos) == 0 {
		return fmt.Errorf("at least one repo is required")
	}

	for _, repo := range s.Repos {
		if !filepath.IsAbs(repo) {
			return fmt.Errorf("repo path must be absolute: %s", repo)
		}
	}

	if s.Tier == TierEco && len(s.WritablePaths) > 0 {
		return fmt.Errorf("eco tier must not have writable paths (read-only)")
	}

	for _, wp := range s.WritablePaths {
		if filepath.IsAbs(wp) {
			return fmt.Errorf("writable path must be relative: %s", wp)
		}
	}

	return nil
}

// GetWorkspacePath returns the workspace path, defaulting to /workspace.
func (s *TierSpec) GetWorkspacePath() string {
	if s.WorkspacePath != "" {
		return s.WorkspacePath
	}
	return DefaultWorkspacePath
}
