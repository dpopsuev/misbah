package cli

import (
	"fmt"
	"os"

	"github.com/jabal/jabal/config"
	"github.com/jabal/jabal/metrics"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	logLevel  string
	configDir string
	verbose   bool

	// Global instances
	logger   *metrics.Logger
	recorder *metrics.MetricsRecorder
	cfg      *config.GlobalConfig
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "jabal",
	Short: "Jabal - Workspace manager for CLI AI agents",
	Long: `Jabal creates unified workspaces from multiple source repositories using
Linux user namespaces and bind mounts, enabling CLI AI agents to work
seamlessly across project boundaries.

Examples:
  jabal create -w myworkspace
  jabal mount -w myworkspace -a claude
  jabal peaks
  jabal summit`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize logger
		level := metrics.LogLevel(logLevel)
		if verbose {
			level = metrics.LogLevelDebug
		}
		logger = metrics.NewConsoleLogger(level)
		recorder = metrics.NewMetricsRecorder()

		// Set global instances
		metrics.SetDefaultLogger(logger)
		metrics.SetDefaultRecorder(recorder)

		// Load global config
		var err error
		cfg, err = config.LoadGlobalConfig()
		if err != nil {
			logger.Warnf("Failed to load global config, using defaults: %v", err)
			cfg = config.DefaultGlobalConfig()
		}

		// Override config dir if specified
		if configDir != "" {
			os.Setenv("JABAL_CONFIG_DIR", configDir)
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&configDir, "config-dir", "", "Configuration directory (default: ~/.config/jabal)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output (same as --log-level=debug)")

	// Add commands
	rootCmd.AddCommand(mountCmd)
	rootCmd.AddCommand(unmountCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(peaksCmd)
	rootCmd.AddCommand(summitCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(versionCmd)
}

// ExitCode constants for standardized exit codes.
const (
	ExitSuccess            = 0
	ExitGeneralError       = 1
	ExitValidationError    = 2
	ExitLockError          = 3
	ExitMountError         = 4
	ExitProviderError      = 5
	ExitNamespaceError     = 6
	ExitSignalError        = 7
	ExitCleanupError       = 8
	ExitConfigurationError = 9
	ExitUnknownError       = 10
)

// HandleError handles errors and exits with appropriate exit code.
func HandleError(err error) {
	if err == nil {
		os.Exit(ExitSuccess)
		return
	}

	// Determine exit code based on error type
	exitCode := ExitGeneralError

	// Log error
	logger.Errorf("Error: %v", err)

	os.Exit(exitCode)
}

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of jabal",
	Long:  "Print the version number and build information for jabal.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("jabal version 0.1.0")
		fmt.Println("A workspace manager for CLI AI agents")
	},
}
