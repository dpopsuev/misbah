package model

import "errors"

// Domain errors for the misbah system.
var (
	// ErrWorkspaceNotFound is returned when a workspace cannot be found.
	ErrWorkspaceNotFound = errors.New("workspace not found")

	// ErrWorkspaceLocked is returned when a workspace is already locked by another process.
	ErrWorkspaceLocked = errors.New("workspace is locked")

	// ErrInvalidWorkspaceName is returned when a workspace name is invalid.
	ErrInvalidWorkspaceName = errors.New("invalid workspace name")

	// ErrSourceNotFound is returned when a source path does not exist.
	ErrSourceNotFound = errors.New("source path not found")

	// ErrSourceNested is returned when sources have overlapping paths.
	ErrSourceNested = errors.New("source paths cannot be nested")

	// ErrInvalidMountName is returned when a mount name is invalid.
	ErrInvalidMountName = errors.New("invalid mount name")

	// ErrDuplicateMountName is returned when mount names are not unique.
	ErrDuplicateMountName = errors.New("duplicate mount name")

	// ErrInvalidManifest is returned when a manifest is malformed.
	ErrInvalidManifest = errors.New("invalid manifest")

	// ErrProviderNotFound is returned when a provider is not in the registry.
	ErrProviderNotFound = errors.New("provider not found")

	// ErrNamespaceCreationFailed is returned when namespace creation fails.
	ErrNamespaceCreationFailed = errors.New("namespace creation failed")

	// ErrMountFailed is returned when a bind mount operation fails.
	ErrMountFailed = errors.New("mount operation failed")

	// ErrLockAcquisitionFailed is returned when lock acquisition fails.
	ErrLockAcquisitionFailed = errors.New("lock acquisition failed")

	// ErrInvalidPath is returned when a path is invalid (absolute, contains .., etc).
	ErrInvalidPath = errors.New("invalid path")

	// ErrPathTraversal is returned when a path attempts to escape its boundary.
	ErrPathTraversal = errors.New("path traversal detected")
)
