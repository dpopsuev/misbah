package validate

import "fmt"

// ValidationError represents a validation error with context.
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

// Error implements the error interface.
func (v *ValidationError) Error() string {
	if v.Value != nil {
		return fmt.Sprintf("validation error in field '%s': %s (value: %v)", v.Field, v.Message, v.Value)
	}
	return fmt.Sprintf("validation error in field '%s': %s", v.Field, v.Message)
}

// NewValidationError creates a new validation error.
func NewValidationError(field, message string, value interface{}) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: message,
		Value:   value,
	}
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors struct {
	Errors []*ValidationError
}

// Error implements the error interface.
func (v *ValidationErrors) Error() string {
	if len(v.Errors) == 0 {
		return "no validation errors"
	}
	if len(v.Errors) == 1 {
		return v.Errors[0].Error()
	}
	return fmt.Sprintf("%d validation errors: %s (and %d more)", len(v.Errors), v.Errors[0].Error(), len(v.Errors)-1)
}

// Add adds a validation error to the collection.
func (v *ValidationErrors) Add(err *ValidationError) {
	v.Errors = append(v.Errors, err)
}

// HasErrors returns true if there are any validation errors.
func (v *ValidationErrors) HasErrors() bool {
	return len(v.Errors) > 0
}

// ToError returns the ValidationErrors as an error if there are any errors, otherwise nil.
func (v *ValidationErrors) ToError() error {
	if v.HasErrors() {
		return v
	}
	return nil
}
