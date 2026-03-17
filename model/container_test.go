package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadContainerSpec(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal container spec",
			yaml: `version: "1.0"
metadata:
  name: test-container
process:
  command: ["/bin/bash"]
  cwd: /container/workspace
namespaces:
  user: true
  mount: true
  pid: true
mounts:
  - type: bind
    source: /tmp
    destination: /container/workspace
`,
			wantErr: false,
		},
		{
			name: "valid container spec with resources",
			yaml: `version: "1.0"
metadata:
  name: test-container
  description: Test container
  labels:
    env: test
process:
  command: ["/usr/bin/claude"]
  env: ["MISBAH_CONTAINER=test"]
  cwd: /container/workspace
namespaces:
  user: true
  mount: true
  pid: true
  network: false
mounts:
  - type: bind
    source: /home/user/repo
    destination: /container/workspace/repo
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
			tmpFile := filepath.Join(t.TempDir(), "container.yaml")
			err := os.WriteFile(tmpFile, []byte(tt.yaml), 0644)
			require.NoError(t, err)

			// Load spec
			spec, err := LoadContainerSpec(tmpFile)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, spec)
			}
		})
	}
}

func TestContainerSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    *ContainerSpec
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal spec",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Mounts: []MountSpec{
					{
						Type:        "bind",
						Source:      "/tmp",
						Destination: "/container/workspace",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid version",
			spec: &ContainerSpec{
				Version: "2.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Mounts: []MountSpec{},
			},
			wantErr: true,
			errMsg:  "unsupported container spec version",
		},
		{
			name: "missing container name",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
			},
			wantErr: true,
			errMsg:  "container name is required",
		},
		{
			name: "missing user namespace",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
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
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Mounts: []MountSpec{
					{
						Type:        "invalid",
						Destination: "/container/workspace",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid mount type",
		},
		{
			name: "valid namespace runtime",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Runtime: "namespace",
			},
			wantErr: false,
		},
		{
			name: "valid kata runtime with image",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Runtime: "kata",
				Image:   "docker.io/library/alpine:latest",
			},
			wantErr: false,
		},
		{
			name: "kata runtime without image",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Runtime: "kata",
			},
			wantErr: true,
			errMsg:  "image is required",
		},
		{
			name: "unsupported runtime",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Runtime: "docker",
			},
			wantErr: true,
			errMsg:  "unsupported runtime",
		},
		{
			name: "valid network config",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Runtime: "kata",
				Image:   "alpine:latest",
				Network: &NetworkConfig{
					Mode:       "none",
					DNSServers: []string{"8.8.8.8"},
					Hostname:   "agent-01",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid network mode",
			spec: &ContainerSpec{
				Version: "1.0",
				Metadata: ContainerMetadata{
					Name: "test-container",
				},
				Process: ProcessSpec{
					Command: []string{"/bin/bash"},
					Cwd:     "/container/workspace",
				},
				Namespaces: NamespaceSpec{
					User:  true,
					Mount: true,
				},
				Runtime: "kata",
				Image:   "alpine:latest",
				Network: &NetworkConfig{
					Mode: "bridge",
				},
			},
			wantErr: true,
			errMsg:  "invalid network mode",
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
				Cwd:     "/container/workspace",
			},
			wantErr: false,
		},
		{
			name: "missing command",
			spec: ProcessSpec{
				Command: []string{},
				Cwd:     "/container/workspace",
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
				Destination: "/container/workspace/repo",
				Options:     []string{"ro"},
			},
			wantErr: false,
		},
		{
			name: "valid tmpfs mount",
			spec: MountSpec{
				Type:        "tmpfs",
				Destination: "/container/workspace/tmp",
			},
			wantErr: false,
		},
		{
			name: "invalid mount type",
			spec: MountSpec{
				Type:        "overlay",
				Destination: "/container/workspace",
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
				Destination: "/container/workspace",
			},
			wantErr: true,
			errMsg:  "bind mount requires source",
		},
		{
			name: "conflicting ro/rw options",
			spec: MountSpec{
				Type:        "bind",
				Source:      "/tmp",
				Destination: "/container/workspace",
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
				Destination: "/container/workspace",
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

func TestSaveContainerSpec(t *testing.T) {
	spec := &ContainerSpec{
		Version: "1.0",
		Metadata: ContainerMetadata{
			Name:        "test-container",
			Description: "Test container for unit tests",
		},
		Process: ProcessSpec{
			Command: []string{"/bin/bash"},
			Cwd:     "/container/workspace",
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
				Destination: "/container/workspace",
				Options:     []string{"rw"},
			},
		},
	}

	tmpFile := filepath.Join(t.TempDir(), "container.yaml")
	err := spec.SaveContainerSpec(tmpFile)
	require.NoError(t, err)

	// Verify file exists
	assert.FileExists(t, tmpFile)

	// Load it back
	loaded, err := LoadContainerSpec(tmpFile)
	require.NoError(t, err)

	assert.Equal(t, spec.Version, loaded.Version)
	assert.Equal(t, spec.Metadata.Name, loaded.Metadata.Name)
	assert.Equal(t, spec.Process.Command, loaded.Process.Command)
	assert.Equal(t, len(spec.Mounts), len(loaded.Mounts))
}

func TestContainerSpecString(t *testing.T) {
	spec := &ContainerSpec{
		Version: "1.0",
		Metadata: ContainerMetadata{
			Name: "test-container",
		},
		Mounts: []MountSpec{
			{Type: "bind", Source: "/tmp", Destination: "/container/workspace"},
		},
	}

	str := spec.String()
	assert.Contains(t, str, "test-container")
	assert.Contains(t, str, "1.0")
	assert.Contains(t, str, "mounts=1")
}
