package cli

import (
	"fmt"

	"github.com/jabal/jabal/config"
	"github.com/jabal/jabal/model"
	"github.com/spf13/cobra"
)

var (
	createWorkspace   string
	createDescription string
)

// createCmd represents the create command.
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new workspace",
	Long: `Create a new workspace by generating a manifest file.

This command creates a minimal workspace manifest that you can edit to add sources.

Examples:
  jabal create -w myworkspace
  jabal create -w myworkspace --description "My project workspace"`,
	RunE: runCreate,
}

func init() {
	createCmd.Flags().StringVarP(&createWorkspace, "workspace", "w", "", "Workspace name (required)")
	createCmd.Flags().StringVarP(&createDescription, "description", "d", "", "Workspace description")

	createCmd.MarkFlagRequired("workspace")
}

func runCreate(cmd *cobra.Command, args []string) error {
	logger.Infof("Creating workspace: %s", createWorkspace)

	// Validate workspace name
	if err := model.ValidateWorkspaceName(createWorkspace); err != nil {
		return fmt.Errorf("invalid workspace name: %w", err)
	}

	// Check if workspace already exists
	if config.WorkspaceExists(createWorkspace) {
		return fmt.Errorf("workspace '%s' already exists", createWorkspace)
	}

	// Create workspace directory
	if err := config.EnsureWorkspaceDir(createWorkspace); err != nil {
		return fmt.Errorf("failed to create workspace directory: %w", err)
	}

	// Create minimal manifest
	manifest := &model.Manifest{
		Name:        createWorkspace,
		Description: createDescription,
		Sources:     []model.SourceSpec{},
		Providers:   make(map[string]interface{}),
		Tags:        []string{},
	}

	// Save manifest
	manifestPath := config.GetManifestPath(createWorkspace)
	if err := manifest.SaveManifest(manifestPath); err != nil {
		return fmt.Errorf("failed to save manifest: %w", err)
	}

	logger.Infof("Workspace '%s' created successfully", createWorkspace)
	logger.Infof("Manifest saved to: %s", manifestPath)
	logger.Infof("")
	logger.Infof("Next steps:")
	logger.Infof("  1. Edit the manifest: jabal edit -w %s", createWorkspace)
	logger.Infof("  2. Add source directories")
	logger.Infof("  3. Mount the workspace: jabal mount -w %s -a claude", createWorkspace)

	return nil
}
