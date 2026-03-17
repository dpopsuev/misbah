//go:build integration && cri

package integration_test

import (
	"os"
	"testing"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/cri"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCRIBackendLifecycle tests the full container lifecycle against a real
// containerd instance with Kata runtime. Requires:
//   - Running containerd: systemctl start containerd
//   - Kata installed: /usr/share/kata-containers
//   - Run with: go test -tags='integration cri' ./test/integration/ -v
func TestCRIBackendLifecycle(t *testing.T) {
	endpoint := config.GetCRIEndpoint()
	handler := config.GetRuntimeHandler()

	// Check if containerd is reachable
	if _, err := os.Stat("/run/containerd/containerd.sock"); os.IsNotExist(err) {
		t.Skip("containerd not running (socket not found)")
	}

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)

	backend, err := cri.NewBackend(endpoint, handler, logger)
	if err != nil {
		t.Skipf("Failed to create CRI backend (containerd not available?): %v", err)
	}
	defer backend.Close()

	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name:   "integration-test-kata",
			Labels: map[string]string{"provider": "test"},
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh", "-c", "echo 'Hello from Kata VM' && uname -r"},
			Cwd:     "/",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Image:   "docker.io/library/alpine:latest",
		Runtime: "kata",
	}

	// 1. Start container (pulls image, creates sandbox, creates+starts container)
	t.Run("start", func(t *testing.T) {
		containerID, err := backend.Start(spec)
		require.NoError(t, err)
		assert.NotEmpty(t, containerID)
		t.Logf("Container started: %s", containerID)
	})

	// 2. Check status
	t.Run("status", func(t *testing.T) {
		info, err := backend.Status("integration-test-kata")
		require.NoError(t, err)
		assert.Equal(t, "integration-test-kata", info.Name)
		t.Logf("Container state: %s", info.State)
	})

	// 3. Exec a command
	t.Run("exec", func(t *testing.T) {
		stdout, stderr, exitCode, err := backend.Exec("integration-test-kata", []string{"echo", "exec-test"}, 30)
		require.NoError(t, err)
		assert.Equal(t, int32(0), exitCode)
		assert.Contains(t, string(stdout), "exec-test")
		t.Logf("Exec stdout: %s", string(stdout))
		if len(stderr) > 0 {
			t.Logf("Exec stderr: %s", string(stderr))
		}
	})

	// 4. List containers
	t.Run("list", func(t *testing.T) {
		infos, err := backend.List()
		require.NoError(t, err)
		t.Logf("Listed %d misbah-managed containers", len(infos))
	})

	// 5. Stop and cleanup
	t.Run("stop", func(t *testing.T) {
		err := backend.Stop("integration-test-kata", false)
		require.NoError(t, err)
		t.Log("Container stopped and cleaned up")
	})
}

// TestCRIBackendWithNetwork tests container creation with network configuration.
func TestCRIBackendWithNetwork(t *testing.T) {
	endpoint := config.GetCRIEndpoint()
	handler := config.GetRuntimeHandler()

	if _, err := os.Stat("/run/containerd/containerd.sock"); os.IsNotExist(err) {
		t.Skip("containerd not running")
	}

	logger := metrics.NewJSONLogger(metrics.LogLevelDebug)

	backend, err := cri.NewBackend(endpoint, handler, logger)
	if err != nil {
		t.Skipf("Failed to create CRI backend: %v", err)
	}
	defer backend.Close()

	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name: "integration-test-network",
		},
		Process: model.ProcessSpec{
			Command: []string{"/bin/sh", "-c", "hostname && cat /etc/resolv.conf"},
			Cwd:     "/",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
		},
		Image:   "docker.io/library/alpine:latest",
		Runtime: "kata",
		Network: &model.NetworkConfig{
			Hostname:   "misbah-agent",
			DNSServers: []string{"8.8.8.8"},
			DNSSearch:  []string{"misbah.local"},
		},
	}

	containerID, err := backend.Start(spec)
	require.NoError(t, err)
	t.Logf("Container with network config started: %s", containerID)

	// Cleanup
	err = backend.Stop("integration-test-network", true)
	require.NoError(t, err)
}
