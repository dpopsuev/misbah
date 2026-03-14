package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/jabal/jabal/config"
	"github.com/spf13/cobra"
)

var (
	editWorkspace string
	editEditor    string
)

// editCmd represents the edit command.
var editCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit a workspace manifest",
	Long: `Edit a workspace manifest using your default editor.

The editor is determined by the EDITOR environment variable, or falls back to vi.

Examples:
  jabal edit -w myworkspace
  jabal edit -w myworkspace --editor nano`,
	RunE: runEdit,
}

func init() {
	editCmd.Flags().StringVarP(&editWorkspace, "workspace", "w", "", "Workspace name (required)")
	editCmd.Flags().StringVar(&editEditor, "editor", "", "Editor to use (default: $EDITOR or vi)")

	editCmd.MarkFlagRequired("workspace")
}

func runEdit(cmd *cobra.Command, args []string) error {
	logger.Infof("Editing workspace: %s", editWorkspace)

	// Check if workspace exists
	if !config.WorkspaceExists(editWorkspace) {
		return fmt.Errorf("workspace '%s' does not exist", editWorkspace)
	}

	// Get manifest path
	manifestPath := config.GetManifestPath(editWorkspace)

	// Determine editor
	editor := editEditor
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	// Launch editor
	logger.Debugf("Launching editor: %s %s", editor, manifestPath)

	editorCmd := exec.Command(editor, manifestPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("editor exited with error: %w", err)
	}

	logger.Infof("Manifest saved. Run 'jabal validate -w %s' to validate changes.", editWorkspace)

	return nil
}
