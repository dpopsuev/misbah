package cli

import (
	"fmt"

	"github.com/dpopsuev/misbah/mount"
	"github.com/spf13/cobra"
)

var (
	unmountWorkspace string
	unmountForce     bool
)

// unmountCmd represents the unmount command.
var unmountCmd = &cobra.Command{
	Use:   "unmount",
	Short: "Unmount a workspace",
	Long: `Unmount a workspace by releasing its lock and cleaning up mount points.

If the workspace is currently mounted, this command will attempt to release
the lock gracefully. Use --force to forcefully terminate the provider process.

Examples:
  misbah unmount -w myworkspace
  misbah unmount -w myworkspace --force`,
	RunE: runUnmount,
}

func init() {
	unmountCmd.Flags().StringVarP(&unmountWorkspace, "workspace", "w", "", "Workspace name (required)")
	unmountCmd.Flags().BoolVarP(&unmountForce, "force", "f", false, "Force unmount by terminating the provider process")

	unmountCmd.MarkFlagRequired("workspace")
}

func runUnmount(cmd *cobra.Command, args []string) error {
	logger.Infof("Unmounting workspace: %s (force=%v)", unmountWorkspace, unmountForce)

	// Create lifecycle manager
	lifecycle := mount.NewLifecycle(logger, recorder)

	// Unmount workspace
	if err := lifecycle.Unmount(unmountWorkspace, unmountForce); err != nil {
		return fmt.Errorf("failed to unmount workspace: %w", err)
	}

	logger.Infof("Workspace %s unmounted successfully", unmountWorkspace)
	return nil
}
