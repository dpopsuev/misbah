package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source represents a source directory to be mounted in the workspace.
type Source struct {
	// Path is the absolute path to the source directory on the host.
	Path string

	// Mount is the name used to mount the source in the workspace.
	Mount string
}

// SourceSpec represents a source specification in the manifest YAML.
type SourceSpec struct {
	Path  string `yaml:"path"`
	Mount string `yaml:"mount"`
}

// Validate validates the source specification.
func (s *Source) Validate() error {
	// Validate mount name
	if err := ValidateMountName(s.Mount); err != nil {
		return err
	}

	// Validate path exists
	if _, err := os.Stat(s.Path); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrSourceNotFound, s.Path)
	}

	// Ensure path is absolute
	if !filepath.IsAbs(s.Path) {
		return fmt.Errorf("%w: %s (must be absolute)", ErrInvalidPath, s.Path)
	}

	return nil
}

// ValidateMountName validates a mount name.
// Mount names must be alphanumeric with optional dashes and underscores.
func ValidateMountName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: mount name cannot be empty", ErrInvalidMountName)
	}

	// Check for valid characters (alphanumeric, dash, underscore)
	for _, char := range name {
		if !isAlphanumericOrDashOrUnderscore(char) {
			return fmt.Errorf("%w: %s (only alphanumeric, dash, and underscore allowed)", ErrInvalidMountName, name)
		}
	}

	// Cannot start with dash or underscore
	if name[0] == '-' || name[0] == '_' {
		return fmt.Errorf("%w: %s (cannot start with dash or underscore)", ErrInvalidMountName, name)
	}

	return nil
}

// isAlphanumericOrDashOrUnderscore checks if a character is valid in a mount name.
func isAlphanumericOrDashOrUnderscore(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '-' ||
		char == '_'
}

// ValidateSourcesNonNested validates that no source paths are nested within each other.
func ValidateSourcesNonNested(sources []Source) error {
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			// Check if path i is a prefix of path j or vice versa
			pathI := filepath.Clean(sources[i].Path)
			pathJ := filepath.Clean(sources[j].Path)

			if isNestedPath(pathI, pathJ) {
				return fmt.Errorf("%w: %s and %s", ErrSourceNested, pathI, pathJ)
			}
		}
	}
	return nil
}

// isNestedPath checks if one path is nested within another.
func isNestedPath(path1, path2 string) bool {
	// Normalize paths
	path1 = filepath.Clean(path1) + string(filepath.Separator)
	path2 = filepath.Clean(path2) + string(filepath.Separator)

	// Check if one is a prefix of the other
	return strings.HasPrefix(path1, path2) || strings.HasPrefix(path2, path1)
}

// ValidateSourcesUniqueMounts validates that all mount names are unique.
func ValidateSourcesUniqueMounts(sources []Source) error {
	seen := make(map[string]bool)

	for _, source := range sources {
		if seen[source.Mount] {
			return fmt.Errorf("%w: %s", ErrDuplicateMountName, source.Mount)
		}
		seen[source.Mount] = true
	}

	return nil
}

// ResolvePath resolves a path by expanding ~ and environment variables.
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

	// Make absolute
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to make path absolute: %w", err)
		}
		path = absPath
	}

	return path, nil
}
