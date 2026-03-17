package cri

import (
	"testing"

	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMountsToContainerMounts(t *testing.T) {
	mounts := []model.MountSpec{
		{
			Type:        "bind",
			Source:      "/home/user/repo",
			Destination: "/workspace/repo",
			Options:     []string{"ro"},
		},
		{
			Type:        "tmpfs",
			Destination: "/tmp/scratch",
		},
		{
			Type:        "bind",
			Source:      "/var/data",
			Destination: "/data",
			Options:     []string{"rw", "nosuid"},
		},
	}

	result := MountsToContainerMounts(mounts)
	require.Len(t, result, 3)

	assert.Equal(t, "/workspace/repo", result[0].ContainerPath)
	assert.Equal(t, "/home/user/repo", result[0].HostPath)
	assert.True(t, result[0].Readonly)

	assert.Equal(t, "/tmp/scratch", result[1].ContainerPath)
	assert.False(t, result[1].Readonly)

	assert.Equal(t, "/data", result[2].ContainerPath)
	assert.False(t, result[2].Readonly)
}

func TestMountsToContainerMounts_Empty(t *testing.T) {
	result := MountsToContainerMounts(nil)
	assert.Nil(t, result)

	result = MountsToContainerMounts([]model.MountSpec{})
	assert.Nil(t, result)
}

func TestResourcesToLinuxResources(t *testing.T) {
	t.Run("nil resources", func(t *testing.T) {
		result := ResourcesToLinuxResources(nil)
		assert.Nil(t, result)
	})

	t.Run("memory and cpu", func(t *testing.T) {
		resources := &model.ResourceSpec{
			Memory:    "2GB",
			CPUShares: 1024,
		}

		result := ResourcesToLinuxResources(resources)
		require.NotNil(t, result)
		assert.Equal(t, int64(2*1024*1024*1024), result.MemoryLimitInBytes)
		assert.Equal(t, int64(1024), result.CpuShares)
	})

	t.Run("memory only", func(t *testing.T) {
		resources := &model.ResourceSpec{
			Memory: "512MB",
		}

		result := ResourcesToLinuxResources(resources)
		require.NotNil(t, result)
		assert.Equal(t, int64(512*1024*1024), result.MemoryLimitInBytes)
		assert.Equal(t, int64(0), result.CpuShares)
	})

	t.Run("empty resources", func(t *testing.T) {
		resources := &model.ResourceSpec{}

		result := ResourcesToLinuxResources(resources)
		require.NotNil(t, result)
		assert.Equal(t, int64(0), result.MemoryLimitInBytes)
	})
}

func TestEnvToKeyValues(t *testing.T) {
	envs := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/root",
		"EMPTY=",
		"NOVALUE",
	}

	result := EnvToKeyValues(envs)
	require.Len(t, result, 4)

	assert.Equal(t, "PATH", result[0].Key)
	assert.Equal(t, "/usr/bin:/bin", result[0].Value)

	assert.Equal(t, "HOME", result[1].Key)
	assert.Equal(t, "/root", result[1].Value)

	assert.Equal(t, "EMPTY", result[2].Key)
	assert.Equal(t, "", result[2].Value)

	assert.Equal(t, "NOVALUE", result[3].Key)
	assert.Equal(t, "", result[3].Value)
}

func TestEnvToKeyValues_Empty(t *testing.T) {
	result := EnvToKeyValues(nil)
	assert.Nil(t, result)

	result = EnvToKeyValues([]string{})
	assert.Nil(t, result)
}

func TestBuildPodSandboxConfig(t *testing.T) {
	config := BuildPodSandboxConfig("test-agent")

	require.NotNil(t, config)
	require.NotNil(t, config.Metadata)
	assert.Equal(t, "test-agent", config.Metadata.Name)
	assert.Equal(t, "misbah", config.Metadata.Namespace)
	assert.Equal(t, "misbah-test-agent", config.Metadata.Uid)
	assert.Equal(t, uint32(0), config.Metadata.Attempt)
	assert.Equal(t, "true", config.Labels["misbah.dev/managed"])
	assert.Equal(t, "test-agent", config.Labels["misbah.dev/name"])
	require.NotNil(t, config.Linux)
}

func TestBuildContainerConfig(t *testing.T) {
	spec := &model.ContainerSpec{
		Metadata: model.ContainerMetadata{
			Name: "test-container",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/bash", "-c", "echo hello"},
			Env:     []string{"FOO=bar"},
			Cwd:     "/workspace",
		},
		Image: "docker.io/library/alpine:latest",
		Mounts: []model.MountSpec{
			{
				Type:        "bind",
				Source:      "/tmp",
				Destination: "/workspace",
				Options:     []string{"rw"},
			},
		},
		Resources: &model.ResourceSpec{
			Memory:    "1GB",
			CPUShares: 512,
		},
	}

	config := BuildContainerConfig(spec)

	require.NotNil(t, config)
	require.NotNil(t, config.Metadata)
	assert.Equal(t, "test-container", config.Metadata.Name)
	assert.Equal(t, "docker.io/library/alpine:latest", config.Image.Image)
	assert.Equal(t, []string{"/bin/bash", "-c", "echo hello"}, config.Command)
	assert.Equal(t, "/workspace", config.WorkingDir)
	require.Len(t, config.Envs, 1)
	assert.Equal(t, "FOO", config.Envs[0].Key)
	assert.Equal(t, "bar", config.Envs[0].Value)
	require.Len(t, config.Mounts, 1)
	assert.Equal(t, "/workspace", config.Mounts[0].ContainerPath)
	assert.Equal(t, "true", config.Labels["misbah.dev/managed"])
	require.NotNil(t, config.Linux)
	require.NotNil(t, config.Linux.Resources)
	assert.Equal(t, int64(1*1024*1024*1024), config.Linux.Resources.MemoryLimitInBytes)
	assert.Equal(t, int64(512), config.Linux.Resources.CpuShares)
}

func TestBuildContainerConfig_NoResources(t *testing.T) {
	spec := &model.ContainerSpec{
		Metadata: model.ContainerMetadata{
			Name: "test-container",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/bash"},
			Cwd:     "/workspace",
		},
		Image: "alpine:latest",
	}

	config := BuildContainerConfig(spec)
	assert.Nil(t, config.Linux)
}

func TestParseMemoryToBytes(t *testing.T) {
	tests := []struct {
		spec    string
		want    int64
		wantErr bool
	}{
		{"512KB", 512 * 1024, false},
		{"512MB", 512 * 1024 * 1024, false},
		{"2GB", 2 * 1024 * 1024 * 1024, false},
		{"1gb", 1 * 1024 * 1024 * 1024, false},
		{"2T", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			got, err := parseMemoryToBytes(tt.spec)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
