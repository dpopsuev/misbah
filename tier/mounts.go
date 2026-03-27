package tier

import (
	"fmt"
	"path/filepath"

	"github.com/dpopsuev/misbah/model"
)

// GenerateTierMounts produces an ordered mount list for a tier specification.
// Each repo is mounted read-only at /workspace/<repo-basename>.
// Writable paths are overlaid as read-write bind mounts on top.
// The returned list is ordered: RO mounts first, then RW overlays.
func GenerateTierMounts(spec *TierSpec) ([]model.MountSpec, error) {
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("invalid tier spec: %w", err)
	}

	workspace := spec.GetWorkspacePath()
	var roMounts, rwMounts []model.MountSpec

	// 1. Mount each repo read-only
	for _, repo := range spec.Repos {
		base := filepath.Base(repo)
		dest := filepath.Join(workspace, base)

		roMounts = append(roMounts, model.MountSpec{
			Type:        model.MountTypeBind,
			Source:      repo,
			Destination: dest,
			Options:     []string{"ro", "rbind"},
		})
	}

	// 2. Overlay writable paths — copy-on-write via overlayfs.
	// Each writable path gets its own overlay with a unique upper/work dir.
	// Agent writes are captured in upper. Real workspace is untouched.
	for i, wp := range spec.WritablePaths {
		source, dest, err := resolveWritablePath(spec.Repos, workspace, wp)
		if err != nil {
			return nil, err
		}

		upperDir := fmt.Sprintf("/tmp/misbah-overlay-%s-%d/upper", filepath.Base(spec.Repos[0]), i)
		workDir := fmt.Sprintf("/tmp/misbah-overlay-%s-%d/work", filepath.Base(spec.Repos[0]), i)

		rwMounts = append(rwMounts, model.MountSpec{
			Type:        model.MountTypeOverlay,
			Source:      source,
			Destination: dest,
			Overlay: &model.OverlaySpec{
				Lower: source,
				Upper: upperDir,
				Work:  workDir,
			},
		})
	}

	// RO first, then RW overlays (mount order matters)
	return append(roMounts, rwMounts...), nil
}

// resolveWritablePath finds the host source path for a relative writable path.
// The writable path must be within one of the repos.
func resolveWritablePath(repos []string, workspace, writablePath string) (source, dest string, err error) {
	// Try each repo to find which one contains this path
	for _, repo := range repos {
		base := filepath.Base(repo)
		candidateSource := filepath.Join(repo, writablePath)
		candidateDest := filepath.Join(workspace, base, writablePath)

		// Check if the writable path starts with repo basename
		// (for multi-repo setups, writable_paths like "misbah/pkg/auth/" include the repo name)
		if len(repos) > 1 {
			// Multi-repo: writable path should start with repo basename
			repoPrefix := base + "/"
			if len(writablePath) > len(repoPrefix) && writablePath[:len(repoPrefix)] == repoPrefix {
				subPath := writablePath[len(repoPrefix):]
				return filepath.Join(repo, subPath), filepath.Join(workspace, writablePath), nil
			}
		} else {
			// Single repo: writable path is relative to the repo root
			return candidateSource, candidateDest, nil
		}
	}

	return "", "", fmt.Errorf("writable path %q does not match any mounted repo", writablePath)
}
