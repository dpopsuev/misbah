package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVsockForwarder_ForwardsTraffic(t *testing.T) {
	// Create a mock upstream (simulates the host-side proxy) on a Unix socket
	upstreamDir := t.TempDir()
	upstreamSock := filepath.Join(upstreamDir, "upstream.sock")

	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "forwarded-ok")
	}))

	upstreamLn, err := net.Listen("unix", upstreamSock)
	require.NoError(t, err)
	upstream.Listener = upstreamLn
	upstream.Start()
	defer upstream.Close()

	// Create forwarder: TCP → Unix socket (simulating vsock)
	fwdLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	fwdAddr := fwdLn.Addr().String()

	fwd := NewVsockForwarder(fwdAddr, "unix", upstreamSock, testLogger())

	go fwd.StartOnListener(fwdLn)
	defer fwd.Stop(context.Background())

	// Send HTTP request through forwarder, disable keepalive to avoid stuck connections
	transport := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()

	resp, err := client.Get("http://" + fwdAddr + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "forwarded-ok", string(body))
}

func TestVsockForwarder_UpstreamUnavailable(t *testing.T) {
	// Forwarder points to a non-existent upstream
	fwdLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	fwdAddr := fwdLn.Addr().String()

	fwd := NewVsockForwarder(fwdAddr, "unix", "/tmp/nonexistent-"+t.Name()+".sock", testLogger())

	go fwd.StartOnListener(fwdLn)
	defer fwd.Stop(context.Background())

	// Connection should be accepted but upstream dial fails → connection closed
	conn, err := net.DialTimeout("tcp", fwdAddr, time.Second)
	require.NoError(t, err)
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	assert.Error(t, err) // EOF or connection reset
}

func TestVsockForwarder_ProxyEndToEnd(t *testing.T) {
	// Full chain: client → forwarder → proxy (on Unix socket) → upstream HTTP server

	// 1. Upstream HTTP server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "e2e-ok")
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	domain := upstreamURL.Hostname()

	// 2. Network proxy on Unix socket (simulates host-side proxy)
	checker := newMockChecker().withWhitelist(ResourceNetwork, domain, DecisionAlways)
	proxyDir := t.TempDir()
	proxySock := filepath.Join(proxyDir, "proxy.sock")

	proxyLn, err := net.Listen("unix", proxySock)
	require.NoError(t, err)

	p := NewNetworkProxy(checker, "test-container", proxySock, testLogger())
	go p.StartOnListener(proxyLn)
	defer p.Stop(context.Background())

	// 3. Forwarder: TCP → Unix socket (simulates vsock bridge)
	fwdLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	fwdAddr := fwdLn.Addr().String()

	fwd := NewVsockForwarder(fwdAddr, "unix", proxySock, testLogger())
	go fwd.StartOnListener(fwdLn)
	defer fwd.Stop(context.Background())

	// 4. Client uses forwarder as HTTP proxy, disable keepalive
	proxyURL, _ := url.Parse("http://" + fwdAddr)
	transport := &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: transport}
	defer transport.CloseIdleConnections()

	resp, err := client.Get(upstream.URL + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "e2e-ok", string(body))
}

func TestVsockForwarder_Stop(t *testing.T) {
	fwdLn, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	fwd := NewVsockForwarder(fwdLn.Addr().String(), "unix", "/dev/null", testLogger())

	errCh := make(chan error, 1)
	go func() {
		errCh <- fwd.StartOnListener(fwdLn)
	}()

	// Wait for goroutine to start accepting
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, fwd.Stop(context.Background()))

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("forwarder did not stop in time")
	}
}
