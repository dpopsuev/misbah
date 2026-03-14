package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dpopsuev/misbah/model"
	"github.com/dpopsuev/misbah/mount"
	"github.com/spf13/cobra"
)

var (
	jailSpecFile string
	jailName     string
	jailCommand  []string
)

// jailCmd represents the jail command (parent for subcommands).
var jailCmd = &cobra.Command{
	Use:   "jail",
	Short: "Manage jails (namespace-isolated execution environments)",
	Long: `Manage jails using the Jail Specification v1.0.

Jails are isolated execution environments using Linux namespaces, bind mounts,
and cgroups. They provide secure, resource-limited containers for running processes.

Available subcommands:
  create   - Create a new jail specification file
  validate - Validate a jail specification file
  start    - Start a jail from a specification
  stop     - Stop a running jail
  list     - List all running jails
  inspect  - Inspect jail details
  destroy  - Destroy a jail and clean up resources

Examples:
  misbah jail create --spec jail.yaml
  misbah jail validate --spec jail.yaml
  misbah jail start --spec jail.yaml
  misbah jail list`,
}

// jailCreateCmd creates a new jail specification file.
var jailCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new jail specification file",
	Long: `Create a new jail specification file with minimal configuration.

The generated file follows the Jail Specification v1.0 format and can be
edited to add mounts, resources, and other configuration.

Examples:
  misbah jail create --spec myjail.yaml --name test-jail
  misbah jail create --spec myjail.yaml --name test-jail --command "/bin/bash"`,
	RunE: runJailCreate,
}

// jailValidateCmd validates a jail specification file.
var jailValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a jail specification file",
	Long: `Validate a jail specification file against the Jail Specification v1.0.

This command checks:
  - YAML syntax
  - Required fields (version, metadata, process, namespaces)
  - Namespace requirements (user and mount required)
  - Mount specifications (type, paths, options)
  - Resource specifications (memory format, CPU/IO limits)

Examples:
  misbah jail validate --spec jail.yaml`,
	RunE: runJailValidate,
}

// jailStartCmd starts a jail from a specification.
var jailStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a jail from a specification",
	Long: `Start a jail by creating namespaces, setting up mounts, applying resource
limits, and executing the specified process.

The jail will run until the process exits or is terminated.

Examples:
  misbah jail start --spec jail.yaml
  misbah jail start --spec jail.yaml --log-level debug`,
	RunE: runJailStart,
}

// jailListCmd lists all running jails.
var jailListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all running jails",
	Long: `List all currently running jails by checking lock files.

Shows:
  - Jail name
  - Process ID
  - Provider/command
  - Start time
  - Status (running/stale)

Examples:
  misbah jail list`,
	RunE: runJailList,
}

func init() {
	// Add subcommands to jail command
	jailCmd.AddCommand(jailCreateCmd)
	jailCmd.AddCommand(jailValidateCmd)
	jailCmd.AddCommand(jailStartCmd)
	jailCmd.AddCommand(jailListCmd)

	// Flags for create subcommand
	jailCreateCmd.Flags().StringVar(&jailSpecFile, "spec", "", "Jail specification file path (required)")
	jailCreateCmd.Flags().StringVar(&jailName, "name", "", "Jail name (required)")
	jailCreateCmd.Flags().StringSliceVar(&jailCommand, "command", []string{"/bin/bash"}, "Command to execute in jail")
	jailCreateCmd.MarkFlagRequired("spec")
	jailCreateCmd.MarkFlagRequired("name")

	// Flags for validate subcommand
	jailValidateCmd.Flags().StringVar(&jailSpecFile, "spec", "", "Jail specification file path (required)")
	jailValidateCmd.MarkFlagRequired("spec")

	// Flags for start subcommand
	jailStartCmd.Flags().StringVar(&jailSpecFile, "spec", "", "Jail specification file path (required)")
	jailStartCmd.MarkFlagRequired("spec")

	// Add jail command to root
	rootCmd.AddCommand(jailCmd)
}

func runJailCreate(cmd *cobra.Command, args []string) error {
	logger.Infof("Creating jail specification: %s", jailSpecFile)

	// Create minimal jail spec
	spec := &model.JailSpec{
		Version: "1.0",
		Metadata: model.JailMetadata{
			Name:        jailName,
			Description: "Auto-generated jail specification",
			Labels:      make(map[string]string),
		},
		Process: model.ProcessSpec{
			Command: jailCommand,
			Env:     []string{fmt.Sprintf("MISBAH_JAIL=%s", jailName)},
			Cwd:     "/jail/workspace",
		},
		Namespaces: model.NamespaceSpec{
			User:  true,
			Mount: true,
			PID:   true,
		},
		Mounts: []model.MountSpec{
			{
				Type:        "bind",
				Source:      "/tmp",
				Destination: "/jail/workspace",
				Options:     []string{"rw"},
			},
		},
	}

	// Save to file
	if err := spec.SaveJailSpec(jailSpecFile); err != nil {
		return fmt.Errorf("failed to save jail spec: %w", err)
	}

	logger.Infof("Jail specification created: %s", jailSpecFile)
	logger.Infof("")
	logger.Infof("Next steps:")
	logger.Infof("  1. Edit the specification: %s", jailSpecFile)
	logger.Infof("  2. Add mounts, resources, and configure namespaces")
	logger.Infof("  3. Validate: misbah jail validate --spec %s", jailSpecFile)
	logger.Infof("  4. Start: misbah jail start --spec %s", jailSpecFile)

	return nil
}

func runJailValidate(cmd *cobra.Command, args []string) error {
	logger.Infof("Validating jail specification: %s", jailSpecFile)

	// Load jail spec
	spec, err := model.LoadJailSpec(jailSpecFile)
	if err != nil {
		return fmt.Errorf("failed to load jail spec: %w", err)
	}

	// Validate
	if err := spec.Validate(); err != nil {
		logger.Errorf("Validation failed:")
		logger.Errorf("%v", err)
		return fmt.Errorf("validation failed")
	}

	logger.Infof("✓ Jail specification is valid")
	logger.Infof("  Name: %s", spec.Metadata.Name)
	logger.Infof("  Version: %s", spec.Version)
	logger.Infof("  Mounts: %d", len(spec.Mounts))
	logger.Infof("  Namespaces: user=%v, mount=%v, pid=%v",
		spec.Namespaces.User, spec.Namespaces.Mount, spec.Namespaces.PID)

	if spec.Resources != nil {
		logger.Infof("  Resources configured: memory=%s, cpu_shares=%d",
			spec.Resources.Memory, spec.Resources.CPUShares)
	}

	return nil
}

func runJailStart(cmd *cobra.Command, args []string) error {
	logger.Infof("Starting jail from specification: %s", jailSpecFile)

	// Load jail spec
	spec, err := model.LoadJailSpec(jailSpecFile)
	if err != nil {
		return fmt.Errorf("failed to load jail spec: %w", err)
	}

	logger.Infof("Loaded jail specification: %s", spec.Metadata.Name)

	// Create lifecycle manager
	lifecycle := mount.NewLifecycle(logger, recorder)

	// Mount and execute jail
	logger.Infof("Mounting jail: %s", spec.Metadata.Name)

	if err := lifecycle.MountJail(spec); err != nil {
		return fmt.Errorf("failed to mount jail: %w", err)
	}

	logger.Infof("Jail %s exited successfully", spec.Metadata.Name)
	return nil
}

func runJailList(cmd *cobra.Command, args []string) error {
	logger.Infof("Listing running jails")

	// List lock files
	locksDir := "/tmp/misbah/.locks"
	if _, err := os.Stat(locksDir); os.IsNotExist(err) {
		logger.Infof("No running jails")
		return nil
	}

	entries, err := os.ReadDir(locksDir)
	if err != nil {
		return fmt.Errorf("failed to read locks directory: %w", err)
	}

	if len(entries) == 0 {
		logger.Infof("No running jails")
		return nil
	}

	logger.Infof("Running jails:")
	logger.Infof("")

	lockMgr := mount.NewLockManager(logger)

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".lock" {
			jailName := entry.Name()[:len(entry.Name())-5] // Remove .lock

			lock, err := lockMgr.GetLock(jailName)
			if err != nil {
				logger.Warnf("  %s: failed to read lock", jailName)
				continue
			}

			status := "running"
			if lock.IsStale() {
				status = "stale"
			}

			logger.Infof("  • %s (PID %d, %s, %s)",
				jailName, lock.PID, lock.Provider, status)
		}
	}

	return nil
}
