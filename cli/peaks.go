package cli

import (
	"fmt"

	"github.com/jabal/jabal/config"
	"github.com/jabal/jabal/model"
	"github.com/spf13/cobra"
)

// peaksCmd represents the peaks command (list workspaces).
var peaksCmd = &cobra.Command{
	Use:   "peaks",
	Short: "List all workspaces",
	Long: `List all available workspaces.

This command shows all workspaces found in ~/.config/jabal/workspaces/

Examples:
  jabal peaks`,
	RunE: runPeaks,
}

func runPeaks(cmd *cobra.Command, args []string) error {
	logger.Debugf("Listing workspaces")

	// List workspaces
	workspaces, err := config.ListWorkspaces()
	if err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	if len(workspaces) == 0 {
		logger.Infof("No workspaces found.")
		logger.Infof("Create one with: jabal create -w myworkspace")
		return nil
	}

	logger.Infof("Available workspaces:")
	logger.Infof("")

	// Load and display each workspace
	for _, workspace := range workspaces {
		manifestPath := config.GetManifestPath(workspace)
		manifest, err := model.LoadManifest(manifestPath)
		if err != nil {
			logger.Warnf("  %s (error loading manifest: %v)", workspace, err)
			continue
		}

		description := manifest.Description
		if description == "" {
			description = "(no description)"
		}

		tags := ""
		if len(manifest.Tags) > 0 {
			tags = fmt.Sprintf(" [%v]", manifest.Tags)
		}

		logger.Infof("  • %s - %s (%d sources)%s", manifest.Name, description, len(manifest.Sources), tags)
	}

	logger.Infof("")
	logger.Infof("Total: %d workspace(s)", len(workspaces))

	return nil
}
