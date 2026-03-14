package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLock(t *testing.T) {
	lock, err := NewLock("test-workspace", "claude", 12345)
	require.NoError(t, err)
	assert.Equal(t, "test-workspace", lock.Workspace)
	assert.Equal(t, "claude", lock.Provider)
	assert.Equal(t, 12345, lock.PID)
	assert.NotEmpty(t, lock.User)
	assert.False(t, lock.StartedAt.IsZero())
}

func TestLockMarshalUnmarshal(t *testing.T) {
	original, err := NewLock("test-workspace", "claude", 12345)
	require.NoError(t, err)

	// Marshal
	data, err := original.Marshal()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Unmarshal
	restored, err := UnmarshalLock(data)
	require.NoError(t, err)
	assert.Equal(t, original.Workspace, restored.Workspace)
	assert.Equal(t, original.Provider, restored.Provider)
	assert.Equal(t, original.PID, restored.PID)
	assert.Equal(t, original.User, restored.User)
}

func TestLockIsStale(t *testing.T) {
	// Lock with current process PID (should not be stale)
	currentPID := os.Getpid()
	lock, err := NewLock("test", "claude", currentPID)
	require.NoError(t, err)
	assert.False(t, lock.IsStale(), "Current process lock should not be stale")

	// Lock with non-existent PID (should be stale)
	// Use a very high PID that's unlikely to exist
	staleLock, err := NewLock("test", "claude", 999999)
	require.NoError(t, err)
	assert.True(t, staleLock.IsStale(), "Non-existent PID lock should be stale")
}

func TestLockString(t *testing.T) {
	lock, err := NewLock("test-workspace", "claude", 12345)
	require.NoError(t, err)

	str := lock.String()
	assert.Contains(t, str, "test-workspace")
	assert.Contains(t, str, "claude")
	assert.Contains(t, str, "12345")
}
