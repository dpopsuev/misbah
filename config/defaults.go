package config

import (
	"os"
	"path/filepath"
)

const (
	// DefaultConfigDir is the default configuration directory.
	DefaultConfigDir = ".config/jabal"

	// DefaultWorkspacesDir is the default workspaces directory.
	DefaultWorkspacesDir = "workspaces"

	// DefaultTempDir is the default temporary directory for mounts.
	DefaultTempDir = "/tmp/jabal"

	// DefaultLocksDir is the default directory for lock files.
	DefaultLocksDir = ".locks"

	// ManifestFileName is the standard manifest file name.
	ManifestFileName = "manifest.yaml"
)

// GetConfigDir returns the configuration directory path.
func GetConfigDir() string {
	if configDir := os.Getenv("JABAL_CONFIG_DIR"); configDir != "" {
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
	if tempDir := os.Getenv("JABAL_TEMP_DIR"); tempDir != "" {
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
	return filepath.Join(GetLocksDir(), workspace+".lock")
}

// GetMountPath returns the mount path for a workspace.
func GetMountPath(workspace string) string {
	return filepath.Join(GetTempDir(), workspace)
}
