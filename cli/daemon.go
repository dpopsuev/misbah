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
	daemonConfigPath string
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the Misbah daemon",
	Long: `Manage the Misbah daemon — the unified host broker for container lifecycle
and progressive trust permission brokering.

Available subcommands:
  start - Start the daemon

Examples:
  misbah daemon start
  misbah daemon start --config /etc/misbah/daemon.yaml`,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon",
	Long: `Start the Misbah daemon.

The daemon reads /etc/misbah/daemon.yaml (or --config path), then:
  1. Connects to containerd for Kata container lifecycle
  2. Listens for permission requests and container commands
  3. Prompts the user for unknown resources (unless non_interactive in config)
  4. Persists decisions and logs all activity

Config loading order: defaults -> /etc/misbah/daemon.yaml -> env vars

Examples:
  misbah daemon start
  misbah daemon start --config /path/to/daemon.yaml`,
	RunE: runDaemonStart,
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)

	daemonStartCmd.Flags().StringVar(&daemonConfigPath, "config", "", "Daemon config file (default: /etc/misbah/daemon.yaml)")

	rootCmd.AddCommand(daemonCmd)
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	// Load daemon config: defaults -> file -> env overrides
	daemonCfg, err := config.LoadDaemonConfig(daemonConfigPath)
	if err != nil {
		return err
	}

	logger.Infof("Daemon config loaded (socket=%s, kata.endpoint=%s, kata.handler=%s)",
		daemonCfg.Daemon.Socket, daemonCfg.Kata.Endpoint, daemonCfg.Kata.Handler)

	// Whitelist
	whitelist := daemon.NewWhitelistStore(daemonCfg.Permissions.Whitelist, logger)
	if err := whitelist.Load(); err != nil {
		logger.Warnf("Failed to load whitelist: %v", err)
	}

	// Audit logger
	audit, err := daemon.NewAuditLogger(daemonCfg.Permissions.AuditLog, logger)
	if err != nil {
		return err
	}
	defer audit.Close()

	// Prompter
	var prompter daemon.Prompter
	if daemonCfg.Daemon.NonInteractive {
		logger.Infof("Running in non-interactive mode (auto-deny)")
		prompter = &daemon.AutoDenyPrompter{}
	} else {
		prompter = daemon.NewTerminalPrompter()
	}

	// CRI backend with Kata annotations from config
	var opts []daemon.ServerOption

	criBackend, criErr := cri.NewBackend(
		daemonCfg.Kata.Endpoint,
		daemonCfg.Kata.Handler,
		logger,
		daemonCfg.Kata.Annotations,
	)
	if criErr != nil {
		logger.Warnf("CRI backend unavailable: %v (Kata containers disabled)", criErr)
	} else {
		lifecycle := runtime.NewLifecycle(logger, recorder, criBackend)
		opts = append(opts, daemon.WithLifecycle(lifecycle))
		defer criBackend.Close()
		logger.Infof("CRI backend initialized (endpoint=%s, handler=%s, annotations=%d)",
			daemonCfg.Kata.Endpoint, daemonCfg.Kata.Handler, len(daemonCfg.Kata.Annotations))
	}

	// Create in-process permission checker, network isolator, and proxy manager
	checker := daemon.NewDirectChecker(whitelist, prompter, audit, logger)
	isolator := daemon.NewNetworkIsolator(daemonCfg.Network, logger)
	proxyMgr := daemon.NewProxyManager(checker, logger, isolator)
	opts = append(opts, daemon.WithProxyManager(proxyMgr))

	server := daemon.NewServer(whitelist, prompter, audit, logger, opts...)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Infof("Received shutdown signal")
		server.Stop()
	}()

	return server.Start(daemonCfg.Daemon.Socket)
}
