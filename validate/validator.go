package validate

import (
	"fmt"

	"github.com/jabal/jabal/model"
	"gopkg.in/yaml.v3"
)

// Validator validates workspace manifests.
type Validator struct {
	rules []Rule
}

// NewValidator creates a new validator with default rules.
func NewValidator() *Validator {
	return &Validator{
		rules: []Rule{
			&RequiredFieldsRule{},
			&WorkspaceNameRule{},
			&SourcePathsExistRule{},
			&SourcePathsAbsoluteRule{},
			&SourcePathsNotNestedRule{},
			&MountNamesValidRule{},
			&MountNamesUniqueRule{},
			&NoWildcardsRule{},
		},
	}
}

// AddRule adds a custom validation rule.
func (v *Validator) AddRule(rule Rule) {
	v.rules = append(v.rules, rule)
}

// Validate validates a manifest against all rules.
func (v *Validator) Validate(manifest *model.Manifest) error {
	errors := &ValidationErrors{}

	// First, validate the manifest's basic structure
	if err := manifest.Validate(); err != nil {
		errors.Add(NewValidationError("manifest", err.Error(), nil))
	}

	// Run all validation rules
	for _, rule := range v.rules {
		if err := rule.Validate(manifest); err != nil {
			// If the error is a ValidationErrors, extract individual errors
			if valErrs, ok := err.(*ValidationErrors); ok {
				errors.Errors = append(errors.Errors, valErrs.Errors...)
			} else if valErr, ok := err.(*ValidationError); ok {
				errors.Add(valErr)
			} else {
				// Generic error
				errors.Add(NewValidationError("unknown", err.Error(), nil))
			}
		}
	}

	return errors.ToError()
}

// ValidateFile validates a manifest file.
func (v *Validator) ValidateFile(path string) error {
	manifest, err := model.LoadManifest(path)
	if err != nil {
		return fmt.Errorf("failed to load manifest: %w", err)
	}

	return v.Validate(manifest)
}

// ValidateYAML validates YAML syntax and structure.
func ValidateYAML(data []byte) error {
	var temp interface{}
	if err := yaml.Unmarshal(data, &temp); err != nil {
		return fmt.Errorf("invalid YAML syntax: %w", err)
	}
	return nil
}

// ValidateManifest is a convenience function for validating a manifest with default rules.
func ValidateManifest(manifest *model.Manifest) error {
	validator := NewValidator()
	return validator.Validate(manifest)
}

// ValidateManifestFile is a convenience function for validating a manifest file with default rules.
func ValidateManifestFile(path string) error {
	validator := NewValidator()
	return validator.ValidateFile(path)
}
