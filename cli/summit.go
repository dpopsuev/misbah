package cli

import (
	"fmt"

	"github.com/dpopsuev/misbah/mount"
	"github.com/spf13/cobra"
)

var (
	summitWorkspace string
)

// summitCmd represents the summit command (show workspace status).
var summitCmd = &cobra.Command{
	Use:   "summit",
	Short: "Show workspace status",
	Long: `Show the current status of a workspace.

This command displays whether a workspace is currently mounted, along with
information about the provider, process ID, and mount time.

Examples:
  misbah summit -w myworkspace`,
	RunE: runSummit,
}

func init() {
	summitCmd.Flags().StringVarP(&summitWorkspace, "workspace", "w", "", "Workspace name (required)")

	summitCmd.MarkFlagRequired("workspace")
}

func runSummit(cmd *cobra.Command, args []string) error {
	logger.Debugf("Getting status for workspace: %s", summitWorkspace)

	// Create lifecycle manager
	lifecycle := mount.NewLifecycle(logger, recorder)

	// Get status
	status, err := lifecycle.GetStatus(summitWorkspace)
	if err != nil {
		return fmt.Errorf("failed to get workspace status: %w", err)
	}

	// Display status
	logger.Infof("Workspace: %s", summitWorkspace)
	logger.Infof("")

	if !status.Mounted {
		logger.Infof("Status: Not mounted")
	} else {
		logger.Infof("Status: Mounted")
		logger.Infof("Provider: %s", status.Provider)
		logger.Infof("PID: %d", status.PID)
		logger.Infof("User: %s", status.User)
		logger.Infof("Started: %v", status.StartedAt)

		if status.Stale {
			logger.Warnf("")
			logger.Warnf("⚠ Warning: Lock is stale (process no longer running)")
			logger.Warnf("Run 'misbah unmount -w %s --force' to clean up", summitWorkspace)
		}
	}

	return nil
}
