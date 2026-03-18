package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createBareRepo creates a bare git repo with one commit for testing.
func createBareRepo(t *testing.T, dir, name string) string {
	t.Helper()

	// Create a regular repo with a commit
	workDir := filepath.Join(dir, name+"-work")
	require.NoError(t, os.MkdirAll(workDir, 0755))

	cmds := [][]string{
		{"git", "init", workDir},
		{"git", "-C", workDir, "config", "user.email", "test@test.com"},
		{"git", "-C", workDir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		require.NoError(t, cmd.Run(), "failed: %v", args)
	}

	// Create a file and commit
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Test"), 0644))
	cmd := exec.Command("git", "-C", workDir, "add", ".")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", workDir, "commit", "-m", "initial")
	require.NoError(t, cmd.Run())

	// Create bare clone
	bareDir := filepath.Join(dir, name+".git")
	cmd = exec.Command("git", "clone", "--bare", workDir, bareDir)
	require.NoError(t, cmd.Run())

	return bareDir
}

func TestGitCloneManager_ResolveGitCloneMounts(t *testing.T) {
	tmpDir := t.TempDir()
	bareRepo := createBareRepo(t, tmpDir, "test-repo")

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	mgr := NewGitCloneManager(logger, tmpDir)

	mounts := []model.MountSpec{
		{
			Type:        "git-clone",
			Destination: "/workspace/test-repo",
			GitClone: &model.GitCloneSpec{
				Repository: bareRepo,
			},
		},
	}

	resolved, err := mgr.ResolveGitCloneMounts(mounts)
	require.NoError(t, err)
	require.Len(t, resolved, 1)

	assert.Equal(t, "bind", resolved[0].Type)
	assert.Equal(t, "/workspace/test-repo", resolved[0].Destination)
	assert.NotEmpty(t, resolved[0].Source)
	assert.Nil(t, resolved[0].GitClone)

	// Verify the clone directory exists and has content
	_, err = os.Stat(filepath.Join(resolved[0].Source, "README.md"))
	assert.NoError(t, err)
}

func TestGitCloneManager_Passthrough(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	mgr := NewGitCloneManager(logger, tmpDir)

	mounts := []model.MountSpec{
		{
			Type:        "bind",
			Source:      "/tmp",
			Destination: "/workspace",
			Options:     []string{"rw"},
		},
		{
			Type:        "tmpfs",
			Destination: "/tmp/scratch",
		},
	}

	resolved, err := mgr.ResolveGitCloneMounts(mounts)
	require.NoError(t, err)
	require.Len(t, resolved, 2)

	// Unchanged
	assert.Equal(t, "bind", resolved[0].Type)
	assert.Equal(t, "/tmp", resolved[0].Source)
	assert.Equal(t, "tmpfs", resolved[1].Type)
}

func TestGitCloneManager_MixedMounts(t *testing.T) {
	tmpDir := t.TempDir()
	bareRepo := createBareRepo(t, tmpDir, "mixed-repo")

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	mgr := NewGitCloneManager(logger, tmpDir)

	mounts := []model.MountSpec{
		{
			Type:        "bind",
			Source:      "/tmp",
			Destination: "/workspace/local",
			Options:     []string{"rw"},
		},
		{
			Type:        "git-clone",
			Destination: "/workspace/remote",
			GitClone: &model.GitCloneSpec{
				Repository: bareRepo,
			},
		},
	}

	resolved, err := mgr.ResolveGitCloneMounts(mounts)
	require.NoError(t, err)
	require.Len(t, resolved, 2)

	assert.Equal(t, "bind", resolved[0].Type)
	assert.Equal(t, "/tmp", resolved[0].Source)
	assert.Equal(t, "bind", resolved[1].Type)
	assert.NotEmpty(t, resolved[1].Source)
}

func TestGitCloneManager_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	bareRepo := createBareRepo(t, tmpDir, "cleanup-repo")

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	mgr := NewGitCloneManager(logger, tmpDir)

	mounts := []model.MountSpec{
		{
			Type:        "git-clone",
			Destination: "/workspace/repo",
			GitClone:    &model.GitCloneSpec{Repository: bareRepo},
		},
	}

	resolved, err := mgr.ResolveGitCloneMounts(mounts)
	require.NoError(t, err)

	cloneDir := resolved[0].Source
	_, err = os.Stat(cloneDir)
	require.NoError(t, err, "clone dir should exist before cleanup")

	err = mgr.Cleanup()
	require.NoError(t, err)

	_, err = os.Stat(cloneDir)
	assert.True(t, os.IsNotExist(err), "clone dir should be removed after cleanup")
}

func TestGitCloneManager_InvalidRepo(t *testing.T) {
	tmpDir := t.TempDir()
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	mgr := NewGitCloneManager(logger, tmpDir)

	mounts := []model.MountSpec{
		{
			Type:        "git-clone",
			Destination: "/workspace/repo",
			GitClone: &model.GitCloneSpec{
				Repository: "/nonexistent/repo.git",
			},
		},
	}

	_, err := mgr.ResolveGitCloneMounts(mounts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git clone failed")
}

func TestGitCloneManager_WithOptions(t *testing.T) {
	tmpDir := t.TempDir()
	bareRepo := createBareRepo(t, tmpDir, "options-repo")

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	mgr := NewGitCloneManager(logger, tmpDir)

	mounts := []model.MountSpec{
		{
			Type:        "git-clone",
			Destination: "/workspace/repo",
			Options:     []string{"ro"},
			GitClone: &model.GitCloneSpec{
				Repository: bareRepo,
				Depth:      1,
			},
		},
	}

	resolved, err := mgr.ResolveGitCloneMounts(mounts)
	require.NoError(t, err)

	// Options should be preserved on the rewritten bind mount
	assert.Equal(t, []string{"ro"}, resolved[0].Options)
}

func TestRepoBaseName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/user/repo.git", "repo"},
		{"https://github.com/user/repo", "repo"},
		{"/home/user/my-project.git", "my-project"},
		{"/home/user/my-project", "my-project"},
		{"git@github.com:user/repo.git", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, repoBaseName(tt.input))
		})
	}
}
