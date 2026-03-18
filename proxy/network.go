package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dpopsuev/misbah/daemon"
	"github.com/dpopsuev/misbah/metrics"
)

// DefaultProxyPort is the default port for the network proxy inside containers.
const DefaultProxyPort = 8118

// Hop-by-hop headers that must not be forwarded.
var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

// NetworkProxy is an HTTP/HTTPS forward proxy with permission checking.
type NetworkProxy struct {
	client     *daemon.Client
	container  string
	listenAddr string
	logger     *metrics.Logger
	httpServer *http.Server

	mu    sync.RWMutex
	cache map[string]daemon.Decision
}

// NewNetworkProxy creates a new network proxy.
func NewNetworkProxy(client *daemon.Client, container, listenAddr string, logger *metrics.Logger) *NetworkProxy {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &NetworkProxy{
		client:     client,
		container:  container,
		listenAddr: listenAddr,
		logger:     logger,
		cache:      make(map[string]daemon.Decision),
	}
}

// Start begins listening and serving proxy requests. Blocks until stopped.
func (p *NetworkProxy) Start() error {
	p.httpServer = &http.Server{
		Addr:    p.listenAddr,
		Handler: p,
	}

	p.logger.Infof("Network proxy listening on %s", p.listenAddr)

	if err := p.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("proxy server error: %w", err)
	}
	return nil
}

// Stop gracefully shuts down the proxy.
func (p *NetworkProxy) Stop(ctx context.Context) error {
	if p.httpServer != nil {
		p.logger.Infof("Shutting down network proxy")
		return p.httpServer.Shutdown(ctx)
	}
	return nil
}

// ServeHTTP implements http.Handler. Routes to handleHTTP or handleCONNECT.
func (p *NetworkProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleCONNECT(w, r)
	} else {
		p.handleHTTP(w, r)
	}
}

func (p *NetworkProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	domain := extractDomain(host)

	decision, err := p.checkPermission(r.Context(), domain)
	if err != nil {
		p.logger.Errorf("Permission check failed for %s: %v", domain, err)
		http.Error(w, fmt.Sprintf(`{"error":"permission check failed","domain":%q}`, domain), http.StatusBadGateway)
		return
	}

	if decision != daemon.DecisionAlways && decision != daemon.DecisionOnce {
		p.logger.Infof("Blocked HTTP request to %s: %s", domain, decision)
		http.Error(w, fmt.Sprintf(`{"error":"access denied","domain":%q}`, domain), http.StatusForbidden)
		return
	}

	// Remove hop-by-hop headers
	for _, h := range hopByHopHeaders {
		r.Header.Del(h)
	}

	// Create outbound request
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}
	outReq.Header = r.Header.Clone()

	resp, err := http.DefaultTransport.RoundTrip(outReq)
	if err != nil {
		p.logger.Errorf("Upstream request failed for %s: %v", domain, err)
		http.Error(w, fmt.Sprintf(`{"error":"upstream failed","domain":%q}`, domain), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Remove hop-by-hop headers from response
	for _, h := range hopByHopHeaders {
		resp.Header.Del(h)
	}

	// Copy response headers and body
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (p *NetworkProxy) handleCONNECT(w http.ResponseWriter, r *http.Request) {
	domain := extractDomain(r.Host)

	decision, err := p.checkPermission(r.Context(), domain)
	if err != nil {
		p.logger.Errorf("Permission check failed for CONNECT %s: %v", domain, err)
		http.Error(w, "permission check failed", http.StatusBadGateway)
		return
	}

	if decision != daemon.DecisionAlways && decision != daemon.DecisionOnce {
		p.logger.Infof("Blocked CONNECT to %s: %s", domain, decision)
		http.Error(w, "access denied", http.StatusForbidden)
		return
	}

	// Resolve DNS after permission granted (prevents DNS rebinding)
	targetAddr := r.Host
	if _, _, err := net.SplitHostPort(targetAddr); err != nil {
		targetAddr = net.JoinHostPort(targetAddr, "443")
	}

	destConn, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
	if err != nil {
		p.logger.Errorf("Failed to connect to %s: %v", targetAddr, err)
		http.Error(w, "failed to connect to upstream", http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		destConn.Close()
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		destConn.Close()
		p.logger.Errorf("Hijack failed: %v", err)
		return
	}

	// Write 200 on the raw connection (can't use ResponseWriter after Hijack)
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// Bidirectional copy — when one direction finishes, close both
	done := make(chan struct{})
	go func() {
		io.Copy(destConn, clientConn)
		close(done)
	}()
	io.Copy(clientConn, destConn)
	<-done
	clientConn.Close()
	destConn.Close()
}

// checkPermission checks if a domain is allowed.
// Uses local cache first, then calls daemon.
func (p *NetworkProxy) checkPermission(ctx context.Context, domain string) (daemon.Decision, error) {
	// Check local session cache
	p.mu.RLock()
	if d, ok := p.cache[domain]; ok {
		p.mu.RUnlock()
		return d, nil
	}
	p.mu.RUnlock()

	// Fast path: check whitelist
	req := daemon.PermissionRequest{
		Container:    p.container,
		ResourceType: daemon.ResourceNetwork,
		ResourceID:   domain,
		Description:  fmt.Sprintf("Network access to %s", domain),
	}

	resp, err := p.client.Check(ctx, req)
	if err != nil {
		return daemon.DecisionDeny, err
	}

	if resp.Decision == daemon.DecisionAlways || resp.Decision == daemon.DecisionDeny {
		p.mu.Lock()
		p.cache[domain] = resp.Decision
		p.mu.Unlock()
		return resp.Decision, nil
	}

	// Full flow: prompt user
	resp, err = p.client.Request(ctx, req)
	if err != nil {
		return daemon.DecisionDeny, err
	}

	// Cache always/deny decisions; once is NOT cached
	if resp.Decision == daemon.DecisionAlways || resp.Decision == daemon.DecisionDeny {
		p.mu.Lock()
		p.cache[domain] = resp.Decision
		p.mu.Unlock()
	}

	return resp.Decision, nil
}

// extractDomain extracts the domain (without port) from a host:port string.
func extractDomain(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

// ProxyEnvVars returns the environment variables to set inside the container
// for routing traffic through the network proxy.
func ProxyEnvVars(proxyAddr, daemonSocketPath string) []string {
	proxyURL := "http://" + proxyAddr
	noProxy := "localhost,127.0.0.1," + daemonSocketPath
	return []string{
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
		"NO_PROXY=" + noProxy,
		"no_proxy=" + noProxy,
	}
}
