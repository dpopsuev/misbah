package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/cri"
	"github.com/dpopsuev/misbah/daemon"
	"github.com/dpopsuev/misbah/runtime"
	"github.com/spf13/cobra"
)

var (
	daemonSocket         string
	daemonNonInteractive bool
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the permission broker daemon",
	Long: `Manage the permission broker daemon for progressive trust.

The daemon runs on the host and brokers all permission requests from proxies
inside Kata VMs. Unknown resources prompt the user with [Once]/[Always]/[Deny].

Available subcommands:
  start - Start the permission daemon

Examples:
  misbah daemon start
  misbah daemon start --non-interactive
  misbah daemon start --socket /tmp/misbah/permission.sock`,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the permission daemon",
	Long: `Start the permission daemon listening on a Unix socket.

The daemon:
  1. Loads the whitelist from disk
  2. Listens for permission requests on a Unix socket
  3. Checks the whitelist for known resources
  4. Prompts the user for unknown resources (unless --non-interactive)
  5. Persists ALWAYS/DENY decisions to the whitelist
  6. Logs all decisions to the audit log

Examples:
  misbah daemon start
  misbah daemon start --non-interactive
  misbah daemon start --socket /run/misbah/permission.sock`,
	RunE: runDaemonStart,
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)

	daemonStartCmd.Flags().StringVar(&daemonSocket, "socket", "", "Unix socket path (default: "+config.DefaultDaemonSocket+")")
	daemonStartCmd.Flags().BoolVar(&daemonNonInteractive, "non-interactive", false, "Auto-deny all unknown resources")

	rootCmd.AddCommand(daemonCmd)
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	socketPath := daemonSocket
	if socketPath == "" {
		socketPath = config.GetDaemonSocket()
	}

	// Set up whitelist
	whitelist := daemon.NewWhitelistStore(config.GetWhitelistPath(), logger)
	if err := whitelist.Load(); err != nil {
		logger.Warnf("Failed to load whitelist: %v", err)
	}

	// Set up audit logger
	audit, err := daemon.NewAuditLogger(config.GetAuditLogPath(), logger)
	if err != nil {
		return err
	}
	defer audit.Close()

	// Set up prompter
	var prompter daemon.Prompter
	if daemonNonInteractive {
		logger.Infof("Running in non-interactive mode (auto-deny)")
		prompter = &daemon.AutoDenyPrompter{}
	} else {
		prompter = daemon.NewTerminalPrompter()
	}

	// Initialize CRI backend for Kata containers
	var opts []daemon.ServerOption

	endpoint := config.GetCRIEndpoint()
	handler := config.GetRuntimeHandler()

	criBackend, criErr := cri.NewBackend(endpoint, handler, logger)
	if criErr != nil {
		logger.Warnf("CRI backend unavailable: %v (Kata containers disabled)", criErr)
	} else {
		lifecycle := runtime.NewLifecycle(logger, recorder, criBackend)
		opts = append(opts, daemon.WithLifecycle(lifecycle))
		defer criBackend.Close()
		logger.Infof("CRI backend initialized (endpoint=%s, handler=%s)", endpoint, handler)
	}

	server := daemon.NewServer(whitelist, prompter, audit, logger, opts...)

	// Graceful shutdown on SIGTERM/SIGINT
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Infof("Received shutdown signal")
		server.Stop()
	}()

	return server.Start(socketPath)
}
