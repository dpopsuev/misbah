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
	jailForce    bool
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

// jailStopCmd stops a running jail.
var jailStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running jail",
	Long: `Stop a running jail by terminating its process and releasing the lock.

This command will:
  1. Find the jail by name
  2. Send SIGTERM to the process
  3. Wait for graceful shutdown (5 seconds)
  4. Send SIGKILL if still running
  5. Release the lock

Examples:
  misbah jail stop --name test-jail
  misbah jail stop --name test-jail --force`,
	RunE: runJailStop,
}

// jailInspectCmd inspects jail details.
var jailInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect jail details",
	Long: `Inspect detailed information about a jail.

Shows:
  - Jail specification (if available)
  - Lock status
  - Process status
  - Resource usage

Examples:
  misbah jail inspect --name test-jail
  misbah jail inspect --spec jail.yaml`,
	RunE: runJailInspect,
}

// jailDestroyCmd destroys a jail and cleans up resources.
var jailDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy a jail and clean up resources",
	Long: `Destroy a jail by stopping it and removing all associated resources.

This command will:
  1. Stop the jail (if running)
  2. Remove lock file
  3. Clean up cgroup
  4. Remove temporary files

Examples:
  misbah jail destroy --name test-jail
  misbah jail destroy --name test-jail --force`,
	RunE: runJailDestroy,
}

func init() {
	// Add subcommands to jail command
	jailCmd.AddCommand(jailCreateCmd)
	jailCmd.AddCommand(jailValidateCmd)
	jailCmd.AddCommand(jailStartCmd)
	jailCmd.AddCommand(jailListCmd)
	jailCmd.AddCommand(jailStopCmd)
	jailCmd.AddCommand(jailInspectCmd)
	jailCmd.AddCommand(jailDestroyCmd)

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

	// Flags for stop subcommand
	jailStopCmd.Flags().StringVar(&jailName, "name", "", "Jail name (required)")
	jailStopCmd.Flags().BoolVarP(&jailForce, "force", "f", false, "Force stop (SIGKILL)")
	jailStopCmd.MarkFlagRequired("name")

	// Flags for inspect subcommand
	jailInspectCmd.Flags().StringVar(&jailName, "name", "", "Jail name")
	jailInspectCmd.Flags().StringVar(&jailSpecFile, "spec", "", "Jail specification file path")

	// Flags for destroy subcommand
	jailDestroyCmd.Flags().StringVar(&jailName, "name", "", "Jail name (required)")
	jailDestroyCmd.Flags().BoolVarP(&jailForce, "force", "f", false, "Force destroy")
	jailDestroyCmd.MarkFlagRequired("name")

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

func runJailStop(cmd *cobra.Command, args []string) error {
	logger.Infof("Stopping jail: %s (force=%v)", jailName, jailForce)

	// Create lock manager
	lockMgr := mount.NewLockManager(logger)

	// Check if jail is running
	lock, err := lockMgr.GetLock(jailName)
	if err != nil {
		return fmt.Errorf("jail '%s' is not running", jailName)
	}

	logger.Infof("Found running jail: %s (PID %d)", jailName, lock.PID)

	// Stop the jail
	if jailForce {
		// Force release (SIGKILL)
		if err := lockMgr.ForceRelease(jailName); err != nil {
			return fmt.Errorf("failed to force stop jail: %w", err)
		}
		logger.Infof("Jail %s force stopped (SIGKILL)", jailName)
	} else {
		// Graceful release (SIGTERM)
		if err := lockMgr.ReleaseLock(jailName); err != nil {
			return fmt.Errorf("failed to stop jail: %w (use --force for SIGKILL)", err)
		}
		logger.Infof("Jail %s stopped gracefully", jailName)
	}

	return nil
}

func runJailInspect(cmd *cobra.Command, args []string) error {
	// If spec file provided, inspect the spec
	if jailSpecFile != "" {
		logger.Infof("Inspecting jail specification: %s", jailSpecFile)

		spec, err := model.LoadJailSpec(jailSpecFile)
		if err != nil {
			return fmt.Errorf("failed to load jail spec: %w", err)
		}

		// Display spec details
		logger.Infof("")
		logger.Infof("Jail Specification:")
		logger.Infof("  Version: %s", spec.Version)
		logger.Infof("  Name: %s", spec.Metadata.Name)
		logger.Infof("  Description: %s", spec.Metadata.Description)
		logger.Infof("")
		logger.Infof("Process:")
		logger.Infof("  Command: %v", spec.Process.Command)
		logger.Infof("  Working Directory: %s", spec.Process.Cwd)
		if len(spec.Process.Env) > 0 {
			logger.Infof("  Environment: %v", spec.Process.Env)
		}
		logger.Infof("")
		logger.Infof("Namespaces:")
		logger.Infof("  User: %v", spec.Namespaces.User)
		logger.Infof("  Mount: %v", spec.Namespaces.Mount)
		logger.Infof("  PID: %v", spec.Namespaces.PID)
		logger.Infof("  Network: %v", spec.Namespaces.Network)
		logger.Infof("  IPC: %v", spec.Namespaces.IPC)
		logger.Infof("  UTS: %v", spec.Namespaces.UTS)
		logger.Infof("")
		logger.Infof("Mounts (%d):", len(spec.Mounts))
		for i, m := range spec.Mounts {
			logger.Infof("  [%d] Type: %s", i+1, m.Type)
			if m.Source != "" {
				logger.Infof("      Source: %s", m.Source)
			}
			logger.Infof("      Destination: %s", m.Destination)
			if len(m.Options) > 0 {
				logger.Infof("      Options: %v", m.Options)
			}
		}

		if spec.Resources != nil {
			logger.Infof("")
			logger.Infof("Resources:")
			if spec.Resources.Memory != "" {
				logger.Infof("  Memory: %s", spec.Resources.Memory)
			}
			if spec.Resources.CPUShares > 0 {
				logger.Infof("  CPU Shares: %d", spec.Resources.CPUShares)
			}
			if spec.Resources.IOWeight > 0 {
				logger.Infof("  IO Weight: %d", spec.Resources.IOWeight)
			}
		}

		if len(spec.Metadata.Labels) > 0 {
			logger.Infof("")
			logger.Infof("Labels:")
			for k, v := range spec.Metadata.Labels {
				logger.Infof("  %s: %s", k, v)
			}
		}
	}

	// If jail name provided, inspect running jail
	if jailName != "" {
		logger.Infof("")
		logger.Infof("Inspecting running jail: %s", jailName)

		lockMgr := mount.NewLockManager(logger)
		lock, err := lockMgr.GetLock(jailName)
		if err != nil {
			logger.Infof("  Status: Not running")
			return nil
		}

		logger.Infof("")
		logger.Infof("Lock Information:")
		logger.Infof("  Jail: %s", lock.Workspace)
		logger.Infof("  Provider: %s", lock.Provider)
		logger.Infof("  PID: %d", lock.PID)
		logger.Infof("  User: %s", lock.User)
		logger.Infof("  Started: %v", lock.StartedAt)

		if lock.IsStale() {
			logger.Warnf("  Status: STALE (process no longer running)")
		} else {
			logger.Infof("  Status: Running")
		}
	}

	if jailSpecFile == "" && jailName == "" {
		return fmt.Errorf("must provide either --spec or --name")
	}

	return nil
}

func runJailDestroy(cmd *cobra.Command, args []string) error {
	logger.Infof("Destroying jail: %s (force=%v)", jailName, jailForce)

	lockMgr := mount.NewLockManager(logger)

	// Check if jail is running
	lock, err := lockMgr.GetLock(jailName)
	if err == nil {
		// Jail is running, stop it first
		logger.Infof("Jail is running (PID %d), stopping...", lock.PID)

		if jailForce {
			if err := lockMgr.ForceRelease(jailName); err != nil {
				logger.Warnf("Failed to force stop jail: %v", err)
			}
		} else {
			if err := lockMgr.ReleaseLock(jailName); err != nil {
				logger.Warnf("Failed to stop jail gracefully: %v", err)
			}
		}
	} else {
		logger.Debugf("Jail not running, proceeding with cleanup")
	}

	// Clean up cgroup (if exists)
	cgroupMgr := mount.NewCgroupManager(jailName)
	if err := cgroupMgr.Cleanup(); err != nil {
		logger.Debugf("Cgroup cleanup: %v", err)
	}

	// Remove lock file (if exists)
	lockPath := filepath.Join("/tmp/misbah/.locks", jailName+".lock")
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		logger.Warnf("Failed to remove lock file: %v", err)
	}

	logger.Infof("Jail %s destroyed successfully", jailName)
	return nil
}
