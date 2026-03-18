package runtime

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestBuildMountScript_DaemonSocket(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	// Create a fake socket file
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	require.NoError(t, os.WriteFile(socketPath, []byte{}, 0644))

	t.Setenv("MISBAH_DAEMON_SOCKET", socketPath)

	script := nm.buildMountScript(nil)

	assert.Contains(t, script, fmt.Sprintf("touch \"%s\"", socketPath))
	assert.Contains(t, script, fmt.Sprintf("mount --bind \"%s\" \"%s\"", socketPath, socketPath))
}

func TestBuildMountScript_NoDaemonSocket(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	t.Setenv("MISBAH_DAEMON_SOCKET", "/nonexistent/path/daemon.sock")

	script := nm.buildMountScript(nil)

	assert.NotContains(t, script, "mount --bind")
	assert.NotContains(t, script, "daemon.sock")
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
	assert.Contains(t, cmd, "exec '/usr/bin/claude' '--help'")
}

func TestBuildShellCommand_BashDashC(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	spec := &model.ContainerSpec{
		Process: model.ProcessSpec{
			Command: []string{"/bin/bash", "-c", "echo hello && sleep 5"},
			Cwd:     "/tmp",
		},
	}

	cmd := nm.buildShellCommand(spec, "")
	// The multi-word -c argument must be preserved as a single quoted string
	assert.Contains(t, cmd, "'/bin/bash' '-c' 'echo hello && sleep 5'")
}

func TestBuildShellCommand_ArgsWithSpaces(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	spec := &model.ContainerSpec{
		Process: model.ProcessSpec{
			Command: []string{"prog", "arg with spaces", "another arg"},
			Cwd:     "/tmp",
		},
	}

	cmd := nm.buildShellCommand(spec, "")
	assert.Contains(t, cmd, "'prog' 'arg with spaces' 'another arg'")
}

func TestBuildShellCommand_SingleQuoteInArg(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	spec := &model.ContainerSpec{
		Process: model.ProcessSpec{
			Command: []string{"echo", "it's working"},
			Cwd:     "/tmp",
		},
	}

	cmd := nm.buildShellCommand(spec, "")
	// Single quote in arg must be escaped: it's → 'it'\''s working'
	assert.Contains(t, cmd, "'echo' 'it'\\''s working'")
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"with spaces", "'with spaces'"},
		{"it's", "'it'\\''s'"},
		{"", "''"},
		{"--flag=value", "'--flag=value'"},
		{"hello world", "'hello world'"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, shellQuote(tt.input))
		})
	}
}

func TestBuildProxyScript_DaemonAvailable(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	// Create fake daemon socket
	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	require.NoError(t, os.WriteFile(socketPath, []byte{}, 0644))
	t.Setenv("MISBAH_DAEMON_SOCKET", socketPath)

	// Create fake misbah-proxy binary
	proxyBin := filepath.Join(t.TempDir(), "misbah-proxy")
	require.NoError(t, os.WriteFile(proxyBin, []byte("#!/bin/sh\n"), 0755))
	// Add to PATH
	t.Setenv("PATH", filepath.Dir(proxyBin)+":"+os.Getenv("PATH"))

	spec := &model.ContainerSpec{
		Metadata: model.ContainerMetadata{Name: "test-container"},
	}

	script := nm.buildProxyScript(spec)

	assert.Contains(t, script, "misbah-proxy")
	assert.Contains(t, script, "HTTP_PROXY=http://127.0.0.1:8118")
	assert.Contains(t, script, "HTTPS_PROXY=http://127.0.0.1:8118")
	assert.Contains(t, script, "NO_PROXY=localhost,127.0.0.1")
	assert.Contains(t, script, "test-container")
}

func TestBuildProxyScript_NoDaemon(t *testing.T) {
	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)
	nm := NewNamespaceManager(logger)

	t.Setenv("MISBAH_DAEMON_SOCKET", "/nonexistent/daemon.sock")

	spec := &model.ContainerSpec{
		Metadata: model.ContainerMetadata{Name: "test-container"},
	}

	script := nm.buildProxyScript(spec)
	assert.Empty(t, script)
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

