package validate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dpopsuev/misbah/model"
)

// Rule represents a validation rule.
type Rule interface {
	// Validate performs the validation and returns an error if validation fails.
	Validate(manifest *model.Manifest) error
}

// RequiredFieldsRule validates that required fields are present.
type RequiredFieldsRule struct{}

func (r *RequiredFieldsRule) Validate(manifest *model.Manifest) error {
	errors := &ValidationErrors{}

	if manifest.Name == "" {
		errors.Add(NewValidationError("name", "name is required", nil))
	}

	if len(manifest.Sources) == 0 {
		errors.Add(NewValidationError("sources", "at least one source is required", nil))
	}

	return errors.ToError()
}

// WorkspaceNameRule validates the workspace name.
type WorkspaceNameRule struct{}

func (r *WorkspaceNameRule) Validate(manifest *model.Manifest) error {
	if err := model.ValidateWorkspaceName(manifest.Name); err != nil {
		return NewValidationError("name", err.Error(), manifest.Name)
	}
	return nil
}

// SourcePathsExistRule validates that all source paths exist.
type SourcePathsExistRule struct{}

func (r *SourcePathsExistRule) Validate(manifest *model.Manifest) error {
	errors := &ValidationErrors{}

	for i, source := range manifest.Sources {
		// Resolve path
		resolvedPath, err := model.ResolvePath(source.Path)
		if err != nil {
			errors.Add(NewValidationError(
				fmt.Sprintf("sources[%d].path", i),
				fmt.Sprintf("failed to resolve path: %v", err),
				source.Path,
			))
			continue
		}

		// Check if path exists
		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			errors.Add(NewValidationError(
				fmt.Sprintf("sources[%d].path", i),
				"path does not exist",
				resolvedPath,
			))
		} else if err != nil {
			errors.Add(NewValidationError(
				fmt.Sprintf("sources[%d].path", i),
				fmt.Sprintf("failed to stat path: %v", err),
				resolvedPath,
			))
		}
	}

	return errors.ToError()
}

// SourcePathsAbsoluteRule validates that source paths can be resolved to absolute paths.
type SourcePathsAbsoluteRule struct{}

func (r *SourcePathsAbsoluteRule) Validate(manifest *model.Manifest) error {
	errors := &ValidationErrors{}

	for i, source := range manifest.Sources {
		resolvedPath, err := model.ResolvePath(source.Path)
		if err != nil {
			errors.Add(NewValidationError(
				fmt.Sprintf("sources[%d].path", i),
				fmt.Sprintf("failed to resolve path: %v", err),
				source.Path,
			))
			continue
		}

		if !filepath.IsAbs(resolvedPath) {
			errors.Add(NewValidationError(
				fmt.Sprintf("sources[%d].path", i),
				"path must be absolute or use ~ for home directory",
				source.Path,
			))
		}
	}

	return errors.ToError()
}

// SourcePathsNotNestedRule validates that source paths are not nested.
type SourcePathsNotNestedRule struct{}

func (r *SourcePathsNotNestedRule) Validate(manifest *model.Manifest) error {
	sources, err := manifest.ToSources()
	if err != nil {
		return NewValidationError("sources", fmt.Sprintf("failed to convert sources: %v", err), nil)
	}

	if err := model.ValidateSourcesNonNested(sources); err != nil {
		return NewValidationError("sources", err.Error(), nil)
	}

	return nil
}

// MountNamesValidRule validates that mount names are valid.
type MountNamesValidRule struct{}

func (r *MountNamesValidRule) Validate(manifest *model.Manifest) error {
	errors := &ValidationErrors{}

	for i, source := range manifest.Sources {
		if err := model.ValidateMountName(source.Mount); err != nil {
			errors.Add(NewValidationError(
				fmt.Sprintf("sources[%d].mount", i),
				err.Error(),
				source.Mount,
			))
		}
	}

	return errors.ToError()
}

// MountNamesUniqueRule validates that mount names are unique.
type MountNamesUniqueRule struct{}

func (r *MountNamesUniqueRule) Validate(manifest *model.Manifest) error {
	sources, err := manifest.ToSources()
	if err != nil {
		return NewValidationError("sources", fmt.Sprintf("failed to convert sources: %v", err), nil)
	}

	if err := model.ValidateSourcesUniqueMounts(sources); err != nil {
		return NewValidationError("sources", err.Error(), nil)
	}

	return nil
}

// NoWildcardsRule validates that source paths don't contain wildcards.
type NoWildcardsRule struct{}

func (r *NoWildcardsRule) Validate(manifest *model.Manifest) error {
	errors := &ValidationErrors{}

	for i, source := range manifest.Sources {
		if containsWildcard(source.Path) {
			errors.Add(NewValidationError(
				fmt.Sprintf("sources[%d].path", i),
				"path cannot contain wildcards (* or ?)",
				source.Path,
			))
		}
	}

	return errors.ToError()
}

// containsWildcard checks if a path contains wildcard characters.
func containsWildcard(path string) bool {
	for _, char := range path {
		if char == '*' || char == '?' {
			return true
		}
	}
	return false
}
