//go:build e2e && netns

package daemon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/dpopsuev/misbah/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkIsolator_IPTablesDrop(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}

	bridge := "misbah-t0"
	subnet := "10.99.0.0/24"
	gatewayIP := "10.99.0.1"

	cfg := config.NetworkSection{
		Bridge: bridge,
		Subnet: subnet,
		MTU:    1500,
	}
	ni := NewNetworkIsolator(cfg, testLogger())

	// Start a TCP listener on the gateway IP to simulate the proxy.
	proxyLn, err := net.Listen("tcp", gatewayIP+":0")
	require.NoError(t, err)
	defer proxyLn.Close()

	proxyAddr := proxyLn.Addr().String()
	_, proxyPort, _ := net.SplitHostPort(proxyAddr)

	// Accept connections in background so nc can succeed
	go func() {
		for {
			conn, err := proxyLn.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	nsName, err := ni.Setup("drop-test", proxyAddr)
	require.NoError(t, err)
	defer func() {
		ni.Teardown("drop-test")
		// Best-effort bridge cleanup
		exec.Command("ip", "link", "delete", bridge).Run()
	}()

	t.Run("proxy_reachable", func(t *testing.T) {
		// nc should connect to the proxy address
		out, err := nsExec(nsName, "nc", "-z", "-w", "3", gatewayIP, proxyPort)
		assert.NoError(t, err, "proxy should be reachable: %s", out)
	})

	t.Run("arbitrary_blocked", func(t *testing.T) {
		// nc to a non-proxy port on the gateway should be blocked (DROP = timeout)
		_, err := nsExec(nsName, "nc", "-z", "-w", "2", gatewayIP, "12345")
		assert.Error(t, err, "arbitrary port should be blocked by iptables DROP")
	})

	t.Run("direct_curl_blocked", func(t *testing.T) {
		// Direct curl without proxy should fail
		_, err := nsExec(nsName, "curl", "--connect-timeout", "3", "-s", "-o", "/dev/null", "http://1.1.1.1")
		assert.Error(t, err, "direct curl to external IP should be blocked")
	})

	t.Run("iptables_rules_present", func(t *testing.T) {
		out, err := nsExec(nsName, "iptables", "-L", "OUTPUT", "-n")
		require.NoError(t, err, "iptables -L should succeed: %s", out)
		assert.Contains(t, out, "DROP", "OUTPUT chain should contain DROP rule")
		assert.Contains(t, out, "dpt:53", "OUTPUT chain should allow DNS")
		assert.Contains(t, out, fmt.Sprintf("dpt:%s", proxyPort), "OUTPUT chain should allow proxy port")
	})
}

// nsExec runs a command inside the named network namespace.
func nsExec(nsName string, args ...string) (string, error) {
	cmdArgs := append([]string{"netns", "exec", nsName}, args...)
	cmd := exec.Command("ip", cmdArgs...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
