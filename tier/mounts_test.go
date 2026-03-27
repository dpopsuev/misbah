package tier

import (
	"testing"

	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateTierMounts_Eco(t *testing.T) {
	spec := &TierSpec{
		Tier:  TierEco,
		Repos: []string{"/home/user/misbah"},
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)
	require.Len(t, mounts, 1)

	// Single RO mount
	assert.Equal(t, "bind", mounts[0].Type)
	assert.Equal(t, "/home/user/misbah", mounts[0].Source)
	assert.Equal(t, "/workspace/misbah", mounts[0].Destination)
	assert.Contains(t, mounts[0].Options, "ro")
}

func TestGenerateTierMounts_Sys(t *testing.T) {
	spec := &TierSpec{
		Tier:          TierSys,
		Repos:         []string{"/home/user/misbah"},
		WritablePaths: []string{"src/"},
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)
	require.Len(t, mounts, 2)

	// First: RO base
	assert.Equal(t, "/home/user/misbah", mounts[0].Source)
	assert.Equal(t, "/workspace/misbah", mounts[0].Destination)
	assert.Contains(t, mounts[0].Options, "ro")

	// Second: RW overlay (filepath.Join strips trailing slash)
	assert.Equal(t, "/home/user/misbah/src", mounts[1].Source)
	assert.Equal(t, "/workspace/misbah/src", mounts[1].Destination)
	assert.Equal(t, model.MountTypeOverlay, mounts[1].Type)
}

func TestGenerateTierMounts_Com(t *testing.T) {
	spec := &TierSpec{
		Tier:          TierCom,
		Repos:         []string{"/home/user/misbah"},
		WritablePaths: []string{"pkg/auth"},
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)
	require.Len(t, mounts, 2)

	assert.Contains(t, mounts[0].Options, "ro")
	assert.Equal(t, "/workspace/misbah/pkg/auth", mounts[1].Destination)
	assert.Equal(t, model.MountTypeOverlay, mounts[1].Type)
}

func TestGenerateTierMounts_Mod(t *testing.T) {
	spec := &TierSpec{
		Tier:          TierMod,
		Repos:         []string{"/home/user/misbah"},
		WritablePaths: []string{"pkg/auth/handler.go"},
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)
	require.Len(t, mounts, 2)

	assert.Contains(t, mounts[0].Options, "ro")
	assert.Equal(t, "/home/user/misbah/pkg/auth/handler.go", mounts[1].Source)
	assert.Equal(t, model.MountTypeOverlay, mounts[1].Type)
}

func TestGenerateTierMounts_MultipleRepos(t *testing.T) {
	spec := &TierSpec{
		Tier:  TierEco,
		Repos: []string{"/home/user/misbah", "/home/user/djinn", "/home/user/origami"},
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)
	require.Len(t, mounts, 3)

	assert.Equal(t, "/workspace/misbah", mounts[0].Destination)
	assert.Equal(t, "/workspace/djinn", mounts[1].Destination)
	assert.Equal(t, "/workspace/origami", mounts[2].Destination)

	for _, m := range mounts {
		assert.Contains(t, m.Options, "ro")
	}
}

func TestGenerateTierMounts_MultipleWritablePaths(t *testing.T) {
	spec := &TierSpec{
		Tier:          TierMod,
		Repos:         []string{"/home/user/misbah"},
		WritablePaths: []string{"pkg/auth", "pkg/db"},
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)
	require.Len(t, mounts, 3) // 1 RO + 2 RW

	// RO first
	assert.Contains(t, mounts[0].Options, "ro")
	// RW overlays after
	assert.Equal(t, model.MountTypeOverlay, mounts[1].Type)
	assert.Equal(t, model.MountTypeOverlay, mounts[2].Type)
}

func TestGenerateTierMounts_MountOrdering(t *testing.T) {
	spec := &TierSpec{
		Tier:          TierSys,
		Repos:         []string{"/home/user/misbah"},
		WritablePaths: []string{"src/", "cmd/"},
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)
	require.Len(t, mounts, 3) // 1 RO + 2 RW

	// First mount must be RO (mount order matters)
	assert.Contains(t, mounts[0].Options, "ro")
	// Subsequent mounts are RW overlays
	assert.Equal(t, model.MountTypeOverlay, mounts[1].Type)
	assert.Equal(t, model.MountTypeOverlay, mounts[2].Type)
}

func TestGenerateTierMounts_CustomWorkspace(t *testing.T) {
	spec := &TierSpec{
		Tier:          TierMod,
		Repos:         []string{"/home/user/misbah"},
		WritablePaths: []string{"pkg/auth"},
		WorkspacePath: "/work",
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)

	assert.Equal(t, "/work/misbah", mounts[0].Destination)
	assert.Equal(t, "/work/misbah/pkg/auth", mounts[1].Destination)
}

func TestGenerateTierMounts_InvalidSpec(t *testing.T) {
	spec := &TierSpec{
		Tier: "invalid",
	}

	_, err := GenerateTierMounts(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tier spec")
}

func TestGenerateTierMounts_MultiRepoWritablePath(t *testing.T) {
	spec := &TierSpec{
		Tier:          TierSys,
		Repos:         []string{"/home/user/misbah", "/home/user/djinn"},
		WritablePaths: []string{"misbah/src/", "djinn/src/"},
	}

	mounts, err := GenerateTierMounts(spec)
	require.NoError(t, err)
	require.Len(t, mounts, 4) // 2 RO + 2 RW

	// RO mounts
	assert.Equal(t, "/workspace/misbah", mounts[0].Destination)
	assert.Equal(t, "/workspace/djinn", mounts[1].Destination)

	// RW overlays (prefixed with repo name)
	assert.Equal(t, "/workspace/misbah/src", mounts[2].Destination)
	assert.Equal(t, "/workspace/djinn/src", mounts[3].Destination)
}
