package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadJailSpec(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal jail spec",
			yaml: `version: "1.0"
metadata:
  name: test-jail
process:
  command: ["/bin/bash"]
  cwd: /jail/workspace
namespaces:
  user: true
  mount: true
  pid: true
mounts:
  - type: bind
    source: /tmp
    destination: /jail/workspace
`,
			wantErr: false,
		},
		{
			name: "valid jail spec with resources",
			yaml: `version: "1.0"
metadata:
  name: test-jail
  description: Test jail
  labels:
    env: test
process:
  command: ["/usr/bin/claude"]
  env: ["MISBAH_JAIL=test"]
  cwd: /jail/workspace
namespaces:
  user: true
  mount: true
  pid: true
  network: false
mounts:
  - type: bind
    source: /home/user/repo
    destination: /jail/workspace/repo
    options: [ro]
resources:
  memory: 2GB
  cpu_shares: 1024
`,
			wantErr: false,
		},
		{
			name:    "invalid YAML",
			yaml:    `invalid: [yaml syntax`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write to temp file
			tmpFile := filepath.Join(t.TempDir(), "jail.yaml")
			err := os.WriteFile(tmpFile, []byte(tt.yaml), 0644)
			require.NoError(t, err)

			// Load spec
			spec, err := LoadJailSpec(tmpFile)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, spec)
			}
		})
	}
}

func TestJailSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    *JailSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal spec",
			spec: &JailSpec{
				Version: "1.0",
				Metadata: JailMetadata{
					Name: "test-jail",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/jail/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Mounts: []MountSpec{
					{
						Type:        "bind",
						Source:      "/tmp",
						Destination: "/jail/workspace",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid version",
			spec: &JailSpec{
				Version: "2.0",
				Metadata: JailMetadata{
					Name: "test-jail",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/jail/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Mounts: []MountSpec{},
			},
			wantErr: true,
			errMsg:  "unsupported jail spec version",
		},
		{
			name: "missing jail name",
			spec: &JailSpec{
				Version: "1.0",
				Metadata: JailMetadata{
					Name: "",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/jail/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
			},
			wantErr: true,
			errMsg:  "jail name is required",
		},
		{
			name: "missing user namespace",
			spec: &JailSpec{
				Version: "1.0",
				Metadata: JailMetadata{
					Name: "test-jail",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/jail/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  false,
					Mount: true,
				},
			},
			wantErr: true,
			errMsg:  "user namespace must be enabled",
		},
		{
			name: "invalid mount type",
			spec: &JailSpec{
				Version: "1.0",
				Metadata: JailMetadata{
					Name: "test-jail",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/jail/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Mounts: []MountSpec{
					{
						Type:        "invalid",
						Destination: "/jail/workspace",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid mount type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProcessSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    ProcessSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid process spec",
			spec: ProcessSpec{
				Command: []string{"/bin/bash"},
				Cwd:     "/jail/workspace",
			},
			wantErr: false,
		},
		{
			name: "missing command",
			spec: ProcessSpec{
				Command: []string{},
				Cwd:     "/jail/workspace",
			},
			wantErr: true,
			errMsg:  "process command is required",
		},
		{
			name: "missing cwd",
			spec: ProcessSpec{
				Command: []string{"/bin/bash"},
				Cwd:     "",
			},
			wantErr: true,
			errMsg:  "process working directory is required",
		},
		{
			name: "relative cwd",
			spec: ProcessSpec{
				Command: []string{"/bin/bash"},
				Cwd:     "relative/path",
			},
			wantErr: true,
			errMsg:  "process working directory must be absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMountSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    MountSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid bind mount",
			spec: MountSpec{
				Type:        "bind",
				Source:      "/home/user/repo",
				Destination: "/jail/workspace/repo",
				Options:     []string{"ro"},
			},
			wantErr: false,
		},
		{
			name: "valid tmpfs mount",
			spec: MountSpec{
				Type:        "tmpfs",
				Destination: "/jail/workspace/tmp",
			},
			wantErr: false,
		},
		{
			name: "invalid mount type",
			spec: MountSpec{
				Type:        "overlay",
				Destination: "/jail/workspace",
			},
			wantErr: true,
			errMsg:  "invalid mount type",
		},
		{
			name: "missing destination",
			spec: MountSpec{
				Type:        "bind",
				Source:      "/tmp",
				Destination: "",
			},
			wantErr: true,
			errMsg:  "mount destination is required",
		},
		{
			name: "relative destination",
			spec: MountSpec{
				Type:        "bind",
				Source:      "/tmp",
				Destination: "relative/path",
			},
			wantErr: true,
			errMsg:  "mount destination must be absolute",
		},
		{
			name: "bind mount missing source",
			spec: MountSpec{
				Type:        "bind",
				Destination: "/jail/workspace",
			},
			wantErr: true,
			errMsg:  "bind mount requires source",
		},
		{
			name: "conflicting ro/rw options",
			spec: MountSpec{
				Type:        "bind",
				Source:      "/tmp",
				Destination: "/jail/workspace",
				Options:     []string{"ro", "rw"},
			},
			wantErr: true,
			errMsg:  "conflicting mount options",
		},
		{
			name: "invalid option",
			spec: MountSpec{
				Type:        "bind",
				Source:      "/tmp",
				Destination: "/jail/workspace",
				Options:     []string{"invalid-option"},
			},
			wantErr: true,
			errMsg:  "invalid mount option",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestResourceSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    ResourceSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid resources",
			spec: ResourceSpec{
				Memory:    "2GB",
				CPUShares: 1024,
				IOWeight:  100,
			},
			wantErr: false,
		},
		{
			name: "valid memory formats",
			spec: ResourceSpec{
				Memory: "512MB",
			},
			wantErr: false,
		},
		{
			name: "invalid memory format",
			spec: ResourceSpec{
				Memory: "2G",
			},
			wantErr: true,
			errMsg:  "invalid memory specification",
		},
		{
			name: "cpu shares too low",
			spec: ResourceSpec{
				CPUShares: 0,
			},
			wantErr: false, // 0 means not specified
		},
		{
			name: "cpu shares too high",
			spec: ResourceSpec{
				CPUShares: 20000,
			},
			wantErr: true,
			errMsg:  "cpu_shares must be between 1 and 10000",
		},
		{
			name: "io weight out of range",
			spec: ResourceSpec{
				IOWeight: 15000,
			},
			wantErr: true,
			errMsg:  "io_weight must be between 1 and 10000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSaveJailSpec(t *testing.T) {
	spec := &JailSpec{
		Version: "1.0",
		Metadata: JailMetadata{
			Name:        "test-jail",
			Description: "Test jail for unit tests",
		},
		Process: ProcessSpec{
			Command: []string{"/bin/bash"},
			Cwd:     "/jail/workspace",
		},
		Namespaces: NamespaceSpec{
			User:  true,
			Mount: true,
			PID:   true,
		},
		Mounts: []MountSpec{
			{
				Type:        "bind",
				Source:      "/tmp",
				Destination: "/jail/workspace",
				Options:     []string{"rw"},
			},
		},
	}

	tmpFile := filepath.Join(t.TempDir(), "jail.yaml")
	err := spec.SaveJailSpec(tmpFile)
	require.NoError(t, err)

	// Verify file exists
	assert.FileExists(t, tmpFile)

	// Load it back
	loaded, err := LoadJailSpec(tmpFile)
	require.NoError(t, err)

	assert.Equal(t, spec.Version, loaded.Version)
	assert.Equal(t, spec.Metadata.Name, loaded.Metadata.Name)
	assert.Equal(t, spec.Process.Command, loaded.Process.Command)
	assert.Equal(t, len(spec.Mounts), len(loaded.Mounts))
}

func TestJailSpecString(t *testing.T) {
	spec := &JailSpec{
		Version: "1.0",
		Metadata: JailMetadata{
			Name: "test-jail",
		},
		Mounts: []MountSpec{
			{Type: "bind", Source: "/tmp", Destination: "/jail/workspace"},
		},
	}

	str := spec.String()
	assert.Contains(t, str, "test-jail")
	assert.Contains(t, str, "1.0")
	assert.Contains(t, str, "mounts=1")
}
