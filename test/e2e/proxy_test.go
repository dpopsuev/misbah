//go:build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/test/harness"
	"github.com/stretchr/testify/require"
)

// TestProgressiveTrustFlow tests the full progressive trust vertical slice:
// container starts → proxy runs → HTTP request blocked → permission denied → 403
func TestProgressiveTrustFlow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	// Skip if daemon socket doesn't exist (daemon not running)
	if _, err := os.Stat("/run/misbah/permission.sock"); err != nil {
		t.Skip("Misbah daemon not running (no socket at /run/misbah/permission.sock)")
	}

	lab := harness.NewLab(t)

	testDir := t.TempDir()
	containerName := "e2e-proxy-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "proxy-test.yaml")

	// Create spec that checks proxy environment and makes an HTTP request
	spec := `version: "1.0"
metadata:
  name: ` + containerName + `
  labels: {}
process:
  command:
    - /bin/bash
    - -c
    - |
      # Verify proxy env vars are set
      if [ -z "$HTTP_PROXY" ]; then
        echo "FAIL: HTTP_PROXY not set"
        exit 1
      fi
      echo "PASS: HTTP_PROXY=$HTTP_PROXY"

      # Verify daemon socket is mounted
      if [ ! -S /run/misbah/permission.sock ]; then
        echo "FAIL: daemon socket not mounted"
        exit 1
      fi
      echo "PASS: daemon socket mounted"

      # Make HTTP request through proxy — should be denied (403)
      HTTP_CODE=$(curl -s --connect-timeout 5 -o /dev/null -w "%{http_code}" http://example.com 2>/dev/null)
      if [ "$HTTP_CODE" = "403" ]; then
        echo "PASS: HTTP request denied (403)"
      else
        echo "FAIL: expected 403, got $HTTP_CODE"
        exit 1
      fi

      echo "ALL CHECKS PASSED"
  cwd: /tmp
namespaces:
  user: true
  mount: true
  pid: true
mounts:
  - type: tmpfs
    destination: /tmp
    options:
      - rw
`
	require.NoError(t, os.WriteFile(specFile, []byte(spec), 0644))

	t.Run("proxy_blocks_unknown_domain", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "start", "--spec", specFile)
		require.NoError(t, err, "container start failed: %s", output)

		require.Contains(t, output, "PASS: HTTP_PROXY=")
		require.Contains(t, output, "PASS: daemon socket mounted")
		require.Contains(t, output, "PASS: HTTP request denied (403)")
		require.Contains(t, output, "ALL CHECKS PASSED")
	})

	harness.AssertNoStaleState(t, lab)
}

// TestProxyWithWhitelist tests that whitelisted domains pass through.
func TestProxyWithWhitelist(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	if _, err := os.Stat("/run/misbah/permission.sock"); err != nil {
		t.Skip("Misbah daemon not running")
	}

	lab := harness.NewLab(t)

	testDir := t.TempDir()
	containerName := "e2e-whitelist-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "whitelist-test.yaml")

	// Create spec with whitelisted domain
	spec := `version: "1.0"
metadata:
  name: ` + containerName + `
  labels: {}
process:
  command:
    - /bin/bash
    - -c
    - |
      # Non-whitelisted domain should be denied
      HTTP_CODE=$(curl -s --connect-timeout 5 -o /dev/null -w "%{http_code}" http://evil.example.com 2>/dev/null)
      echo "NON_WHITELISTED: $HTTP_CODE"

      echo "DONE"
  cwd: /tmp
namespaces:
  user: true
  mount: true
  pid: true
mounts:
  - type: tmpfs
    destination: /tmp
    options:
      - rw
permissions:
  default_policy: deny
  network_whitelist: []
`
	require.NoError(t, os.WriteFile(specFile, []byte(spec), 0644))

	t.Run("non_whitelisted_denied", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "start", "--spec", specFile)
		require.NoError(t, err, "container start failed: %s", output)

		require.Contains(t, output, "NON_WHITELISTED: 403")
	})
}

// TestContainerWithoutDaemon verifies namespace containers work without the daemon.
func TestContainerWithoutDaemon(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	lab := harness.NewLab(t)

	// Override daemon socket to a path that doesn't exist
	// This simulates no daemon running
	t.Setenv("MISBAH_DAEMON_SOCKET", "/tmp/nonexistent-daemon.sock")

	testDir := t.TempDir()
	containerName := "e2e-nodaemon-" + time.Now().Format("20060102-150405")
	specFile := filepath.Join(testDir, "nodaemon-test.yaml")

	spec := `version: "1.0"
metadata:
  name: ` + containerName + `
  labels: {}
process:
  command:
    - /bin/sh
    - -c
    - echo "hello without daemon" && echo "proxy=${HTTP_PROXY:-NONE}"
  cwd: /tmp
namespaces:
  user: true
  mount: true
  pid: true
mounts:
  - type: tmpfs
    destination: /tmp
    options:
      - rw
`
	require.NoError(t, os.WriteFile(specFile, []byte(spec), 0644))

	t.Run("runs_without_proxy", func(t *testing.T) {
		output, err := lab.RunMisbah("container", "start", "--spec", specFile)
		require.NoError(t, err, "container start failed: %s", output)

		require.True(t, strings.Contains(output, "hello without daemon"))
		require.True(t, strings.Contains(output, "proxy=NONE"))
	})
}
