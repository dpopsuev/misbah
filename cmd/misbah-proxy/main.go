package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dpopsuev/misbah/daemon"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/proxy"
)

func main() {
	socketPath := flag.String("socket", "/run/misbah/permission.sock", "Permission daemon socket path")
	container := flag.String("container", "", "Container name")
	listenAddr := flag.String("listen", fmt.Sprintf("127.0.0.1:%d", proxy.DefaultProxyPort), "Proxy listen address")
	flag.Parse()

	logger := metrics.NewLogger(metrics.LogLevelInfo, os.Stderr)

	client := daemon.NewClient(*socketPath, logger)
	defer client.Close()

	p := proxy.NewNetworkProxy(client, *container, *listenAddr, logger)

	// Graceful shutdown on signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		p.Stop(nil)
	}()

	if err := p.Start(); err != nil {
		logger.Errorf("Proxy error: %v", err)
		os.Exit(1)
	}
}
