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

