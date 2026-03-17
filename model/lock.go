package model

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"
)

// Lock represents a workspace lock file.
type Lock struct {
	// Workspace is the name of the locked workspace.
	Workspace string `json:"workspace"`

	// Provider is the provider that locked the workspace.
	Provider string `json:"provider"`

	// PID is the process ID of the locking process.
	PID int `json:"pid"`

	// StartedAt is when the lock was acquired.
	StartedAt time.Time `json:"started_at"`

	// User is the username that acquired the lock.
	User string `json:"user"`

	// ContainerID is the CRI container ID (set when using kata runtime).
	ContainerID string `json:"container_id,omitempty"`

	// SandboxID is the CRI pod sandbox ID (set when using kata runtime).
	SandboxID string `json:"sandbox_id,omitempty"`

	// Runtime is the container runtime backend ("namespace" or "kata").
	Runtime string `json:"runtime,omitempty"`
}

// NewLock creates a new lock for the given workspace and provider.
func NewLock(workspace, provider string, pid int) (*Lock, error) {
	username := os.Getenv("USER")
	if username == "" {
		username = "unknown"
	}

	return &Lock{
		Workspace: workspace,
		Provider:  provider,
		PID:       pid,
		StartedAt: time.Now(),
		User:      username,
	}, nil
}

// Marshal serializes the lock to JSON.
func (l *Lock) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lock: %w", err)
	}
	return data, nil
}

// UnmarshalLock deserializes a lock from JSON.
func UnmarshalLock(data []byte) (*Lock, error) {
	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("failed to unmarshal lock: %w", err)
	}
	return &lock, nil
}

// IsStale checks if the lock is stale by verifying if the process is still running.
func (l *Lock) IsStale() bool {
	// On Unix, use kill with signal 0 to check if process exists
	// Signal 0 doesn't actually send a signal but checks if the process exists
	err := syscall.Kill(l.PID, syscall.Signal(0))
	if err != nil {
		// Process doesn't exist (ESRCH) or we don't have permission
		// In either case, consider the lock stale
		return true
	}

	return false
}

// String returns a human-readable representation of the lock.
func (l *Lock) String() string {
	return fmt.Sprintf("Lock{workspace=%s, provider=%s, pid=%d, user=%s, started=%s}",
		l.Workspace, l.Provider, l.PID, l.User, l.StartedAt.Format(time.RFC3339))
}
