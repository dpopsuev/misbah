package proxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/dpopsuev/misbah/metrics"
)

// PackageManager identifies which package manager is being wrapped.
type PackageManager string

const (
	PackageManagerApt PackageManager = "apt"
	PackageManagerPip PackageManager = "pip"
	PackageManagerNpm PackageManager = "npm"
)

// PackageWrapper intercepts package manager commands and checks permissions.
type PackageWrapper struct {
	checker   PermissionChecker
	container string
	logger    *metrics.Logger

	mu    sync.RWMutex
	cache map[string]Decision
}

// NewPackageWrapper creates a new package wrapper.
func NewPackageWrapper(checker PermissionChecker, container string, logger *metrics.Logger) *PackageWrapper {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &PackageWrapper{
		checker:   checker,
		container: container,
		logger:    logger,
		cache:     make(map[string]Decision),
	}
}

// Wrap checks permissions for the given packages, then executes the real command if allowed.
// Returns the exit code.
func (pw *PackageWrapper) Wrap(ctx context.Context, pm PackageManager, realBinary string, args []string) int {
	packages := ExtractPackages(pm, args)

	// If no packages found (e.g., apt update, pip --version), pass through
	if len(packages) == 0 {
		return pw.exec(realBinary, args)
	}

	// Check each package
	for _, pkg := range packages {
		decision, err := pw.checkPermission(ctx, pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "misbah: permission check failed for %s: %v\n", pkg, err)
			return 1
		}

		if decision != DecisionAlways && decision != DecisionOnce {
			fmt.Fprintf(os.Stderr, "misbah: package %q denied by permission policy\n", pkg)
			return 1
		}
	}

	// All packages approved — execute real command
	return pw.exec(realBinary, args)
}

func (pw *PackageWrapper) checkPermission(ctx context.Context, pkg string) (Decision, error) {
	pw.mu.RLock()
	if d, ok := pw.cache[pkg]; ok {
		pw.mu.RUnlock()
		return d, nil
	}
	pw.mu.RUnlock()

	req := PermissionRequest{
		Container:    pw.container,
		ResourceType: ResourcePackage,
		ResourceID:   pkg,
		Description:  fmt.Sprintf("Install package: %s", pkg),
	}

	resp, err := pw.checker.Check(ctx, req)
	if err != nil {
		return DecisionDeny, err
	}

	if resp.Decision == DecisionAlways || resp.Decision == DecisionDeny {
		pw.mu.Lock()
		pw.cache[pkg] = resp.Decision
		pw.mu.Unlock()
		return resp.Decision, nil
	}

	resp, err = pw.checker.Request(ctx, req)
	if err != nil {
		return DecisionDeny, err
	}

	if resp.Decision == DecisionAlways || resp.Decision == DecisionDeny {
		pw.mu.Lock()
		pw.cache[pkg] = resp.Decision
		pw.mu.Unlock()
	}

	return resp.Decision, nil
}

func (pw *PackageWrapper) exec(binary string, args []string) int {
	cmd := exec.Command(binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}

// ExtractPackages parses package names from package manager CLI arguments.
// Returns only the package names (no flags, no subcommands).
func ExtractPackages(pm PackageManager, args []string) []string {
	switch pm {
	case PackageManagerApt:
		return extractAptPackages(args)
	case PackageManagerPip:
		return extractPipPackages(args)
	case PackageManagerNpm:
		return extractNpmPackages(args)
	default:
		return nil
	}
}

// extractAptPackages handles: apt install pkg1 pkg2, apt-get install pkg1
func extractAptPackages(args []string) []string {
	// Find "install" subcommand
	installIdx := -1
	for i, arg := range args {
		if arg == "install" || arg == "add" {
			installIdx = i
			break
		}
	}
	if installIdx < 0 {
		return nil
	}

	var packages []string
	for _, arg := range args[installIdx+1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Strip version specifier: pkg=1.0 -> pkg
		if idx := strings.IndexByte(arg, '='); idx > 0 {
			arg = arg[:idx]
		}
		if arg != "" {
			packages = append(packages, arg)
		}
	}
	return packages
}

// extractPipPackages handles: pip install pkg1 pkg2, pip install -r requirements.txt
func extractPipPackages(args []string) []string {
	installIdx := -1
	for i, arg := range args {
		if arg == "install" {
			installIdx = i
			break
		}
	}
	if installIdx < 0 {
		return nil
	}

	var packages []string
	skipNext := false
	for _, arg := range args[installIdx+1:] {
		if skipNext {
			skipNext = false
			continue
		}
		// Flags that take a value
		if arg == "-r" || arg == "--requirement" || arg == "-c" || arg == "--constraint" ||
			arg == "-e" || arg == "--editable" || arg == "-t" || arg == "--target" ||
			arg == "-i" || arg == "--index-url" || arg == "--extra-index-url" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Strip version specifier: pkg==1.0 -> pkg, pkg>=2.0 -> pkg
		name := stripPipVersion(arg)
		if name != "" {
			packages = append(packages, name)
		}
	}
	return packages
}

// extractNpmPackages handles: npm install pkg1 pkg2, npm i pkg1
func extractNpmPackages(args []string) []string {
	installIdx := -1
	for i, arg := range args {
		if arg == "install" || arg == "i" || arg == "add" {
			installIdx = i
			break
		}
	}
	if installIdx < 0 {
		return nil
	}

	var packages []string
	for _, arg := range args[installIdx+1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Strip version: pkg@1.0 -> pkg
		if idx := strings.LastIndexByte(arg, '@'); idx > 0 {
			arg = arg[:idx]
		}
		if arg != "" {
			packages = append(packages, arg)
		}
	}
	return packages
}

// stripPipVersion removes version specifiers from pip package specs.
// "numpy==1.24" -> "numpy", "numpy>=2.0" -> "numpy", "numpy" -> "numpy"
func stripPipVersion(spec string) string {
	for i, c := range spec {
		if c == '=' || c == '>' || c == '<' || c == '!' || c == '~' {
			return spec[:i]
		}
	}
	return spec
}

// WrapperScript generates a shell wrapper script for a package manager.
// The wrapper calls the misbah package checker before the real binary.
func WrapperScript(pm PackageManager, realBinaryPath, daemonSocket, container string) string {
	return fmt.Sprintf(`#!/bin/sh
# Misbah package wrapper for %s
# Checks permissions via daemon before executing the real package manager

MISBAH_DAEMON_SOCKET="%s" MISBAH_CONTAINER="%s" \
  misbah-pkg-check %s %s "$@"
`, pm, daemonSocket, container, pm, realBinaryPath)
}
