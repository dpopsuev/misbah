package model

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Mount type constants.
const (
	MountTypeBind     = "bind"
	MountTypeTmpfs    = "tmpfs"
	MountTypeProc     = "proc"
	MountTypeGitClone = "git-clone"
)

// Runtime backend constants.
const (
	RuntimeNamespace = "namespace"
	RuntimeKata      = "kata"
)

// ContainerInfo holds runtime information about a container.
type ContainerInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	SandboxID string `json:"sandbox_id,omitempty"`
	CreatedAt int64  `json:"created_at"`
	ExitCode  int32  `json:"exit_code"`
}

// ContainerSpec represents a container specification conforming to MSB-SPC-2026-001.
type ContainerSpec struct {
	Version    string            `yaml:"version"`
	Metadata   ContainerMetadata `yaml:"metadata"`
	Process    ProcessSpec       `yaml:"process"`
	Namespaces NamespaceSpec     `yaml:"namespaces"`
	Mounts     []MountSpec       `yaml:"mounts"`
	Resources  *ResourceSpec     `yaml:"resources,omitempty"`
	Image      string            `yaml:"image,omitempty"`   // OCI image ref (required for kata)
	Runtime    string            `yaml:"runtime,omitempty"` // "" or "namespace" = Phase 1, "kata" = CRI
	Network    *NetworkConfig    `yaml:"network,omitempty"` // Network configuration (kata only)
	TierConfig     *TierConfig          `yaml:"tier,omitempty"`         // Tier-scoped mount configuration
	Nesting        *NestingConfig       `yaml:"nesting,omitempty"`     // Nesting configuration for recursive containers
	Permissions    *PermissionConfig    `yaml:"permissions,omitempty"` // Permission daemon configuration
	VsockForwarder *VsockForwarderConfig `yaml:"-" json:"-"`            // Runtime-injected vsock config (Kata only)
}

// NetworkConfig specifies network configuration for CRI containers.
type NetworkConfig struct {
	Mode     string   `yaml:"mode,omitempty"`     // "none", "host", "pod" (default: "pod")
	DNSServers []string `yaml:"dns_servers,omitempty"`
	DNSSearch  []string `yaml:"dns_search,omitempty"`
	Hostname   string   `yaml:"hostname,omitempty"`
}

// NestingConfig specifies nesting configuration for recursive container creation inside VMs.
type NestingConfig struct {
	Enabled          bool   `yaml:"enabled"`
	MaxDepth         int    `yaml:"max_depth,omitempty"`         // 1-10, default 3
	PermissionPolicy string `yaml:"permission_policy,omitempty"` // "inherit", "restrict", "deny"
}

// Validate validates nesting configuration.
func (n *NestingConfig) Validate() error {
	if !n.Enabled {
		return nil
	}

	if n.MaxDepth < 1 || n.MaxDepth > 10 {
		return fmt.Errorf("nesting max_depth must be between 1 and 10: %d", n.MaxDepth)
	}

	switch n.PermissionPolicy {
	case "", "inherit", "restrict", "deny":
		// Valid policies
	default:
		return fmt.Errorf("invalid nesting permission_policy: %q (must be inherit, restrict, or deny)", n.PermissionPolicy)
	}

	return nil
}

// TierConfig specifies tier-scoped mount configuration for agent isolation.
type TierConfig struct {
	Tier          string   `yaml:"tier"`                       // eco, sys, com, mod
	WritablePaths []string `yaml:"writable_paths,omitempty"`   // relative paths within workspace to make RW
}

// Validate validates tier configuration.
func (t *TierConfig) Validate() error {
	switch t.Tier {
	case "eco", "sys", "com", "mod":
		// Valid tiers
	default:
		return fmt.Errorf("invalid tier: %q (must be eco, sys, com, or mod)", t.Tier)
	}

	if t.Tier == "eco" && len(t.WritablePaths) > 0 {
		return fmt.Errorf("eco tier must not have writable paths (read-only)")
	}

	return nil
}

// VsockForwarderConfig holds runtime-injected vsock forwarder settings for Kata containers.
type VsockForwarderConfig struct {
	Port   uint32 `yaml:"-" json:"-"` // vsock port on host
	BinDir string `yaml:"-" json:"-"` // host path to misbah binaries
}

// PermissionConfig specifies permission daemon configuration for progressive trust.
type PermissionConfig struct {
	NetworkWhitelist []string `yaml:"network_whitelist,omitempty"` // allowed domains
	MCPWhitelist     []string `yaml:"mcp_whitelist,omitempty"`     // allowed MCP tools
	PackageWhitelist []string `yaml:"package_whitelist,omitempty"` // allowed packages
	DefaultPolicy    string   `yaml:"default_policy,omitempty"`    // "deny" (default) or "prompt"
}

// Validate validates permission configuration.
func (p *PermissionConfig) Validate() error {
	switch p.DefaultPolicy {
	case "", "deny", "prompt":
		// Valid policies
	default:
		return fmt.Errorf("invalid default_policy: %q (must be \"deny\" or \"prompt\")", p.DefaultPolicy)
	}
	return nil
}

// ContainerMetadata contains container metadata.
type ContainerMetadata struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
}

// ProcessSpec specifies the process to execute in the container.
type ProcessSpec struct {
	Command   []string `yaml:"command"`
	Env       []string `yaml:"env,omitempty"`
	Cwd       string   `yaml:"cwd"`
	NetNsName string   `yaml:"-" json:"-"` // injected at runtime for pre-created network namespace
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
	Type        string        `yaml:"type"`                  // bind, tmpfs, proc, git-clone
	Source      string        `yaml:"source,omitempty"`
	Destination string        `yaml:"destination"`
	Options     []string      `yaml:"options,omitempty"`     // ro, rw, nosuid, nodev, etc.
	GitClone    *GitCloneSpec `yaml:"git_clone,omitempty"`   // required when type is "git-clone"
}

// GitCloneSpec specifies a remote repository to clone and mount.
type GitCloneSpec struct {
	Repository string `yaml:"repository"`       // remote repo URL (required)
	Ref        string `yaml:"ref,omitempty"`     // branch, tag, or commit SHA
	Depth      int    `yaml:"depth,omitempty"`   // shallow clone depth (default 1)
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

	// Validate runtime
	switch j.Runtime {
	case "", RuntimeNamespace:
		// Phase 1 namespace backend, no extra requirements
	case RuntimeKata:
		if j.Image == "" {
			return fmt.Errorf("image is required when runtime is %q", RuntimeKata)
		}
	default:
		return fmt.Errorf("unsupported runtime: %s (must be %q, %q, or empty)", j.Runtime, RuntimeNamespace, RuntimeKata)
	}

	// Validate network config (if specified)
	if j.Network != nil {
		if err := j.Network.Validate(); err != nil {
			return fmt.Errorf("invalid network: %w", err)
		}
	}

	// Validate tier config (if specified)
	if j.TierConfig != nil {
		if err := j.TierConfig.Validate(); err != nil {
			return fmt.Errorf("invalid tier: %w", err)
		}
	}

	// Validate nesting config (if specified)
	if j.Nesting != nil {
		if err := j.Nesting.Validate(); err != nil {
			return fmt.Errorf("invalid nesting: %w", err)
		}
	}

	// Validate permissions config (if specified)
	if j.Permissions != nil {
		if err := j.Permissions.Validate(); err != nil {
			return fmt.Errorf("invalid permissions: %w", err)
		}
	}

	return nil
}

// Validate validates network configuration.
func (n *NetworkConfig) Validate() error {
	switch n.Mode {
	case "", "pod", "none", "host":
		// Valid modes
	default:
		return fmt.Errorf("invalid network mode: %s (must be \"none\", \"host\", or \"pod\")", n.Mode)
	}
	return nil
}

// Validate validates container metadata.
func (m *ContainerMetadata) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("container name is required")
	}

	if err := validateContainerName(m.Name); err != nil {
		return fmt.Errorf("invalid container name: %w", err)
	}

	return nil
}

// validateContainerName checks that a name contains only alphanumeric chars, dashes, and underscores.
func validateContainerName(name string) error {
	if name[0] == '-' || name[0] == '_' {
		return fmt.Errorf("%w: %s (cannot start with dash or underscore)", ErrInvalidContainerName, name)
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return fmt.Errorf("%w: %q (only alphanumeric, dash, and underscore allowed)", ErrInvalidContainerName, string(c))
		}
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
	case MountTypeBind, MountTypeTmpfs, MountTypeProc:
		// Valid types
	case MountTypeGitClone:
		// git-clone requires GitClone spec, source is auto-generated
		if m.GitClone == nil {
			return fmt.Errorf("git-clone mount requires git_clone specification")
		}
		if m.GitClone.Repository == "" {
			return fmt.Errorf("git-clone mount requires repository")
		}
		if m.Source != "" {
			return fmt.Errorf("git-clone mount must not specify source (auto-generated)")
		}
	default:
		return fmt.Errorf("invalid mount type: %s (must be bind, tmpfs, proc, or git-clone)", m.Type)
	}

	// Validate destination
	if m.Destination == "" {
		return fmt.Errorf("mount destination is required")
	}

	if !filepath.IsAbs(m.Destination) {
		return fmt.Errorf("mount destination must be absolute: %s", m.Destination)
	}

	// Validate source for bind mounts
	if m.Type == MountTypeBind && m.Source == "" {
		return fmt.Errorf("bind mount requires source")
	}

	// Validate options
	validOptions := map[string]bool{
		"ro":     true,
		"rw":     true,
		"nosuid": true,
		"nodev":  true,
		"noexec": true,
		MountTypeBind: true,
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
