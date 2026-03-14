package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolvePath resolves a path by expanding ~ and environment variables.
// This is a convenience wrapper around model.ResolvePath for config usage.
func ResolvePath(path string) (string, error) {
	// Expand tilde
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	// Clean the path
	path = filepath.Clean(path)

	// Make absolute if not already
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to make path absolute: %w", err)
		}
		path = absPath
	}

	return path, nil
}

// EnsureDir ensures a directory exists, creating it if necessary.
func EnsureDir(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	return nil
}

// EnsureConfigDir ensures the misbah config directory exists.
func EnsureConfigDir() error {
	return EnsureDir(GetConfigDir())
}

// EnsureWorkspacesDir ensures the workspaces directory exists.
func EnsureWorkspacesDir() error {
	return EnsureDir(GetWorkspacesDir())
}

// EnsureWorkspaceDir ensures a specific workspace directory exists.
func EnsureWorkspaceDir(workspace string) error {
	return EnsureDir(GetWorkspaceDir(workspace))
}

// EnsureTempDir ensures the temporary directory exists.
func EnsureTempDir() error {
	return EnsureDir(GetTempDir())
}

// EnsureLocksDir ensures the locks directory exists.
func EnsureLocksDir() error {
	return EnsureDir(GetLocksDir())
}

// PathExists checks if a path exists.
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// IsDirectory checks if a path is a directory.
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ListWorkspaces returns a list of workspace names.
func ListWorkspaces() ([]string, error) {
	workspacesDir := GetWorkspacesDir()

	// Check if workspaces directory exists
	if !PathExists(workspacesDir) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(workspacesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read workspaces directory: %w", err)
	}

	var workspaces []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if manifest exists
			manifestPath := GetManifestPath(entry.Name())
			if PathExists(manifestPath) {
				workspaces = append(workspaces, entry.Name())
			}
		}
	}

	return workspaces, nil
}

// WorkspaceExists checks if a workspace exists.
func WorkspaceExists(workspace string) bool {
	manifestPath := GetManifestPath(workspace)
	return PathExists(manifestPath)
}
