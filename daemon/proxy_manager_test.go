package daemon

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyManager_StartAndStop(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())
	defer pm.StopAll()

	addr, err := pm.Start("test-container")
	require.NoError(t, err)
	assert.NotEmpty(t, addr)

	require.NoError(t, pm.Stop("test-container"))
}

func TestProxyManager_DuplicateStart(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())
	defer pm.StopAll()

	_, err := pm.Start("test")
	require.NoError(t, err)

	_, err = pm.Start("test")
	assert.Error(t, err)
}

func TestProxyManager_StopNonexistent(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())

	err := pm.Stop("nonexistent")
	assert.Error(t, err)
}

func TestProxyManager_ProxyDeniesUnknownDomain(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach upstream")
	}))
	defer upstream.Close()

	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())
	defer pm.StopAll()

	addr, err := pm.Start("test")
	require.NoError(t, err)

	proxyURL, _ := url.Parse("http://" + addr)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}

	resp, err := client.Get(upstream.URL + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestProxyManager_ProxyAllowsWhitelistedDomain(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()

	upstreamURL, _ := url.Parse(upstream.URL)
	domain := upstreamURL.Hostname()

	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	whitelist.Set(ResourceNetwork, domain, DecisionAlways)

	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())
	defer pm.StopAll()

	addr, err := pm.Start("test")
	require.NoError(t, err)

	proxyURL, _ := url.Parse("http://" + addr)
	client := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}

	resp, err := client.Get(upstream.URL + "/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))
}

func TestProxyManager_StopAll(t *testing.T) {
	whitelist := NewWhitelistStore(filepath.Join(t.TempDir(), "wl.yaml"), testLogger())
	checker := NewDirectChecker(whitelist, &AutoDenyPrompter{}, nil, testLogger())
	pm := NewProxyManager(checker, testLogger())

	_, err := pm.Start("container-1")
	require.NoError(t, err)

	_, err = pm.Start("container-2")
	require.NoError(t, err)

	pm.StopAll()

	assert.Error(t, pm.Stop("container-1"))
	assert.Error(t, pm.Stop("container-2"))
}
