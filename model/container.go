package model

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ContainerSpec represents a container specification conforming to MSB-SPC-2026-001.
type ContainerSpec struct {
	Version    string            `yaml:"version"`
	Metadata   ContainerMetadata      `yaml:"metadata"`
	Process    ProcessSpec       `yaml:"process"`
	Namespaces NamespaceSpec     `yaml:"namespaces"`
	Mounts     []MountSpec       `yaml:"mounts"`
	Resources  *ResourceSpec     `yaml:"resources,omitempty"`
}

// ContainerMetadata contains container metadata.
type ContainerMetadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
}

// ProcessSpec specifies the process to execute in the container.
type ProcessSpec struct {
	Command []string `yaml:"command"`
	Env     []string `yaml:"env,omitempty"`
	Cwd     string   `yaml:"cwd"`
}

// NamespaceSpec specifies which namespaces to create.
type NamespaceSpec struct {
	User    bool `yaml:"user"`
	Mount   bool `yaml:"mount"`
	PID     bool `yaml:"pid"`
	Network bool `yaml:"network,omitempty"`
	IPC     bool `yaml:"ipc,omitempty"`
	UTS     bool `yaml:"uts,omitempty"`
}

// MountSpec specifies a mount operation.
type MountSpec struct {
	Type        string   `yaml:"type"`        // bind, tmpfs, proc
	Source      string   `yaml:"source,omitempty"`
	Destination string   `yaml:"destination"`
	Options     []string `yaml:"options,omitempty"` // ro, rw, nosuid, nodev, etc.
}

// ResourceSpec specifies resource limits.
type ResourceSpec struct {
	Memory    string `yaml:"memory,omitempty"`     // "2GB", "512MB"
	CPUShares int    `yaml:"cpu_shares,omitempty"` // 1024 = 100%
	IOWeight  int    `yaml:"io_weight,omitempty"`  // 1-10000
}

// LoadContainerSpec loads a container specification from a YAML file.
func LoadContainerSpec(path string) (*ContainerSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read container spec: %w", err)
	}

	var spec ContainerSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse container spec: %w", err)
	}

	return &spec, nil
}

// SaveContainerSpec saves a container specification to a YAML file.
func (j *ContainerSpec) SaveContainerSpec(path string) error {
	data, err := yaml.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to marshal container spec: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write container spec: %w", err)
	}

	return nil
}

// Validate validates the container specification.
func (j *ContainerSpec) Validate() error {
	// Validate version
	if j.Version != "1.0" {
		return fmt.Errorf("unsupported container spec version: %s (expected 1.0)", j.Version)
	}

	// Validate metadata
	if err := j.Metadata.Validate(); err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}

	// Validate process
	if err := j.Process.Validate(); err != nil {
		return fmt.Errorf("invalid process: %w", err)
	}

	// Validate namespaces
	if err := j.Namespaces.Validate(); err != nil {
		return fmt.Errorf("invalid namespaces: %w", err)
	}

	// Validate mounts
	for i, mount := range j.Mounts {
		if err := mount.Validate(); err != nil {
			return fmt.Errorf("invalid mount %d: %w", i, err)
		}
	}

	// Validate resources (if specified)
	if j.Resources != nil {
		if err := j.Resources.Validate(); err != nil {
			return fmt.Errorf("invalid resources: %w", err)
		}
	}

	return nil
}

// Validate validates container metadata.
func (m *ContainerMetadata) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("container name is required")
	}

	// Use existing workspace name validation
	if err := ValidateWorkspaceName(m.Name); err != nil {
		return fmt.Errorf("invalid container name: %w", err)
	}

	return nil
}

// Validate validates process specification.
func (p *ProcessSpec) Validate() error {
	if len(p.Command) == 0 {
		return fmt.Errorf("process command is required")
	}

	if p.Cwd == "" {
		return fmt.Errorf("process working directory is required")
	}

	// Validate cwd is absolute
	if !filepath.IsAbs(p.Cwd) {
		return fmt.Errorf("process working directory must be absolute: %s", p.Cwd)
	}

	return nil
}

// Validate validates namespace specification.
func (n *NamespaceSpec) Validate() error {
	// At minimum, user and mount namespaces must be enabled
	if !n.User {
		return fmt.Errorf("user namespace must be enabled")
	}

	if !n.Mount {
		return fmt.Errorf("mount namespace must be enabled")
	}

	return nil
}

// Validate validates mount specification.
func (m *MountSpec) Validate() error {
	// Validate type
	switch m.Type {
	case "bind", "tmpfs", "proc":
		// Valid types
	default:
		return fmt.Errorf("invalid mount type: %s (must be bind, tmpfs, or proc)", m.Type)
	}

	// Validate destination
	if m.Destination == "" {
		return fmt.Errorf("mount destination is required")
	}

	if !filepath.IsAbs(m.Destination) {
		return fmt.Errorf("mount destination must be absolute: %s", m.Destination)
	}

	// Validate source for bind mounts
	if m.Type == "bind" && m.Source == "" {
		return fmt.Errorf("bind mount requires source")
	}

	// Validate options
	validOptions := map[string]bool{
		"ro":     true,
		"rw":     true,
		"nosuid": true,
		"nodev":  true,
		"noexec": true,
		"bind":   true,
		"rbind":  true,
	}

	for _, opt := range m.Options {
		if !validOptions[opt] {
			return fmt.Errorf("invalid mount option: %s", opt)
		}
	}

	// Check for conflicting ro/rw options
	hasRO := false
	hasRW := false
	for _, opt := range m.Options {
		if opt == "ro" {
			hasRO = true
		}
		if opt == "rw" {
			hasRW = true
		}
	}
	if hasRO && hasRW {
		return fmt.Errorf("conflicting mount options: ro and rw")
	}

	return nil
}

// Validate validates resource specification.
func (r *ResourceSpec) Validate() error {
	// Validate memory format (e.g., "2GB", "512MB")
	if r.Memory != "" {
		if !isValidMemorySpec(r.Memory) {
			return fmt.Errorf("invalid memory specification: %s (examples: 512MB, 2GB)", r.Memory)
		}
	}

	// Validate CPU shares (1-10000, default 1024)
	if r.CPUShares != 0 && (r.CPUShares < 1 || r.CPUShares > 10000) {
		return fmt.Errorf("cpu_shares must be between 1 and 10000: %d", r.CPUShares)
	}

	// Validate IO weight (1-10000)
	if r.IOWeight != 0 && (r.IOWeight < 1 || r.IOWeight > 10000) {
		return fmt.Errorf("io_weight must be between 1 and 10000: %d", r.IOWeight)
	}

	return nil
}

// isValidMemorySpec checks if a memory specification is valid.
func isValidMemorySpec(spec string) bool {
	// Simple validation: must end with MB, GB, or KB
	if len(spec) < 3 {
		return false
	}

	suffix := spec[len(spec)-2:]
	if suffix != "MB" && suffix != "GB" && suffix != "KB" {
		return false
	}

	// Check numeric prefix
	numPart := spec[:len(spec)-2]
	if len(numPart) == 0 {
		return false
	}

	for _, c := range numPart {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

// String returns a human-readable representation of the container spec.
func (j *ContainerSpec) String() string {
	return fmt.Sprintf("ContainerSpec{name=%s, version=%s, mounts=%d}",
		j.Metadata.Name, j.Version, len(j.Mounts))
}
