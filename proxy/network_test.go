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
	"strings"
	"testing"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *metrics.Logger {
	return metrics.NewLogger(metrics.LogLevelDebug, os.Stderr)
}

// testSetup creates a mock checker and network proxy.
func testSetup(t *testing.T, checker PermissionChecker) (*NetworkProxy, *http.Client, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	proxyAddr := ln.Addr().String()
	p := NewNetworkProxy(checker, "test-container", proxyAddr, testLogger())
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

	upstreamURL, _ := url.Parse(upstream.URL)
	checker := newMockChecker().withWhitelist(ResourceNetwork, extractDomain(upstreamURL.Host), DecisionAlways)

	_, client, _ := testSetup(t, checker)

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

	checker := newMockChecker() // empty whitelist, auto-deny prompter

	_, client, _ := testSetup(t, checker)

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

	checker := newMockChecker()

	_, client, _ := testSetup(t, checker)

	_, err := client.Get(upstream.URL + "/test")
	assert.Error(t, err)
}

func TestCONNECT_AllowedDomain(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "tls hello")
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	checker := newMockChecker().withWhitelist(ResourceNetwork, extractDomain(upstreamURL.Host), DecisionAlways)

	_, _, proxyAddr := testSetup(t, checker)

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
	cp := newCountingPrompter(DecisionAlways)
	checker := newMockChecker().withPrompter(cp.prompt)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	_, client, _ := testSetup(t, checker)

	resp, err := client.Get(upstream.URL + "/first")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp2, err := client.Get(upstream.URL + "/second")
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	assert.Equal(t, int32(1), cp.count.Load())
}

func TestPermissionOnce_NotCached(t *testing.T) {
	cp := newCountingPrompter(DecisionOnce)
	checker := newMockChecker().withPrompter(cp.prompt)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	_, client, _ := testSetup(t, checker)

	resp, err := client.Get(upstream.URL + "/first")
	require.NoError(t, err)
	resp.Body.Close()

	resp2, err := client.Get(upstream.URL + "/second")
	require.NoError(t, err)
	resp2.Body.Close()

	assert.Equal(t, int32(2), cp.count.Load())
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

	upstreamURL, _ := url.Parse(upstream.URL)
	checker := newMockChecker().withWhitelist(ResourceNetwork, extractDomain(upstreamURL.Host), DecisionAlways)

	_, client, _ := testSetup(t, checker)

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

	upstreamURL, _ := url.Parse(upstream.URL)
	checker := newMockChecker().withWhitelist(ResourceNetwork, extractDomain(upstreamURL.Host), DecisionAlways)

	_, client, _ := testSetup(t, checker)

	payload := `{"key": "value"}`
	resp, err := client.Post(upstream.URL+"/data", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, payload, receivedBody)
}
