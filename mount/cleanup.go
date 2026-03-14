package mount

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/dpopsuev/misbah/metrics"
)

// CleanupHandler handles cleanup on signals.
type CleanupHandler struct {
	logger   *metrics.Logger
	cleanups []func() error
}

// NewCleanupHandler creates a new cleanup handler.
func NewCleanupHandler(logger *metrics.Logger) *CleanupHandler {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}

	return &CleanupHandler{
		logger:   logger,
		cleanups: make([]func() error, 0),
	}
}

// RegisterCleanup registers a cleanup function.
func (ch *CleanupHandler) RegisterCleanup(cleanup func() error) {
	ch.cleanups = append(ch.cleanups, cleanup)
}

// SetupSignalHandlers sets up signal handlers for graceful shutdown.
func (ch *CleanupHandler) SetupSignalHandlers() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigChan
		ch.logger.Infof("Received signal %s, cleaning up...", sig)

		// Run all cleanup functions
		for i, cleanup := range ch.cleanups {
			if err := cleanup(); err != nil {
				ch.logger.Errorf("Cleanup function %d failed: %v", i, err)
			}
		}

		os.Exit(0)
	}()
}

// RunCleanup runs all registered cleanup functions.
func (ch *CleanupHandler) RunCleanup() {
	ch.logger.Debugf("Running %d cleanup functions", len(ch.cleanups))

	for i, cleanup := range ch.cleanups {
		if err := cleanup(); err != nil {
			ch.logger.Errorf("Cleanup function %d failed: %v", i, err)
		}
	}
}
