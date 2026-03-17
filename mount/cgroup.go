package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dpopsuev/misbah/model"
)

// // CgroupManager manages cgroup v2 resource limits for containers.
type CgroupManager struct {
	cgroupRoot string
	containerName   string
}

// NewCgroupManager creates a new cgroup manager for a container.
func NewCgroupManager(containerName string) *CgroupManager {
	return &CgroupManager{
		cgroupRoot: "/sys/fs/cgroup",
		containerName:   containerName,
	}
}

// Setup creates and configures cgroup for the container with specified resource limits.
func (c *CgroupManager) Setup(resources *model.ResourceSpec) error {
	// Skip if no resources specified
	if resources == nil {
		return nil
	}

	// Check if cgroup v2 is available
	if !c.isCgroupV2Available() {
		return fmt.Errorf("cgroup v2 not available (required for resource limits)")
	}

	// // Create cgroup directory for this container
	cgroupPath := c.getCgroupPath()
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup directory: %w", err)
	}

	// Set memory limit
	if resources.Memory != "" {
		if err := c.setMemoryLimit(resources.Memory); err != nil {
			return fmt.Errorf("failed to set memory limit: %w", err)
		}
	}

	// Set CPU shares (weight in cgroup v2)
	if resources.CPUShares > 0 {
		if err := c.setCPUWeight(resources.CPUShares); err != nil {
			return fmt.Errorf("failed to set CPU shares: %w", err)
		}
	}

	// Set IO weight
	if resources.IOWeight > 0 {
		if err := c.setIOWeight(resources.IOWeight); err != nil {
			return fmt.Errorf("failed to set IO weight: %w", err)
		}
	}

	return nil
}

// AddProcess adds a process to the cgroup.
func (c *CgroupManager) AddProcess(pid int) error {
	cgroupPath := c.getCgroupPath()
	procsFile := filepath.Join(cgroupPath, "cgroup.procs")

	pidStr := strconv.Itoa(pid)
	if err := os.WriteFile(procsFile, []byte(pidStr), 0644); err != nil {
		return fmt.Errorf("failed to add process %d to cgroup: %w", pid, err)
	}

	return nil
}

// // Cleanup removes the cgroup for this container.
func (c *CgroupManager) Cleanup() error {
	cgroupPath := c.getCgroupPath()

	// Remove cgroup directory (will fail if processes still in it)
	if err := os.Remove(cgroupPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove cgroup: %w", err)
		}
	}

	return nil
}

// getCgroupPath returns the full path to the container's cgroup directory.
func (c *CgroupManager) getCgroupPath() string {
	return filepath.Join(c.cgroupRoot, "misbah", c.containerName)
}

// isCgroupV2Available checks if cgroup v2 is available and mounted.
func (c *CgroupManager) isCgroupV2Available() bool {
	// Check if cgroup root exists
	if _, err := os.Stat(c.cgroupRoot); err != nil {
		return false
	}

	// Check for cgroup.controllers file (cgroup v2 indicator)
	controllersFile := filepath.Join(c.cgroupRoot, "cgroup.controllers")
	if _, err := os.Stat(controllersFile); err != nil {
		return false
	}

	return true
}

// setMemoryLimit sets the memory limit for the cgroup.
func (c *CgroupManager) setMemoryLimit(memorySpec string) error {
	// Parse memory spec (e.g., "2GB" -> 2147483648 bytes)
	bytes, err := parseMemorySpec(memorySpec)
	if err != nil {
		return err
	}

	cgroupPath := c.getCgroupPath()
	memoryMaxFile := filepath.Join(cgroupPath, "memory.max")

	if err := os.WriteFile(memoryMaxFile, []byte(fmt.Sprintf("%d", bytes)), 0644); err != nil {
		return fmt.Errorf("failed to write memory.max: %w", err)
	}

	return nil
}

// setCPUWeight sets the CPU weight (cgroup v2 equivalent of cpu.shares).
func (c *CgroupManager) setCPUWeight(shares int) error {
	// Convert shares (1-10000) to weight (1-10000)
	// cgroup v2 uses cpu.weight instead of cpu.shares
	weight := shares

	cgroupPath := c.getCgroupPath()
	cpuWeightFile := filepath.Join(cgroupPath, "cpu.weight")

	if err := os.WriteFile(cpuWeightFile, []byte(fmt.Sprintf("%d", weight)), 0644); err != nil {
		return fmt.Errorf("failed to write cpu.weight: %w", err)
	}

	return nil
}

// setIOWeight sets the IO weight for the cgroup.
func (c *CgroupManager) setIOWeight(weight int) error {
	cgroupPath := c.getCgroupPath()
	ioWeightFile := filepath.Join(cgroupPath, "io.weight")

	// io.weight format: "default WEIGHT"
	content := fmt.Sprintf("default %d", weight)

	if err := os.WriteFile(ioWeightFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write io.weight: %w", err)
	}

	return nil
}

// parseMemorySpec parses a memory specification like "2GB", "512MB" to bytes.
func parseMemorySpec(spec string) (int64, error) {
	if len(spec) < 3 {
		return 0, fmt.Errorf("invalid memory spec: %s", spec)
	}

	// Extract suffix
	suffix := spec[len(spec)-2:]
	numPart := spec[:len(spec)-2]

	// Parse numeric part
	num, err := strconv.ParseInt(numPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory spec number: %s", numPart)
	}

	// Convert to bytes based on suffix
	var multiplier int64
	switch strings.ToUpper(suffix) {
	case "KB":
		multiplier = 1024
	case "MB":
		multiplier = 1024 * 1024
	case "GB":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("invalid memory spec suffix: %s (must be KB, MB, or GB)", suffix)
	}

	return num * multiplier, nil
}
