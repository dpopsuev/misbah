package tier

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidTier(t *testing.T) {
	assert.True(t, ValidTier(TierEco))
	assert.True(t, ValidTier(TierSys))
	assert.True(t, ValidTier(TierCom))
	assert.True(t, ValidTier(TierMod))
	assert.False(t, ValidTier("invalid"))
	assert.False(t, ValidTier(""))
}

func TestDefaultWritablePaths(t *testing.T) {
	assert.Nil(t, DefaultWritablePaths(TierEco))
	assert.Equal(t, []string{"src/"}, DefaultWritablePaths(TierSys))
	assert.Equal(t, []string{"pkg/"}, DefaultWritablePaths(TierCom))
	assert.Nil(t, DefaultWritablePaths(TierMod))
}

func TestTierSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    TierSpec
		wantErr string
	}{
		{
			name: "valid eco tier",
			spec: TierSpec{
				Tier:  TierEco,
				Repos: []string{"/home/user/repo"},
			},
		},
		{
			name: "valid sys tier",
			spec: TierSpec{
				Tier:          TierSys,
				Repos:         []string{"/home/user/repo"},
				WritablePaths: []string{"src/"},
			},
		},
		{
			name: "valid com tier",
			spec: TierSpec{
				Tier:          TierCom,
				Repos:         []string{"/home/user/repo"},
				WritablePaths: []string{"pkg/auth"},
			},
		},
		{
			name: "valid mod tier with explicit path",
			spec: TierSpec{
				Tier:          TierMod,
				Repos:         []string{"/home/user/repo"},
				WritablePaths: []string{"pkg/auth/handler.go"},
			},
		},
		{
			name: "valid with multiple repos",
			spec: TierSpec{
				Tier:  TierSys,
				Repos: []string{"/home/user/misbah", "/home/user/djinn"},
				WritablePaths: []string{"misbah/src/", "djinn/src/"},
			},
		},
		{
			name: "valid with custom workspace path",
			spec: TierSpec{
				Tier:          TierMod,
				Repos:         []string{"/home/user/repo"},
				WritablePaths: []string{"pkg/auth"},
				WorkspacePath: "/work",
			},
		},
		{
			name:    "invalid tier",
			spec:    TierSpec{Tier: "invalid", Repos: []string{"/repo"}},
			wantErr: "invalid tier",
		},
		{
			name:    "empty repos",
			spec:    TierSpec{Tier: TierMod},
			wantErr: "at least one repo",
		},
		{
			name:    "relative repo path",
			spec:    TierSpec{Tier: TierMod, Repos: []string{"relative/path"}},
			wantErr: "repo path must be absolute",
		},
		{
			name: "eco with writable paths",
			spec: TierSpec{
				Tier:          TierEco,
				Repos:         []string{"/repo"},
				WritablePaths: []string{"src/"},
			},
			wantErr: "eco tier must not have writable paths",
		},
		{
			name: "absolute writable path",
			spec: TierSpec{
				Tier:          TierMod,
				Repos:         []string{"/repo"},
				WritablePaths: []string{"/absolute/path"},
			},
			wantErr: "writable path must be relative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTierSpec_GetWorkspacePath(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		spec := TierSpec{}
		assert.Equal(t, DefaultWorkspacePath, spec.GetWorkspacePath())
	})

	t.Run("custom", func(t *testing.T) {
		spec := TierSpec{WorkspacePath: "/work"}
		assert.Equal(t, "/work", spec.GetWorkspacePath())
	})
}
