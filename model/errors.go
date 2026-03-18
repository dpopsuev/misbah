package model

import "errors"

// Domain errors for the misbah system.
var (
	// ErrWorkspaceLocked is returned when a workspace is already locked by another process.
	ErrWorkspaceLocked = errors.New("workspace is locked")

	// ErrInvalidContainerName is returned when a container name is invalid.
	ErrInvalidContainerName = errors.New("invalid container name")

	// ErrNamespaceCreationFailed is returned when namespace creation fails.
	ErrNamespaceCreationFailed = errors.New("namespace creation failed")

	// ErrMountFailed is returned when a bind mount operation fails.
	ErrMountFailed = errors.New("mount operation failed")

)
