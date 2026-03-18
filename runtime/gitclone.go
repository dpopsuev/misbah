package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
)

// GitCloneManager resolves git-clone mounts by cloning repos to temp dirs
// and rewriting them as bind mounts.
type GitCloneManager struct {
	logger     *metrics.Logger
	tempDir    string
	clonedDirs []string
}

// NewGitCloneManager creates a new git clone manager.
func NewGitCloneManager(logger *metrics.Logger, tempDir string) *GitCloneManager {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &GitCloneManager{
		logger:  logger,
		tempDir: tempDir,
	}
}

// ResolveGitCloneMounts resolves git-clone mounts by cloning repos and
// rewriting them as bind mounts. Non-git-clone mounts pass through unchanged.
func (g *GitCloneManager) ResolveGitCloneMounts(mounts []model.MountSpec) ([]model.MountSpec, error) {
	result := make([]model.MountSpec, 0, len(mounts))

	for _, m := range mounts {
		if m.Type != "git-clone" {
			result = append(result, m)
			continue
		}

		cloneDir, err := g.cloneRepo(m.GitClone)
		if err != nil {
			return nil, fmt.Errorf("failed to clone %s: %w", m.GitClone.Repository, err)
		}

		g.clonedDirs = append(g.clonedDirs, cloneDir)

		// Rewrite as bind mount
		result = append(result, model.MountSpec{
			Type:        "bind",
			Source:      cloneDir,
			Destination: m.Destination,
			Options:     m.Options,
		})

		g.logger.Infof("Cloned %s -> %s (mount at %s)", m.GitClone.Repository, cloneDir, m.Destination)
	}

	return result, nil
}

// Cleanup removes all cloned temporary directories.
func (g *GitCloneManager) Cleanup() error {
	var firstErr error
	for _, dir := range g.clonedDirs {
		if err := os.RemoveAll(dir); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	g.clonedDirs = nil
	return firstErr
}

func (g *GitCloneManager) cloneRepo(spec *model.GitCloneSpec) (string, error) {
	// Create temp dir for the clone
	cloneDir, err := os.MkdirTemp(g.tempDir, "misbah-clone-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	depth := spec.Depth
	if depth <= 0 {
		depth = 1
	}

	args := []string{"clone", "--depth", strconv.Itoa(depth)}

	if spec.Ref != "" {
		args = append(args, "--branch", spec.Ref)
	}

	// Clone into a subdirectory named after the repo
	repoName := repoBaseName(spec.Repository)
	targetDir := filepath.Join(cloneDir, repoName)

	args = append(args, spec.Repository, targetDir)

	g.logger.Debugf("git %v", args)

	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(cloneDir)
		return "", fmt.Errorf("git clone failed: %w\noutput: %s", err, string(output))
	}

	return targetDir, nil
}

// repoBaseName extracts a directory name from a repository URL.
func repoBaseName(repo string) string {
	base := filepath.Base(repo)
	// Remove .git suffix
	if len(base) > 4 && base[len(base)-4:] == ".git" {
		base = base[:len(base)-4]
	}
	if base == "" || base == "." {
		return "repo"
	}
	return base
}
