package cli

import (
	"fmt"

	"github.com/jabal/jabal/config"
	"github.com/jabal/jabal/validate"
	"github.com/spf13/cobra"
)

var (
	validateWorkspace string
)

// validateCmd represents the validate command.
var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a workspace manifest",
	Long: `Validate a workspace manifest against all validation rules.

This command checks:
  - YAML syntax
  - Required fields
  - Source paths existence
  - Mount name validity and uniqueness
  - Workspace name validity
  - No nested source paths
  - No wildcard characters in paths

Examples:
  jabal validate -w myworkspace`,
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().StringVarP(&validateWorkspace, "workspace", "w", "", "Workspace name (required)")

	validateCmd.MarkFlagRequired("workspace")
}

func runValidate(cmd *cobra.Command, args []string) error {
	logger.Infof("Validating workspace: %s", validateWorkspace)

	// Check if workspace exists
	if !config.WorkspaceExists(validateWorkspace) {
		return fmt.Errorf("workspace '%s' does not exist", validateWorkspace)
	}

	// Get manifest path
	manifestPath := config.GetManifestPath(validateWorkspace)

	// Validate manifest
	logger.Debugf("Validating manifest at: %s", manifestPath)

	if err := validate.ValidateManifestFile(manifestPath); err != nil {
		logger.Errorf("Validation failed:")
		logger.Errorf("%v", err)
		return fmt.Errorf("validation failed")
	}

	logger.Infof("✓ Workspace '%s' is valid", validateWorkspace)

	return nil
}
