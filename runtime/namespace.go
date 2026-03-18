package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dpopsuev/misbah/config"
	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"github.com/dpopsuev/misbah/proxy"
)

// // NamespaceManager manages Linux namespaces for containers.
type NamespaceManager struct {
	logger *metrics.Logger
}

// NewNamespaceManager creates a new namespace manager.
func NewNamespaceManager(logger *metrics.Logger) *NamespaceManager {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}

	return &NamespaceManager{
		logger: logger,
	}
}

// CreateContainer creates a new container with namespaces, mounts, and resource limits.
func (nm *NamespaceManager) CreateContainer(spec *model.ContainerSpec, cgroupMgr *CgroupManager) error {
	// Verify we're on Linux
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: containers are only supported on Linux (current OS: %s)",
			model.ErrNamespaceCreationFailed, runtime.GOOS)
	}

	// Validate container spec
	if err := spec.Validate(); err != nil {
		return fmt.Errorf("invalid container spec: %w", err)
	}

	nm.logger.Debugf("Creating container: %s", spec.Metadata.Name)

	// Check if unshare is available
	if _, err := exec.LookPath("unshare"); err != nil {
		return fmt.Errorf("%w: unshare command not found (install util-linux)",
			model.ErrNamespaceCreationFailed)
	}

	// Build the mount script
	mountScript := nm.buildMountScript(spec.Mounts)

	// Build the command to execute in the container
	shellCmd := nm.buildShellCommand(spec, mountScript)

	// Build unshare arguments based on namespace spec
	unshareArgs := nm.buildUnshareArgs(spec.Namespaces)

	// Add bash execution
	unshareArgs = append(unshareArgs, "bash", "-c", shellCmd)

	// Create unshare command
	cmd := exec.Command("unshare", unshareArgs...)

	// Set environment
	cmd.Env = append(os.Environ(), spec.Process.Env...)

	// Connect stdio
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	nm.logger.Infof("Executing container process: %v", spec.Process.Command)

	// Setup cgroup before starting process (if resources specified)
	if spec.Resources != nil && cgroupMgr != nil {
		if err := cgroupMgr.Setup(spec.Resources); err != nil {
			nm.logger.Warnf("Failed to setup cgroup: %v", err)
			// Continue anyway - cgroups are optional
		}
	}

	// Run the command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %v", model.ErrNamespaceCreationFailed, err)
	}

	// Cleanup cgroup
	if cgroupMgr != nil {
		if err := cgroupMgr.Cleanup(); err != nil {
			nm.logger.Warnf("Failed to cleanup cgroup: %v", err)
		}
	}

	nm.logger.Infof("Container exited successfully")
	return nil
}

// buildUnshareArgs builds the unshare command arguments based on namespace spec.
func (nm *NamespaceManager) buildUnshareArgs(ns model.NamespaceSpec) []string {
	args := []string{}

	// User namespace (required)
	if ns.User {
		args = append(args, "--user", "--map-root-user")
	}

	// Mount namespace (required)
	if ns.Mount {
		args = append(args, "--mount")
	}

	// PID namespace
	if ns.PID {
		args = append(args, "--pid", "--fork")
	}

	// Network namespace
	if ns.Network {
		args = append(args, "--net")
	}

	// IPC namespace
	if ns.IPC {
		args = append(args, "--ipc")
	}

	// UTS namespace (hostname)
	if ns.UTS {
		args = append(args, "--uts")
	}

	return args
}

// buildMountScript builds the shell script to create mounts.
func (nm *NamespaceManager) buildMountScript(mounts []model.MountSpec) string {
	var script strings.Builder

	for _, mount := range mounts {
		switch mount.Type {
		case model.MountTypeBind:
			script.WriteString(nm.buildBindMount(mount))
		case model.MountTypeTmpfs:
			script.WriteString(nm.buildTmpfsMount(mount))
		case model.MountTypeProc:
			script.WriteString(nm.buildProcMount(mount))
		default:
			nm.logger.Warnf("Unknown mount type: %s", mount.Type)
		}
	}

	// Mount daemon socket if it exists and is accessible
	socketPath := config.GetDaemonSocket()
	if _, err := os.Stat(socketPath); err == nil {
		socketDir := filepath.Dir(socketPath)
		fmt.Fprintf(&script, "if mkdir -p \"%s\" 2>/dev/null && touch \"%s\" 2>/dev/null; then\n", socketDir, socketPath)
		fmt.Fprintf(&script, "  mount --bind \"%s\" \"%s\"\n", socketPath, socketPath)
		fmt.Fprintf(&script, "fi\n")
		nm.logger.Debugf("Daemon socket mount added: %s", socketPath)
	}

	return script.String()
}

// buildBindMount builds a bind mount command.
func (nm *NamespaceManager) buildBindMount(mount model.MountSpec) string {
	var script strings.Builder

	// Create destination directory
	script.WriteString(fmt.Sprintf("mkdir -p \"%s\"\n", mount.Destination))

	// Build mount options
	options := "bind"
	for _, opt := range mount.Options {
		if opt != "bind" && opt != "rbind" {
			options += "," + opt
		}
	}

	// Create bind mount
	script.WriteString(fmt.Sprintf("mount --bind -o %s \"%s\" \"%s\"\n",
		options, mount.Source, mount.Destination))

	nm.logger.Debugf("Will bind mount %s -> %s (options: %s)",
		mount.Source, mount.Destination, options)

	return script.String()
}

// buildTmpfsMount builds a tmpfs mount command.
func (nm *NamespaceManager) buildTmpfsMount(mount model.MountSpec) string {
	var script strings.Builder

	// Create destination directory
	script.WriteString(fmt.Sprintf("mkdir -p \"%s\"\n", mount.Destination))

	// Build mount options
	options := ""
	if len(mount.Options) > 0 {
		options = " -o " + strings.Join(mount.Options, ",")
	}

	// Create tmpfs mount
	script.WriteString(fmt.Sprintf("mount -t tmpfs%s tmpfs \"%s\"\n",
		options, mount.Destination))

	nm.logger.Debugf("Will tmpfs mount %s", mount.Destination)

	return script.String()
}

// buildProcMount builds a proc filesystem mount command.
func (nm *NamespaceManager) buildProcMount(mount model.MountSpec) string {
	var script strings.Builder

	// Create destination directory
	script.WriteString(fmt.Sprintf("mkdir -p \"%s\"\n", mount.Destination))

	// Create proc mount
	script.WriteString(fmt.Sprintf("mount -t proc proc \"%s\"\n", mount.Destination))

	nm.logger.Debugf("Will proc mount %s", mount.Destination)

	return script.String()
}

// shellQuote escapes a string for safe embedding in a POSIX shell command.
// Uses single-quote wrapping with escaped embedded single quotes.
func shellQuote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
}

// buildShellCommand builds the complete shell command to execute in the container.
func (nm *NamespaceManager) buildShellCommand(spec *model.ContainerSpec, mountScript string) string {
	var quoted []string
	for _, arg := range spec.Process.Command {
		quoted = append(quoted, shellQuote(arg))
	}
	cmdStr := strings.Join(quoted, " ")

	// Build proxy startup script if daemon socket is available
	proxyScript := nm.buildProxyScript(spec)

	return fmt.Sprintf(`
		set -e
		%s
		%s
		cd "%s"
		exec %s
	`, mountScript, proxyScript, spec.Process.Cwd, cmdStr)
}

// buildProxyScript generates shell commands to start the network proxy in background.
// Returns empty string if the daemon socket is not available.
func (nm *NamespaceManager) buildProxyScript(spec *model.ContainerSpec) string {
	socketPath := config.GetDaemonSocket()
	if _, err := os.Stat(socketPath); err != nil {
		return ""
	}

	// Look for misbah-proxy binary next to the misbah binary, or in PATH
	proxyBin, err := exec.LookPath("misbah-proxy")
	if err != nil {
		// Try same directory as current executable
		if self, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(self), "misbah-proxy")
			if _, err := os.Stat(candidate); err == nil {
				proxyBin = candidate
			}
		}
	}
	if proxyBin == "" {
		nm.logger.Warnf("misbah-proxy binary not found, skipping proxy startup")
		return ""
	}

	containerName := spec.Metadata.Name
	listenAddr := fmt.Sprintf("127.0.0.1:%d", proxy.DefaultProxyPort)

	return fmt.Sprintf(`
		# Start network proxy in background
		%s --socket %s --container %s --listen %s &
		export HTTP_PROXY=http://%s
		export HTTPS_PROXY=http://%s
		export http_proxy=http://%s
		export https_proxy=http://%s
		export NO_PROXY=localhost,127.0.0.1
		export no_proxy=localhost,127.0.0.1
	`, shellQuote(proxyBin), shellQuote(socketPath), shellQuote(containerName), shellQuote(listenAddr),
		listenAddr, listenAddr, listenAddr, listenAddr)
}

// CheckNamespaceSupport checks if unprivileged user namespaces are supported.
func (nm *NamespaceManager) CheckNamespaceSupport() error {
	// Verify we're on Linux
	if runtime.GOOS != "linux" {
		return fmt.Errorf("namespaces are only supported on Linux (current OS: %s)", runtime.GOOS)
	}

	// Check if unshare is available
	if _, err := exec.LookPath("unshare"); err != nil {
		return fmt.Errorf("unshare command not found (install util-linux)")
	}

	// Try to create a simple namespace to test support
	cmd := exec.Command("unshare",
		"--user",
		"--mount",
		"--map-root-user",
		"--pid",
		"--fork",
		"true")

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unprivileged user namespaces are not available: %w\n"+
			"You may need to enable them:\n"+
			"  sudo sysctl -w kernel.unprivileged_userns_clone=1\n"+
			"  sudo sysctl -w user.max_user_namespaces=15000", err)
	}

	nm.logger.Debugf("Namespace support verified")
	return nil
}

