package mount

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jabal/jabal/metrics"
	"github.com/jabal/jabal/model"
)

// NamespaceManager manages Linux namespaces.
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

// CreateNamespace creates a new user+mount+pid namespace and executes a command.
func (nm *NamespaceManager) CreateNamespace(mountPath string, sources []model.Source, providerBinary string, env []string) error {
	// Verify we're on Linux
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: namespaces are only supported on Linux (current OS: %s)", model.ErrNamespaceCreationFailed, runtime.GOOS)
	}

	nm.logger.Debugf("Creating namespace for mount path %s", mountPath)

	// Check if unshare is available
	if _, err := exec.LookPath("unshare"); err != nil {
		return fmt.Errorf("%w: unshare command not found (install util-linux)", model.ErrNamespaceCreationFailed)
	}

	// Build the mount script
	mountScript := nm.buildMountScript(mountPath, sources)

	// Build the command to execute
	shellCmd := fmt.Sprintf(`
		set -e
		%s
		cd "%s"
		export JABAL_WORKSPACE="%s"
		exec %s
	`, mountScript, mountPath, mountPath, providerBinary)

	// Create unshare command
	cmd := exec.Command("unshare",
		"--user",              // User namespace
		"--mount",             // Mount namespace
		"--map-root-user",     // Map current user to root in namespace
		"--pid",               // PID namespace
		"--fork",              // Fork before executing
		"bash", "-c", shellCmd)

	// Set environment
	cmd.Env = append(os.Environ(), env...)

	// Connect stdio
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	nm.logger.Infof("Executing provider in namespace: %s", providerBinary)

	// Run the command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %v", model.ErrNamespaceCreationFailed, err)
	}

	nm.logger.Infof("Namespace exited successfully")
	return nil
}

// buildMountScript builds the shell script to create bind mounts.
func (nm *NamespaceManager) buildMountScript(mountPath string, sources []model.Source) string {
	var script strings.Builder

	// Create mount point directory
	script.WriteString(fmt.Sprintf("mkdir -p \"%s\"\n", mountPath))

	// Create bind mounts for each source
	for _, source := range sources {
		sourceMountPath := fmt.Sprintf("%s/%s", mountPath, source.Mount)
		script.WriteString(fmt.Sprintf("mkdir -p \"%s\"\n", sourceMountPath))
		script.WriteString(fmt.Sprintf("mount --bind \"%s\" \"%s\"\n", source.Path, sourceMountPath))
		nm.logger.Debugf("Will mount %s -> %s", source.Path, sourceMountPath)
	}

	return script.String()
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
