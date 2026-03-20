//go:build e2e && claude

package e2e_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dpopsuev/misbah/test/harness"
	"github.com/stretchr/testify/require"
)

// TestClaudeNamespaceProxy runs Claude Code inside a namespace container
// with the proxy enforcing network whitelist.
// Requires: claude binary in PATH, MISBAH_E2E_CLAUDE=true.
func TestClaudeNamespaceProxy(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	lab := harness.NewLab(t)
	specFile := filepath.Join(repoRoot(t), "test", "e2e", "specs", "claude-namespace.yaml")

	output, err := lab.RunMisbah("container", "start", "--spec", specFile)
	require.NoError(t, err, "container start failed: %s", output)

	t.Run("proxy_env_set", func(t *testing.T) {
		require.True(t, strings.Contains(output, "HTTP_PROXY=http://127.0.0.1:"),
			"HTTP_PROXY not set in container output:\n%s", output)
	})

	t.Run("whitelisted_domain_allowed", func(t *testing.T) {
		// oauth2.googleapis.com returns 404 for the well-known path, but proxy allows it
		require.True(t,
			strings.Contains(output, "Result: 404") || strings.Contains(output, "Result: 200"),
			"whitelisted domain not allowed:\n%s", output)
	})

	t.Run("non_whitelisted_domain_blocked", func(t *testing.T) {
		require.Contains(t, output, "Result: 403",
			"non-whitelisted domain not blocked:\n%s", output)
	})

	t.Run("claude_responds", func(t *testing.T) {
		require.Contains(t, output, "MISBAH_OK",
			"Claude did not respond with MISBAH_OK:\n%s", output)
	})

	harness.AssertNoStaleState(t, lab)
}

// TestClaudeKataProxy runs a test container inside a Kata VM with the
// daemon managing the proxy. Requires: daemon running, Kata + containerd.
func TestClaudeKataProxy(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("E2E tests require Linux")
	}

	lab := harness.NewLab(t)
	specFile := filepath.Join(repoRoot(t), "test", "e2e", "specs", "claude-kata.yaml")

	output, err := lab.RunMisbah("container", "start", "--spec", specFile)
	require.NoError(t, err, "container start failed: %s", output)

	t.Run("kata_container_started", func(t *testing.T) {
		require.Contains(t, output, "started",
			"Kata container did not start:\n%s", output)
	})

	harness.AssertNoStaleState(t, lab)
}
