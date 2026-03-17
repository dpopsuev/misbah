package runtime

import (
	"runtime"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNamespaceManager(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)
	assert.NotNil(t, nm)
	assert.NotNil(t, nm.logger)
}

func TestNamespaceManagerCheckNamespaceSupport(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	err := nm.CheckNamespaceSupport()

	if runtime.GOOS != "linux" {
		// Should fail on non-Linux
		assert.Error(t, err)
	} else {
		// On Linux, may pass or fail depending on system configuration
		// We just verify it doesn't panic
		_ = err
	}
}

func TestNamespaceManagerBuildUnshareArgs(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	tests := []struct {
		name     string
		ns       model.NamespaceSpec
		expected []string
	}{
		{
			name: "minimal namespaces",
			ns: model.NamespaceSpec{
				User:  true,
				Mount: true,
			},
			expected: []string{"--user", "--map-root-user", "--mount"},
		},
		{
			name: "all namespaces",
			ns: model.NamespaceSpec{
				User:    true,
				Mount:   true,
				PID:     true,
				Network: true,
				IPC:     true,
				UTS:     true,
			},
			expected: []string{
				"--user", "--map-root-user",
				"--mount",
				"--pid", "--fork",
				"--net",
				"--ipc",
				"--uts",
			},
		},
		{
			name: "user and mount only",
			ns: model.NamespaceSpec{
				User:  true,
				Mount: true,
			},
			expected: []string{"--user", "--map-root-user", "--mount"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := nm.buildUnshareArgs(tt.ns)
			assert.Equal(t, tt.expected, args)
		})
	}
}

func TestNamespaceManagerBuildMountScript(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	mounts := []model.MountSpec{
		{
			Type:        "bind",
			Source:      "/tmp/source1",
			Destination: "/container/workspace/source1",
			Options:     []string{"rw"},
		},
		{
			Type:        "bind",
			Source:      "/tmp/source2",
			Destination: "/container/workspace/source2",
			Options:     []string{"ro"},
		},
		{
			Type:        "tmpfs",
			Destination: "/container/workspace/tmp",
		},
	}

	script := nm.buildMountScript(mounts)

	// Check bind mounts
	assert.Contains(t, script, "mkdir -p \"/container/workspace/source1\"")
	assert.Contains(t, script, "mount --bind")
	assert.Contains(t, script, "/tmp/source1")

	assert.Contains(t, script, "mkdir -p \"/container/workspace/source2\"")
	assert.Contains(t, script, "/tmp/source2")

	// Check tmpfs mount
	assert.Contains(t, script, "mkdir -p \"/container/workspace/tmp\"")
	assert.Contains(t, script, "mount -t tmpfs")
}

func TestNamespaceManagerBuildBindMount(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	mount := model.MountSpec{
		Type:        "bind",
		Source:      "/home/user/repo",
		Destination: "/container/workspace/repo",
		Options:     []string{"ro", "nosuid"},
	}

	script := nm.buildBindMount(mount)

	assert.Contains(t, script, "mkdir -p \"/container/workspace/repo\"")
	assert.Contains(t, script, "mount --bind")
	assert.Contains(t, script, "/home/user/repo")
	assert.Contains(t, script, "/container/workspace/repo")
	assert.Contains(t, script, "ro")
	assert.Contains(t, script, "nosuid")
}

func TestNamespaceManagerBuildTmpfsMount(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	mount := model.MountSpec{
		Type:        "tmpfs",
		Destination: "/container/workspace/tmp",
		Options:     []string{"size=1G"},
	}

	script := nm.buildTmpfsMount(mount)

	assert.Contains(t, script, "mkdir -p \"/container/workspace/tmp\"")
	assert.Contains(t, script, "mount -t tmpfs")
	assert.Contains(t, script, "size=1G")
}

func TestNamespaceManagerBuildProcMount(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	mount := model.MountSpec{
		Type:        "proc",
		Destination: "/container/proc",
	}

	script := nm.buildProcMount(mount)

	assert.Contains(t, script, "mkdir -p \"/container/proc\"")
	assert.Contains(t, script, "mount -t proc")
}

func TestNamespaceManagerBuildShellCommand(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	spec := &model.ContainerSpec{
		Process: model.ProcessSpec{
			Command: []string{"/usr/bin/claude", "--help"},
			Cwd:     "/container/workspace",
		},
	}

	mountScript := "# mount script here"

	cmd := nm.buildShellCommand(spec, mountScript)

	assert.Contains(t, cmd, "set -e")
	assert.Contains(t, cmd, "# mount script here")
	assert.Contains(t, cmd, "cd \"/container/workspace\"")
	assert.Contains(t, cmd, "exec /usr/bin/claude --help")
}

func TestNamespaceManagerCreateContainer_InvalidSpec(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	// Invalid spec (missing version)
	spec := &model.ContainerSpec{
		Metadata: model.ContainerMetadata{
			Name: "test-container",
		},
	}

	err := nm.CreateContainer(spec, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid container spec")
}

func TestNamespaceManagerCreateContainer_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Test requires non-Linux OS")
	}

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: "test-container",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/true"},
			Cwd:     "/tmp",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
	}

	err := nm.CreateContainer(spec, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only supported on Linux")
}

func TestNamespaceManagerCreateNamespace_DeprecationWarning(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Namespace tests require Linux")
	}

	// This test verifies the deprecated CreateNamespace still works
	// Full integration tests are in test/integration/
	t.Skip("Integration tests for CreateNamespace are in test/integration/")
}
