package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDaemonConfig_Defaults(t *testing.T) {
	cfg, err := LoadDaemonConfig("/nonexistent/path.yaml")
	require.NoError(t, err)

	assert.Equal(t, DefaultDaemonSocket, cfg.Daemon.Socket)
	assert.Equal(t, "misbah", cfg.Daemon.Group)
	assert.Equal(t, "info", cfg.Daemon.LogLevel)
	assert.Equal(t, false, cfg.Daemon.NonInteractive)
	assert.Equal(t, DefaultCRIEndpoint, cfg.Kata.Endpoint)
	assert.Equal(t, "kata", cfg.Kata.Handler)
	assert.Equal(t, "none", cfg.Kata.Annotations["io.katacontainers.config.runtime.internetworking_model"])
	assert.Equal(t, "true", cfg.Kata.Annotations["io.katacontainers.config.runtime.disable_new_netns"])
}

func TestLoadDaemonConfig_FromFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "daemon.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
daemon:
  socket: /tmp/test.sock
  group: testgroup
  log_level: debug
  non_interactive: true
kata:
  endpoint: unix:///tmp/test.sock
  handler: test-handler
  annotations:
    io.katacontainers.config.runtime.internetworking_model: "tcfilter"
permissions:
  whitelist: /tmp/wl.yaml
  audit_log: /tmp/audit.log
`), 0644))

	cfg, err := LoadDaemonConfig(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "/tmp/test.sock", cfg.Daemon.Socket)
	assert.Equal(t, "testgroup", cfg.Daemon.Group)
	assert.Equal(t, "debug", cfg.Daemon.LogLevel)
	assert.True(t, cfg.Daemon.NonInteractive)
	assert.Equal(t, "unix:///tmp/test.sock", cfg.Kata.Endpoint)
	assert.Equal(t, "test-handler", cfg.Kata.Handler)
	assert.Equal(t, "tcfilter", cfg.Kata.Annotations["io.katacontainers.config.runtime.internetworking_model"])
	assert.Equal(t, "/tmp/wl.yaml", cfg.Permissions.Whitelist)
	assert.Equal(t, "/tmp/audit.log", cfg.Permissions.AuditLog)
}

func TestLoadDaemonConfig_EnvOverrides(t *testing.T) {
	t.Setenv("MISBAH_DAEMON_SOCKET", "/tmp/env-socket.sock")
	t.Setenv("MISBAH_CRI_ENDPOINT", "unix:///tmp/env-cri.sock")
	t.Setenv("MISBAH_RUNTIME_HANDLER", "env-handler")

	cfg, err := LoadDaemonConfig("/nonexistent/path.yaml")
	require.NoError(t, err)

	assert.Equal(t, "/tmp/env-socket.sock", cfg.Daemon.Socket)
	assert.Equal(t, "unix:///tmp/env-cri.sock", cfg.Kata.Endpoint)
	assert.Equal(t, "env-handler", cfg.Kata.Handler)
}

func TestLoadDaemonConfig_EnvOverridesFile(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "daemon.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`
daemon:
  socket: /from/file.sock
kata:
  endpoint: unix:///from/file.sock
`), 0644))

	t.Setenv("MISBAH_DAEMON_SOCKET", "/from/env.sock")

	cfg, err := LoadDaemonConfig(cfgPath)
	require.NoError(t, err)

	// Env wins over file
	assert.Equal(t, "/from/env.sock", cfg.Daemon.Socket)
	// File wins over default (no env override for this)
	assert.Equal(t, "unix:///from/file.sock", cfg.Kata.Endpoint)
}

func TestLoadDaemonConfig_InvalidYAML(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "daemon.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("not: valid: yaml: ["), 0644))

	_, err := LoadDaemonConfig(cfgPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}
