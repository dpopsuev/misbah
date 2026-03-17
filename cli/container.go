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
	containerSpecFile string
	containerName     string
	containerCommand  []string
	containerForce    bool
)

// containerCmd represents the container command (parent for subcommands).
var containerCmd = &cobra.Command{
	Use:   "container",
	Short: "Manage containers (namespace-isolated execution environments)",
	Long: `Manage containers using the Container Specification v1.0.

Containers are isolated execution environments using Linux namespaces, bind mounts,
and cgroups. They provide secure, resource-limited containers for running processes.

Available subcommands:
  create   - Create a new container specification file
  validate - Validate a container specification file
  start    - Start a container from a specification
  stop     - Stop a running container
  list     - List all running containers
  inspect  - Inspect container details
  destroy  - Destroy a container and clean up resources

Examples:
  misbah container create --spec container.yaml
  misbah container validate --spec container.yaml
  misbah container start --spec container.yaml
  misbah container list`,
}

// // containerCreateCmd creates a new container specification file.
var containerCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new container specification file",
	Long: `Create a new container specification file with minimal configuration.

The generated file follows the Container Specification v1.0 format and can be
edited to add mounts, resources, and other configuration.

Examples:
  misbah container create --spec mycontainer.yaml --name test-container
  misbah container create --spec mycontainer.yaml --name test-container --command "/bin/bash"`,
	RunE: runContainerCreate,
}

// // containerValidateCmd validates a container specification file.
var containerValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a container specification file",
	Long: `Validate a container specification file against the Container Specification v1.0.

This command checks:
  - YAML syntax
  - Required fields (version, metadata, process, namespaces)
  - Namespace requirements (user and mount required)
  - Mount specifications (type, paths, options)
  - Resource specifications (memory format, CPU/IO limits)

Examples:
  misbah container validate --spec container.yaml`,
	RunE: runContainerValidate,
}

// // containerStartCmd starts a container from a specification.
var containerStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a container from a specification",
	Long: `Start a container by creating namespaces, setting up mounts, applying resource
limits, and executing the specified process.

The container will run until the process exits or is terminated.

Examples:
  misbah container start --spec container.yaml
  misbah container start --spec container.yaml --log-level debug`,
	RunE: runContainerStart,
}

// // containerListCmd lists all running containers.
var containerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all running containers",
	Long: `List all currently running containers by checking lock files.

Shows:
  - Container name
  - Process ID
  - Provider/command
  - Start time
  - Status (running/stale)

Examples:
  misbah container list`,
	RunE: runContainerList,
}

// // containerStopCmd stops a running container.
var containerStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a running container",
	Long: `Stop a running container by terminating its process and releasing the lock.

This command will:
  1. Find the container by name
  2. Send SIGTERM to the process
  3. Wait for graceful shutdown (5 seconds)
  4. Send SIGKILL if still running
  5. Release the lock

Examples:
  misbah container stop --name test-container
  misbah container stop --name test-container --force`,
	RunE: runContainerStop,
}

// // containerInspectCmd inspects container details.
var containerInspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect container details",
	Long: `Inspect detailed information about a container.

Shows:
  - Container specification (if available)
  - Lock status
  - Process status
  - Resource usage

Examples:
  misbah container inspect --name test-container
  misbah container inspect --spec container.yaml`,
	RunE: runContainerInspect,
}

// // containerDestroyCmd destroys a container and cleans up resources.
var containerDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy a container and clean up resources",
	Long: `Destroy a container by stopping it and removing all associated resources.

This command will:
  1. Stop the container (if running)
  2. Remove lock file
  3. Clean up cgroup
  4. Remove temporary files

Examples:
  misbah container destroy --name test-container
  misbah container destroy --name test-container --force`,
	RunE: runContainerDestroy,
}

func init() {
	// Add subcommands to container command
	containerCmd.AddCommand(containerCreateCmd)
	containerCmd.AddCommand(containerValidateCmd)
	containerCmd.AddCommand(containerStartCmd)
	containerCmd.AddCommand(containerListCmd)
	containerCmd.AddCommand(containerStopCmd)
	containerCmd.AddCommand(containerInspectCmd)
	containerCmd.AddCommand(containerDestroyCmd)

	// Flags for create subcommand
	containerCreateCmd.Flags().StringVar(&containerSpecFile, "spec", "", "Container specification file path (required)")
	containerCreateCmd.Flags().StringVar(&containerName, "name", "", "Container name (required)")
	containerCreateCmd.Flags().StringSliceVar(&containerCommand, "command", []string{"/bin/bash"}, "Command to execute in container")
	containerCreateCmd.MarkFlagRequired("spec")
	containerCreateCmd.MarkFlagRequired("name")

	// Flags for validate subcommand
	containerValidateCmd.Flags().StringVar(&containerSpecFile, "spec", "", "Container specification file path (required)")
	containerValidateCmd.MarkFlagRequired("spec")

	// Flags for start subcommand
	containerStartCmd.Flags().StringVar(&containerSpecFile, "spec", "", "Container specification file path (required)")
	containerStartCmd.MarkFlagRequired("spec")

	// Flags for stop subcommand
	containerStopCmd.Flags().StringVar(&containerName, "name", "", "Container name (required)")
	containerStopCmd.Flags().BoolVarP(&containerForce, "force", "f", false, "Force stop (SIGKILL)")
	containerStopCmd.MarkFlagRequired("name")

	// Flags for inspect subcommand
	containerInspectCmd.Flags().StringVar(&containerName, "name", "", "Container name")
	containerInspectCmd.Flags().StringVar(&containerSpecFile, "spec", "", "Container specification file path")

	// Flags for destroy subcommand
	containerDestroyCmd.Flags().StringVar(&containerName, "name", "", "Container name (required)")
	containerDestroyCmd.Flags().BoolVarP(&containerForce, "force", "f", false, "Force destroy")
	containerDestroyCmd.MarkFlagRequired("name")

	// Add container command to root
	rootCmd.AddCommand(containerCmd)
}

func runContainerCreate(cmd *cobra.Command, args []string) error {
	logger.Infof("Creating container specification: %s", containerSpecFile)

	// Create minimal container spec
	spec := &model.ContainerSpec{
		Version: "1.0",
		Metadata: model.ContainerMetadata{
			Name:        containerName,
			Description: "Auto-generated container specification",
			Labels:      make(map[string]string),
		},
		Process: model.ProcessSpec{
			Command: containerCommand,
			Env:     []string{fmt.Sprintf("MISBAH_CONTAINER=%s", containerName)},
			Cwd:     "/container/workspace",
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
				Destination: "/container/workspace",
				Options:     []string{"rw"},
			},
		},
	}

	// Save to file
	if err := spec.SaveContainerSpec(containerSpecFile); err != nil {
		return fmt.Errorf("failed to save container spec: %w", err)
	}

	logger.Infof("Container specification created: %s", containerSpecFile)
	logger.Infof("")
	logger.Infof("Next steps:")
	logger.Infof("  1. Edit the specification: %s", containerSpecFile)
	logger.Infof("  2. Add mounts, resources, and configure namespaces")
	logger.Infof("  3. Validate: misbah container validate --spec %s", containerSpecFile)
	logger.Infof("  4. Start: misbah container start --spec %s", containerSpecFile)

	return nil
}

func runContainerValidate(cmd *cobra.Command, args []string) error {
	logger.Infof("Validating container specification: %s", containerSpecFile)

	// Load container spec
	spec, err := model.LoadContainerSpec(containerSpecFile)
	if err != nil {
		return fmt.Errorf("failed to load container spec: %w", err)
	}

	// Validate
	if err := spec.Validate(); err != nil {
		logger.Errorf("Validation failed:")
		logger.Errorf("%v", err)
		return fmt.Errorf("validation failed")
	}

	logger.Infof("✓ Container specification is valid")
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

func runContainerStart(cmd *cobra.Command, args []string) error {
	logger.Infof("Starting container from specification: %s", containerSpecFile)

	// Load container spec
	spec, err := model.LoadContainerSpec(containerSpecFile)
	if err != nil {
		return fmt.Errorf("failed to load container spec: %w", err)
	}

	logger.Infof("Loaded container specification: %s", spec.Metadata.Name)

	// Create lifecycle manager
	lifecycle := mount.NewLifecycle(logger, recorder)

	// // Mount and execute container
	logger.Infof("Mounting container: %s", spec.Metadata.Name)

	if err := lifecycle.CreateContainer(spec); err != nil {
		return fmt.Errorf("failed to mount container: %w", err)
	}

	logger.Infof("Container %s exited successfully", spec.Metadata.Name)
	return nil
}

func runContainerList(cmd *cobra.Command, args []string) error {
	logger.Infof("Listing running containers")

	// List lock files
	locksDir := "/tmp/misbah/.locks"
	if _, err := os.Stat(locksDir); os.IsNotExist(err) {
		logger.Infof("No running containers")
		return nil
	}

	entries, err := os.ReadDir(locksDir)
	if err != nil {
		return fmt.Errorf("failed to read locks directory: %w", err)
	}

	if len(entries) == 0 {
		logger.Infof("No running containers")
		return nil
	}

	logger.Infof("Running containers:")
	logger.Infof("")

	lockMgr := mount.NewLockManager(logger)

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".lock" {
			containerName := entry.Name()[:len(entry.Name())-5] // Remove .lock

			lock, err := lockMgr.GetLock(containerName)
			if err != nil {
				logger.Warnf("  %s: failed to read lock", containerName)
				continue
			}

			status := "running"
			if lock.IsStale() {
				status = "stale"
			}

			logger.Infof("  • %s (PID %d, %s, %s)",
				containerName, lock.PID, lock.Provider, status)
		}
	}

	return nil
}

func runContainerStop(cmd *cobra.Command, args []string) error {
	logger.Infof("Stopping container: %s (force=%v)", containerName, containerForce)

	// Create lock manager
	lockMgr := mount.NewLockManager(logger)

	// Check if container is running
	lock, err := lockMgr.GetLock(containerName)
	if err != nil {
		return fmt.Errorf("container '%s' is not running", containerName)
	}

	logger.Infof("Found running container: %s (PID %d)", containerName, lock.PID)

	// Stop the container
	if containerForce {
		// Force release (SIGKILL)
		if err := lockMgr.ForceRelease(containerName); err != nil {
			return fmt.Errorf("failed to force stop container: %w", err)
		}
		logger.Infof("Container %s force stopped (SIGKILL)", containerName)
	} else {
		// Graceful release (SIGTERM)
		if err := lockMgr.ReleaseLock(containerName); err != nil {
			return fmt.Errorf("failed to stop container: %w (use --force for SIGKILL)", err)
		}
		logger.Infof("Container %s stopped gracefully", containerName)
	}

	return nil
}

func runContainerInspect(cmd *cobra.Command, args []string) error {
	// If spec file provided, inspect the spec
	if containerSpecFile != "" {
		logger.Infof("Inspecting container specification: %s", containerSpecFile)

		spec, err := model.LoadContainerSpec(containerSpecFile)
		if err != nil {
			return fmt.Errorf("failed to load container spec: %w", err)
		}

		// Display spec details
		logger.Infof("")
		logger.Infof("Container Specification:")
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

	// // If container name provided, inspect running container
	if containerName != "" {
		logger.Infof("")
		logger.Infof("Inspecting running container: %s", containerName)

		lockMgr := mount.NewLockManager(logger)
		lock, err := lockMgr.GetLock(containerName)
		if err != nil {
			logger.Infof("  Status: Not running")
			return nil
		}

		logger.Infof("")
		logger.Infof("Lock Information:")
		logger.Infof("  Container: %s", lock.Workspace)
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

	if containerSpecFile == "" && containerName == "" {
		return fmt.Errorf("must provide either --spec or --name")
	}

	return nil
}

func runContainerDestroy(cmd *cobra.Command, args []string) error {
	logger.Infof("Destroying container: %s (force=%v)", containerName, containerForce)

	lockMgr := mount.NewLockManager(logger)

	// Check if container is running
	lock, err := lockMgr.GetLock(containerName)
	if err == nil {
		// Container is running, stop it first
		logger.Infof("Container is running (PID %d), stopping...", lock.PID)

		if containerForce {
			if err := lockMgr.ForceRelease(containerName); err != nil {
				logger.Warnf("Failed to force stop container: %v", err)
			}
		} else {
			if err := lockMgr.ReleaseLock(containerName); err != nil {
				logger.Warnf("Failed to stop container gracefully: %v", err)
			}
		}
	} else {
		logger.Debugf("Container not running, proceeding with cleanup")
	}

	// Clean up cgroup (if exists)
	cgroupMgr := mount.NewCgroupManager(containerName)
	if err := cgroupMgr.Cleanup(); err != nil {
		logger.Debugf("Cgroup cleanup: %v", err)
	}

	// Remove lock file (if exists)
	lockPath := filepath.Join("/tmp/misbah/.locks", containerName+".lock")
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		logger.Warnf("Failed to remove lock file: %v", err)
	}

	logger.Infof("Container %s destroyed successfully", containerName)
	return nil
}
