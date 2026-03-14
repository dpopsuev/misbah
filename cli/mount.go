package cli

import (
	"fmt"
	"os/exec"

	"github.com/jabal/jabal/config"
	"github.com/jabal/jabal/model"
	"github.com/jabal/jabal/mount"
	"github.com/jabal/jabal/provider"
	"github.com/spf13/cobra"
)

var (
	mountWorkspace string
	mountProvider  string
)

// mountCmd represents the mount command.
var mountCmd = &cobra.Command{
	Use:   "mount",
	Short: "Mount a workspace and launch a provider",
	Long: `Mount a workspace by creating a namespace with bind mounts and launching
a provider (Claude Code, Aider, etc.) in the unified workspace.

The mount command:
  1. Validates the workspace manifest
  2. Acquires a lock to prevent concurrent access
  3. Creates a Linux namespace with bind mounts
  4. Generates provider configuration
  5. Launches the provider in the workspace
  6. Cleans up when the provider exits

Examples:
  jabal mount -w myworkspace -a claude
  jabal mount -w project -a aider
  jabal mount -w demo -a claude --log-level debug`,
	RunE: runMount,
}

func init() {
	mountCmd.Flags().StringVarP(&mountWorkspace, "workspace", "w", "", "Workspace name (required)")
	mountCmd.Flags().StringVarP(&mountProvider, "agent", "a", "", "Provider name (claude, aider, cursor)")

	mountCmd.MarkFlagRequired("workspace")
}

func runMount(cmd *cobra.Command, args []string) error {
	logger.Infof("Mounting workspace: %s", mountWorkspace)

	// Use default provider if not specified
	if mountProvider == "" {
		mountProvider = cfg.DefaultProvider
		logger.Infof("Using default provider: %s", mountProvider)
	}

	// Load workspace
	manifestPath := config.GetManifestPath(mountWorkspace)
	if !config.PathExists(manifestPath) {
		return fmt.Errorf("workspace '%s' does not exist (manifest not found at %s)", mountWorkspace, manifestPath)
	}

	workspace, err := model.LoadWorkspace(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to load workspace: %w", err)
	}

	logger.Debugf("Loaded workspace: %s with %d sources", workspace.Name, len(workspace.Sources))

	// Get provider
	prov, err := provider.Get(mountProvider)
	if err != nil {
		return fmt.Errorf("provider '%s' not found: %w", mountProvider, err)
	}

	// Check if provider binary is available
	providerBinary := prov.BinaryName()
	if _, err := exec.LookPath(providerBinary); err != nil {
		return fmt.Errorf("provider binary '%s' not found in PATH: %w", providerBinary, err)
	}

	// Generate provider configuration
	providerConfigDir := workspace.GetProviderConfigDir(mountProvider)
	logger.Debugf("Generating provider config at: %s", providerConfigDir)

	providerConfig, err := workspace.GetProvider(mountProvider)
	if err != nil {
		// No provider config in manifest, use empty config
		logger.Debugf("No provider config in manifest, using default")
		providerConfig = make(map[string]interface{})
	}

	if err := prov.GenerateConfig(providerConfigDir, providerConfig); err != nil {
		return fmt.Errorf("failed to generate provider config: %w", err)
	}

	// Create lifecycle manager
	lifecycle := mount.NewLifecycle(logger, recorder)

	// Mount workspace and launch provider
	logger.Infof("Launching %s in workspace %s", providerBinary, workspace.Name)

	if err := lifecycle.Mount(workspace, mountProvider, providerBinary); err != nil {
		return fmt.Errorf("failed to mount workspace: %w", err)
	}

	logger.Infof("Workspace %s unmounted successfully", workspace.Name)
	return nil
}
