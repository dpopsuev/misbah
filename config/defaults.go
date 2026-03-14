package config

import (
	"os"
	"path/filepath"
)

const (
	// DefaultConfigDir is the default configuration directory.
	DefaultConfigDir = ".config/misbah"

	// DefaultWorkspacesDir is the default workspaces directory.
	DefaultWorkspacesDir = "workspaces"

	// DefaultTempDir is the default temporary directory for mounts.
	DefaultTempDir = "/tmp/misbah"

	// DefaultLocksDir is the default directory for lock files (relative to TempDir).
	DefaultLocksDir = ".locks"

	// ManifestFileName is the standard manifest file name.
	ManifestFileName = "manifest.yaml"

	// LockFileExt is the lock file extension.
	LockFileExt = ".lock"

	// DefaultCgroupRoot is the root cgroup v2 directory.
	DefaultCgroupRoot = "/sys/fs/cgroup"

	// CgroupSubdir is the subdirectory for Misbah cgroups.
	CgroupSubdir = "misbah"

	// JailSpecVersion is the current jail specification version.
	JailSpecVersion = "1.0"

	// DefaultJailWorkspace is the default working directory inside jails.
	DefaultJailWorkspace = "/jail/workspace"
)

// Environment variable names.
const (
	// EnvConfigDir overrides the config directory.
	EnvConfigDir = "MISBAH_CONFIG_DIR"

	// EnvTempDir overrides the temp directory.
	EnvTempDir = "MISBAH_TEMP_DIR"

	// EnvJail is set inside the jail to identify it.
	EnvJail = "MISBAH_JAIL"

	// EnvWorkspace is set for legacy workspace compatibility.
	EnvWorkspace = "MISBAH_WORKSPACE"

	// EnvProvider is set to identify the provider/command.
	EnvProvider = "MISBAH_PROVIDER"

	// EnvLockPID is set to the lock PID.
	EnvLockPID = "MISBAH_LOCK_PID"
)

// GetConfigDir returns the configuration directory path.
func GetConfigDir() string {
	if configDir := os.Getenv(EnvConfigDir); configDir != "" {
		return configDir
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", DefaultConfigDir)
	}
	return filepath.Join(homeDir, DefaultConfigDir)
}

// GetWorkspacesDir returns the workspaces directory path.
func GetWorkspacesDir() string {
	return filepath.Join(GetConfigDir(), DefaultWorkspacesDir)
}

// GetWorkspaceDir returns the directory for a specific workspace.
func GetWorkspaceDir(workspace string) string {
	return filepath.Join(GetWorkspacesDir(), workspace)
}

// GetManifestPath returns the path to a workspace's manifest file.
func GetManifestPath(workspace string) string {
	return filepath.Join(GetWorkspaceDir(workspace), ManifestFileName)
}

// GetTempDir returns the temporary directory for mounts.
func GetTempDir() string {
	if tempDir := os.Getenv(EnvTempDir); tempDir != "" {
		return tempDir
	}
	return DefaultTempDir
}

// GetLocksDir returns the locks directory path.
func GetLocksDir() string {
	return filepath.Join(GetTempDir(), DefaultLocksDir)
}

// GetLockPath returns the path to a workspace's lock file.
func GetLockPath(workspace string) string {
	return filepath.Join(GetLocksDir(), workspace+LockFileExt)
}

// GetMountPath returns the mount path for a workspace.
func GetMountPath(workspace string) string {
	return filepath.Join(GetTempDir(), workspace)
}
