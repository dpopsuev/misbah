package daemon

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/dpopsuev/misbah/metrics"
)

// VsockListener accepts connections from Kata VMs via vsock and forwards
// them to the host-side network proxy.
type VsockListener struct {
	proxyMgr *ProxyManager
	logger   *metrics.Logger
	listener net.Listener
	mu       sync.Mutex
	done     chan struct{}
}

// NewVsockListener creates a new vsock listener that routes connections
// to the appropriate per-container proxy via ProxyManager.
func NewVsockListener(proxyMgr *ProxyManager, logger *metrics.Logger) *VsockListener {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &VsockListener{
		proxyMgr: proxyMgr,
		logger:   logger,
		done:     make(chan struct{}),
	}
}

// Start begins listening on the given address (typically a vsock listener).
// For production: vsock listener. For tests: Unix or TCP listener.
func (vl *VsockListener) Start(ln net.Listener) error {
	vl.mu.Lock()
	vl.listener = ln
	vl.mu.Unlock()

	vl.logger.Infof("Vsock listener accepting on %s", ln.Addr())

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-vl.done:
				return nil
			default:
				return fmt.Errorf("vsock accept error: %w", err)
			}
		}
		go vl.handleConn(conn)
	}
}

// Stop shuts down the vsock listener.
func (vl *VsockListener) Stop(_ context.Context) error {
	close(vl.done)
	vl.mu.Lock()
	defer vl.mu.Unlock()
	if vl.listener != nil {
		return vl.listener.Close()
	}
	return nil
}

// handleConn forwards a vsock connection to the proxy.
// The first container's proxy is used (in practice, each VM maps to one container).
func (vl *VsockListener) handleConn(client net.Conn) {
	defer client.Close()

	// Get the proxy address from the proxy manager
	vl.proxyMgr.mu.Lock()
	var proxyAddr string
	for _, mp := range vl.proxyMgr.proxies {
		proxyAddr = mp.addr
		break
	}
	vl.proxyMgr.mu.Unlock()

	if proxyAddr == "" {
		vl.logger.Errorf("No proxy available for vsock connection")
		return
	}

	upstream, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		vl.logger.Errorf("Failed to connect to proxy %s: %v", proxyAddr, err)
		return
	}
	defer upstream.Close()

	done := make(chan struct{})
	go func() {
		io.Copy(upstream, client)
		close(done)
	}()
	io.Copy(client, upstream)
	<-done
}
