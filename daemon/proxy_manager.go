package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/proxy"
)

type managedProxy struct {
	server *http.Server
	addr   string
}

// ProxyManager manages per-container network proxy lifecycle.
type ProxyManager struct {
	checker  proxy.PermissionChecker
	isolator *NetworkIsolator
	logger   *metrics.Logger
	mu       sync.Mutex
	proxies  map[string]*managedProxy
}

// NewProxyManager creates a new proxy manager.
// If isolator is non-nil, per-container network namespaces are created.
func NewProxyManager(checker proxy.PermissionChecker, logger *metrics.Logger, isolator ...*NetworkIsolator) *ProxyManager {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	pm := &ProxyManager{
		checker: checker,
		logger:  logger,
		proxies: make(map[string]*managedProxy),
	}
	if len(isolator) > 0 {
		pm.isolator = isolator[0]
	}
	return pm
}

// Start launches a network proxy for the given container.
// Returns the listen address (host:port) for setting HTTP_PROXY.
func (pm *ProxyManager) Start(containerName string) (string, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.proxies[containerName]; exists {
		return "", fmt.Errorf("proxy already running for container %s", containerName)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to listen: %w", err)
	}
	addr := ln.Addr().String()

	p := proxy.NewNetworkProxy(pm.checker, containerName, addr, pm.logger)

	// Create server before goroutine to avoid race with Stop
	srv := &http.Server{Handler: p}

	pm.proxies[containerName] = &managedProxy{
		server: srv,
		addr:   addr,
	}

	go func() {
		pm.logger.Infof("Network proxy listening on %s", addr)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			pm.logger.Errorf("Proxy for %s exited: %v", containerName, err)
		}
	}()

	// Setup network namespace isolation if configured
	if pm.isolator != nil {
		if _, nsErr := pm.isolator.Setup(containerName, addr); nsErr != nil {
			// Proxy started but netns failed — clean up proxy
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv.Shutdown(ctx)
			delete(pm.proxies, containerName)
			return "", fmt.Errorf("network isolation failed: %w", nsErr)
		}
	}

	pm.logger.Infof("Started proxy for %s on %s", containerName, addr)
	return addr, nil
}

// Stop stops the proxy for a specific container.
func (pm *ProxyManager) Stop(containerName string) error {
	pm.mu.Lock()
	mp, ok := pm.proxies[containerName]
	if !ok {
		pm.mu.Unlock()
		return fmt.Errorf("no proxy running for container %s", containerName)
	}
	delete(pm.proxies, containerName)
	pm.mu.Unlock()

	if pm.isolator != nil {
		if err := pm.isolator.Teardown(containerName); err != nil {
			pm.logger.Warnf("Network namespace teardown for %s: %v", containerName, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mp.server.Shutdown(ctx)

	pm.logger.Infof("Stopped proxy for %s", containerName)
	return nil
}

// StopAll stops all running proxies.
func (pm *ProxyManager) StopAll() {
	pm.mu.Lock()
	snapshot := make(map[string]*managedProxy, len(pm.proxies))
	for k, v := range pm.proxies {
		snapshot[k] = v
	}
	pm.proxies = make(map[string]*managedProxy)
	pm.mu.Unlock()

	if pm.isolator != nil {
		pm.isolator.TeardownAll()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for name, mp := range snapshot {
		mp.server.Shutdown(ctx)
		pm.logger.Infof("Stopped proxy for %s", name)
	}
}
