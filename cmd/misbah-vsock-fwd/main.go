// misbah-vsock-fwd is a tiny TCP-to-vsock forwarder for inside Kata VMs.
// Agent sets HTTP_PROXY=http://127.0.0.1:8118, traffic flows:
// agent → forwarder (TCP) → vsock → host proxy → permission check.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/proxy"
)

func main() {
	listenAddr := flag.String("listen", fmt.Sprintf("127.0.0.1:%d", proxy.DefaultProxyPort), "Local TCP listen address")
	upstreamNet := flag.String("net", "vsock", "Upstream network type (vsock or unix)")
	upstreamAddr := flag.String("upstream", "", "Upstream address (vsock CID:port or Unix socket path)")
	flag.Parse()

	if *upstreamAddr == "" {
		fmt.Fprintln(os.Stderr, "Usage: misbah-vsock-fwd --upstream <addr> [--listen host:port] [--net vsock|unix]")
		os.Exit(1)
	}

	logger := metrics.NewLogger(metrics.LogLevelInfo, os.Stderr)
	fwd := proxy.NewVsockForwarder(*listenAddr, *upstreamNet, *upstreamAddr, logger)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fwd.Stop(context.Background())
	}()

	if err := fwd.Start(); err != nil {
		logger.Errorf("Forwarder error: %v", err)
		os.Exit(1)
	}
}
