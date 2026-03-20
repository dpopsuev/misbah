package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/dpopsuev/misbah/metrics"
)

// VsockForwarder forwards TCP connections on localhost to a vsock/Unix upstream.
// Used inside Kata VMs: agent → TCP 127.0.0.1:8118 → forwarder → vsock → host proxy.
type VsockForwarder struct {
	listenAddr   string
	upstreamAddr string
	upstreamNet  string // "unix" or "vsock"
	logger       *metrics.Logger
	listener     net.Listener
	mu           sync.Mutex
	done         chan struct{}
}

// NewVsockForwarder creates a new TCP-to-upstream forwarder.
// upstreamNet is "unix" (for testing) or "vsock" (for production).
func NewVsockForwarder(listenAddr, upstreamNet, upstreamAddr string, logger *metrics.Logger) *VsockForwarder {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &VsockForwarder{
		listenAddr:   listenAddr,
		upstreamAddr: upstreamAddr,
		upstreamNet:  upstreamNet,
		logger:       logger,
		done:         make(chan struct{}),
	}
}

// Start begins accepting TCP connections and forwarding to the upstream.
// Blocks until stopped.
func (f *VsockForwarder) Start() error {
	ln, err := net.Listen("tcp", f.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", f.listenAddr, err)
	}
	f.mu.Lock()
	f.listener = ln
	f.mu.Unlock()

	f.logger.Infof("Vsock forwarder listening on %s → %s://%s", f.listenAddr, f.upstreamNet, f.upstreamAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-f.done:
				return nil
			default:
				return fmt.Errorf("accept error: %w", err)
			}
		}
		go f.handleConn(conn)
	}
}

// StartOnListener begins forwarding on a pre-created listener (for tests).
func (f *VsockForwarder) StartOnListener(ln net.Listener) error {
	f.mu.Lock()
	f.listener = ln
	f.mu.Unlock()

	f.logger.Infof("Vsock forwarder listening on %s → %s://%s", ln.Addr(), f.upstreamNet, f.upstreamAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-f.done:
				return nil
			default:
				return fmt.Errorf("accept error: %w", err)
			}
		}
		go f.handleConn(conn)
	}
}

// Stop gracefully shuts down the forwarder.
func (f *VsockForwarder) Stop(_ context.Context) error {
	close(f.done)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listener != nil {
		return f.listener.Close()
	}
	return nil
}

func (f *VsockForwarder) handleConn(client net.Conn) {
	defer client.Close()

	upstream, err := net.Dial(f.upstreamNet, f.upstreamAddr)
	if err != nil {
		f.logger.Errorf("Failed to connect to upstream %s://%s: %v", f.upstreamNet, f.upstreamAddr, err)
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
