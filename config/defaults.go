package config

import (
	"os"
	"path/filepath"
	"strconv"
)

const (
	// DefaultConfigDir is the default configuration directory.
	DefaultConfigDir = ".config/misbah"

	// DefaultTempDir is the default temporary directory for mounts.
	DefaultTempDir = "/tmp/misbah"

	// DefaultLocksDir is the default directory for lock files (relative to TempDir).
	DefaultLocksDir = ".locks"

	// LockFileExt is the lock file extension.
	LockFileExt = ".lock"

	// DefaultCgroupRoot is the root cgroup v2 directory.
	DefaultCgroupRoot = "/sys/fs/cgroup"

	// CgroupSubdir is the subdirectory for Misbah cgroups.
	CgroupSubdir = "misbah"

	// ContainerSpecVersion is the current container specification version.
	ContainerSpecVersion = "1.0"

	// // DefaultContainerWorkspace is the default working directory inside containers.
	DefaultContainerWorkspace = "/container/workspace"
)

// Permission daemon defaults.
const (
	DefaultDaemonSocket  = "/run/misbah/permission.sock"
	DefaultAuditLogPath  = "audit.log"
	DefaultWhitelistPath = "whitelist.yaml"
)

// Network proxy defaults.
const (
	DefaultProxyPort = 8118
	EnvProxyPort     = "MISBAH_PROXY_PORT"
)

// Environment variable names.
const (
	// EnvDaemonSocket overrides the daemon socket path.
	EnvDaemonSocket = "MISBAH_DAEMON_SOCKET"

	// EnvConfigDir overrides the config directory.
	EnvConfigDir = "MISBAH_CONFIG_DIR"

	// EnvTempDir overrides the temp directory.
	EnvTempDir = "MISBAH_TEMP_DIR"

	// EnvContainer is set inside the container to identify it.
	EnvContainer = "MISBAH_CONTAINER"

	// EnvCRIEndpoint overrides the CRI endpoint.
	EnvCRIEndpoint = "MISBAH_CRI_ENDPOINT"

	// EnvRuntimeHandler overrides the default runtime handler.
	EnvRuntimeHandler = "MISBAH_RUNTIME_HANDLER"
)

// CRI defaults.
const (
	DefaultCRIEndpoint    = "unix:///run/containerd/containerd.sock"
	DefaultRuntimeHandler = "kata"
	DefaultCRITimeout     = 120
)

// Vsock transport defaults.
const (
	DefaultVsockPort   = uint32(DefaultProxyPort) // host vsock listen port
	DefaultVsockBinDir = "/usr/local/lib/misbah"  // host path to misbah binaries
	VsockHostCID       = 2                        // vsock CID for the host
	ForwarderBinName   = "misbah-vsock-fwd"       // forwarder binary name
	ForwarderMountPath = "/opt/misbah/bin"        // container-side mount path
	DefaultNoProxy     = "localhost,127.0.0.1"    // default NO_PROXY value for VM-local forwarder
)

// GetConfigDir returns the configuration directory path.
func GetConfigDir() string {
	if configDir := os.Getenv(EnvConfigDir); configDir != "" {
		return configDir
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("/tmp", DefaultConfigDir)
	}
	return filepath.Join(homeDir, DefaultConfigDir)
}

// GetTempDir returns the temporary directory for mounts.
func GetTempDir() string {
	if tempDir := os.Getenv(EnvTempDir); tempDir != "" {
		return tempDir
	}
	return DefaultTempDir
}

// GetLocksDir returns the locks directory path.
func GetLocksDir() string {
	return filepath.Join(GetTempDir(), DefaultLocksDir)
}

// GetLockPath returns the path to a workspace's lock file.
func GetLockPath(workspace string) string {
	return filepath.Join(GetLocksDir(), workspace+LockFileExt)
}

// GetMountPath returns the mount path for a workspace.
func GetMountPath(workspace string) string {
	return filepath.Join(GetTempDir(), workspace)
}

// GetCRIEndpoint returns the CRI endpoint, checking env override first.
func GetCRIEndpoint() string {
	if ep := os.Getenv(EnvCRIEndpoint); ep != "" {
		return ep
	}
	return DefaultCRIEndpoint
}

// GetRuntimeHandler returns the runtime handler, checking env override first.
func GetRuntimeHandler() string {
	if rh := os.Getenv(EnvRuntimeHandler); rh != "" {
		return rh
	}
	return DefaultRuntimeHandler
}

// GetDaemonSocket returns the daemon socket path, checking env override first.
func GetDaemonSocket() string {
	if sock := os.Getenv(EnvDaemonSocket); sock != "" {
		return sock
	}
	return DefaultDaemonSocket
}

// GetAuditLogPath returns the audit log path (relative to config dir).
func GetAuditLogPath() string {
	return filepath.Join(GetConfigDir(), DefaultAuditLogPath)
}

// GetWhitelistPath returns the whitelist file path (relative to config dir).
func GetWhitelistPath() string {
	return filepath.Join(GetConfigDir(), DefaultWhitelistPath)
}

// GetProxyPort returns the proxy port, checking env override first.
func GetProxyPort() int {
	if port := os.Getenv(EnvProxyPort); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			return p
		}
	}
	return DefaultProxyPort
}
