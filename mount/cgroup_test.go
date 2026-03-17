package mount

import (
	"testing"

	"github.com/dpopsuev/misbah/model"
	"github.com/stretchr/testify/assert"
)

func TestNewCgroupManager(t *testing.T) {
	mgr := NewCgroupManager("test-container")
	assert.NotNil(t, mgr)
	assert.Equal(t, "test-container", mgr.containerName)
	assert.Equal(t, "/sys/fs/cgroup", mgr.cgroupRoot)
}

func TestCgroupManager_getCgroupPath(t *testing.T) {
	mgr := NewCgroupManager("test-container")
	path := mgr.getCgroupPath()
	assert.Equal(t, "/sys/fs/cgroup/misbah/test-container", path)
}

func TestParseMemorySpec(t *testing.T) {
	tests := []struct {
		name     string
		spec     string
		want     int64
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid KB",
			spec: "512KB",
			want: 512 * 1024,
		},
		{
			name: "valid MB",
			spec: "512MB",
			want: 512 * 1024 * 1024,
		},
		{
			name: "valid GB",
			spec: "2GB",
			want: 2 * 1024 * 1024 * 1024,
		},
		{
			name: "lowercase suffix",
			spec: "1gb",
			want: 1 * 1024 * 1024 * 1024,
		},
		{
			name:    "invalid suffix",
			spec:    "2TB",
			wantErr: true,
			errMsg:  "invalid memory spec suffix",
		},
		{
			name:    "invalid number",
			spec:    "abcMB",
			wantErr: true,
			errMsg:  "invalid memory spec number",
		},
		{
			name:    "too short",
			spec:    "2",
			wantErr: true,
			errMsg:  "invalid memory spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMemorySpec(tt.spec)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestCgroupManager_isCgroupV2Available(t *testing.T) {
	mgr := NewCgroupManager("test-container")

	// This test depends on the system configuration
	// Just verify it doesn't panic and returns a boolean
	available := mgr.isCgroupV2Available()
	assert.IsType(t, false, available)
}

func TestCgroupManager_Setup_NilResources(t *testing.T) {
	mgr := NewCgroupManager("test-container")

	// Should succeed with nil resources (no-op)
	err := mgr.Setup(nil)
	assert.NoError(t, err)
}

func TestCgroupManager_Setup_ValidResources(t *testing.T) {
	mgr := NewCgroupManager("test-container")

	resources := &model.ResourceSpec{
		Memory:    "512MB",
		CPUShares: 1024,
		IOWeight:  100,
	}

	// This test will fail if cgroup v2 is not available on the system
	// We test the logic but expect potential errors on systems without cgroup v2
	err := mgr.Setup(resources)

	// On systems with cgroup v2, this should work (if we have permissions)
	// On systems without cgroup v2, we expect an error
	if err != nil {
		// Check it's the expected error (not a programming bug)
		assert.Contains(t, err.Error(), "cgroup")
	}
}

func TestCgroupManager_Cleanup(t *testing.T) {
	mgr := NewCgroupManager("test-container")

	// Cleanup should not fail even if cgroup doesn't exist
	err := mgr.Cleanup()
	assert.NoError(t, err)
}
