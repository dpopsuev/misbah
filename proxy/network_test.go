package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/misbah/daemon"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *metrics.Logger {
	return metrics.NewLogger(metrics.LogLevelDebug, os.Stderr)
}

// testSetup creates a daemon server, daemon client, and network proxy.
// Returns the proxy, an http.Client configured to use it, and the proxy address.
func testSetup(t *testing.T, whitelist *daemon.WhitelistStore, prompter daemon.Prompter) (*NetworkProxy, *http.Client, string) {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "daemon.sock")
	logger := testLogger()

	audit := daemon.NewAuditLoggerFromWriter(io.Discard, logger)
	daemonServer := daemon.NewServer(whitelist, prompter, audit, logger)

	daemonReady := make(chan struct{})
	go func() {
		close(daemonReady)
		daemonServer.Start(socketPath)
	}()
	<-daemonReady
	time.Sleep(10 * time.Millisecond)
	t.Cleanup(func() { daemonServer.Stop() })

	daemonClient := daemon.NewClient(socketPath, logger)
	t.Cleanup(func() { daemonClient.Close() })

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyAddr := ln.Addr().String()
	p := NewNetworkProxy(daemonClient, "test-container", proxyAddr, logger)
	p.httpServer = &http.Server{Handler: p}

	go func() { p.httpServer.Serve(ln) }()
	t.Cleanup(func() { p.Stop(context.Background()) })

	proxyURL, _ := url.Parse("http://" + proxyAddr)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	return p, client, proxyAddr
}

func TestHTTPProxy_AllowedDomain(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from upstream"))
	}))
	defer upstream.Close()

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	upstreamURL, _ := url.Parse(upstream.URL)
	whitelist.Set(daemon.ResourceNetwork, extractDomain(upstreamURL.Host), daemon.DecisionAlways)

	_, client, _ := testSetup(t, whitelist, &daemon.AutoDenyPrompter{})

	resp, err := client.Get(upstream.URL + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "hello from upstream", string(body))
}

func TestHTTPProxy_DeniedDomain(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach upstream")
	}))
	defer upstream.Close()

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client, _ := testSetup(t, whitelist, &daemon.AutoDenyPrompter{})

	resp, err := client.Get(upstream.URL + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestCONNECT_DeniedDomain(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach upstream")
	}))
	defer upstream.Close()

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client, _ := testSetup(t, whitelist, &daemon.AutoDenyPrompter{})

	_, err := client.Get(upstream.URL + "/test")
	assert.Error(t, err)
}

func TestCONNECT_AllowedDomain(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "tls hello")
	}))
	defer upstream.Close()

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	upstreamURL, _ := url.Parse(upstream.URL)
	whitelist.Set(daemon.ResourceNetwork, extractDomain(upstreamURL.Host), daemon.DecisionAlways)

	_, _, proxyAddr := testSetup(t, whitelist, &daemon.AutoDenyPrompter{})

	proxyURL, _ := url.Parse("http://" + proxyAddr)
	tlsConfig := upstream.Client().Transport.(*http.Transport).TLSClientConfig
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: tlsConfig,
		},
	}

	resp, err := client.Get(upstream.URL + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "tls hello", string(body))
}

func TestPermissionCaching(t *testing.T) {
	var count atomic.Int32
	prompter := &countingFixedPrompter{decision: daemon.DecisionAlways, count: &count}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client, _ := testSetup(t, whitelist, prompter)

	resp, err := client.Get(upstream.URL + "/first")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp2, err := client.Get(upstream.URL + "/second")
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	// Prompter only called once — second request was cached
	assert.Equal(t, int32(1), count.Load())
}

func TestPermissionOnce_NotCached(t *testing.T) {
	var count atomic.Int32
	prompter := &countingFixedPrompter{decision: daemon.DecisionOnce, count: &count}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	_, client, _ := testSetup(t, whitelist, prompter)

	resp, err := client.Get(upstream.URL + "/first")
	require.NoError(t, err)
	resp.Body.Close()

	resp2, err := client.Get(upstream.URL + "/second")
	require.NoError(t, err)
	resp2.Body.Close()

	// Prompter called twice — Once is not cached
	assert.Equal(t, int32(2), count.Load())
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com:443", "example.com"},
		{"example.com:80", "example.com"},
		{"example.com", "example.com"},
		{"[::1]:8080", "::1"},
		{"127.0.0.1:8080", "127.0.0.1"},
		{"sub.domain.com:9090", "sub.domain.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, extractDomain(tt.input))
		})
	}
}

func TestProxyEnvVars(t *testing.T) {
	envs := ProxyEnvVars("127.0.0.1:8118", "/run/misbah/permission.sock")

	assert.Contains(t, envs, "HTTP_PROXY=http://127.0.0.1:8118")
	assert.Contains(t, envs, "HTTPS_PROXY=http://127.0.0.1:8118")
	assert.Contains(t, envs, "http_proxy=http://127.0.0.1:8118")
	assert.Contains(t, envs, "https_proxy=http://127.0.0.1:8118")
	assert.Contains(t, envs, "NO_PROXY=localhost,127.0.0.1,/run/misbah/permission.sock")
	assert.Contains(t, envs, "no_proxy=localhost,127.0.0.1,/run/misbah/permission.sock")
}

func TestHTTPProxy_HopByHopHeaders(t *testing.T) {
	var receivedHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Custom", "value")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	upstreamURL, _ := url.Parse(upstream.URL)
	whitelist.Set(daemon.ResourceNetwork, extractDomain(upstreamURL.Host), daemon.DecisionAlways)

	_, client, _ := testSetup(t, whitelist, &daemon.AutoDenyPrompter{})

	req, _ := http.NewRequest("GET", upstream.URL+"/test", nil)
	req.Header.Set("Proxy-Authorization", "secret")
	req.Header.Set("X-Request-Id", "123")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, receivedHeaders.Get("Proxy-Authorization"))
	assert.Equal(t, "123", receivedHeaders.Get("X-Request-Id"))
	assert.Empty(t, resp.Header.Get("Connection"))
	assert.Equal(t, "value", resp.Header.Get("X-Custom"))
}

func TestHTTPProxy_PreservesRequestBody(t *testing.T) {
	var receivedBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	whitelist := daemon.NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	upstreamURL, _ := url.Parse(upstream.URL)
	whitelist.Set(daemon.ResourceNetwork, extractDomain(upstreamURL.Host), daemon.DecisionAlways)

	_, client, _ := testSetup(t, whitelist, &daemon.AutoDenyPrompter{})

	payload := `{"key": "value"}`
	resp, err := client.Post(upstream.URL+"/data", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, payload, receivedBody)
}

// countingFixedPrompter returns a fixed decision and counts calls.
type countingFixedPrompter struct {
	decision daemon.Decision
	count    *atomic.Int32
}

func (c *countingFixedPrompter) Prompt(req *daemon.PermissionRequest) (daemon.Decision, error) {
	c.count.Add(1)
	return c.decision, nil
}
