package cri

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/misbah/model"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)


// Mount option constants.
const (
	MountOptReadOnly = "ro"
	MountOptNoSuid   = "nosuid"
	MountOptNoDev    = "nodev"
	MountOptNoExec   = "noexec"
)

// MountsToContainerMounts converts model MountSpecs to CRI Mount format.
func MountsToContainerMounts(mounts []model.MountSpec) []*runtimeapi.Mount {
	var result []*runtimeapi.Mount

	for _, m := range mounts {
		criMount := &runtimeapi.Mount{
			ContainerPath: m.Destination,
			HostPath:      m.Source,
		}

		for _, opt := range m.Options {
			switch opt {
			case MountOptReadOnly:
				criMount.Readonly = true
			case MountOptNoSuid, MountOptNoDev, MountOptNoExec:
				criMount.Propagation = runtimeapi.MountPropagation_PROPAGATION_PRIVATE
			}
		}

		result = append(result, criMount)
	}

	return result
}

// ResourcesToLinuxResources converts model ResourceSpec to CRI LinuxContainerResources.
func ResourcesToLinuxResources(resources *model.ResourceSpec) *runtimeapi.LinuxContainerResources {
	if resources == nil {
		return nil
	}

	result := &runtimeapi.LinuxContainerResources{}

	if resources.Memory != "" {
		if bytes, err := parseMemoryToBytes(resources.Memory); err == nil {
			result.MemoryLimitInBytes = bytes
		}
	}

	if resources.CPUShares > 0 {
		result.CpuShares = int64(resources.CPUShares)
	}

	return result
}

// EnvToKeyValues converts KEY=VALUE string pairs to CRI KeyValue format.
func EnvToKeyValues(envs []string) []*runtimeapi.KeyValue {
	var result []*runtimeapi.KeyValue

	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		kv := &runtimeapi.KeyValue{Key: parts[0]}
		if len(parts) == 2 {
			kv.Value = parts[1]
		}
		result = append(result, kv)
	}

	return result
}

// BuildPodSandboxConfig creates a PodSandboxConfig for a Misbah container.
// Kata annotations (from daemon config) are injected as OCI annotations
// for per-pod runtime configuration without editing global Kata config.
func BuildPodSandboxConfig(name string, annotations map[string]string) *runtimeapi.PodSandboxConfig {
	config := &runtimeapi.PodSandboxConfig{
		Metadata: &runtimeapi.PodSandboxMetadata{
			Name:      name,
			Namespace: "misbah",
			Uid:       fmt.Sprintf("misbah-%s", name),
			Attempt:   0,
		},
		Labels: map[string]string{
			"misbah.dev/managed": "true",
			"misbah.dev/name":    name,
		},
		Linux: &runtimeapi.LinuxPodSandboxConfig{},
	}

	if len(annotations) > 0 {
		config.Annotations = annotations
	}

	return config
}

// ApplyNetworkConfig applies NetworkConfig to a PodSandboxConfig.
func ApplyNetworkConfig(sandboxConfig *runtimeapi.PodSandboxConfig, network *model.NetworkConfig) {
	if network == nil {
		return
	}

	if network.Hostname != "" {
		sandboxConfig.Hostname = network.Hostname
	}

	if len(network.DNSServers) > 0 || len(network.DNSSearch) > 0 {
		sandboxConfig.DnsConfig = &runtimeapi.DNSConfig{
			Servers:  network.DNSServers,
			Searches: network.DNSSearch,
		}
	}

	if sandboxConfig.Linux == nil {
		sandboxConfig.Linux = &runtimeapi.LinuxPodSandboxConfig{}
	}

	switch network.Mode {
	case "none":
		sandboxConfig.Linux.SecurityContext = &runtimeapi.LinuxSandboxSecurityContext{
			NamespaceOptions: &runtimeapi.NamespaceOption{
				Network: runtimeapi.NamespaceMode_TARGET,
			},
		}
	case "host":
		sandboxConfig.Linux.SecurityContext = &runtimeapi.LinuxSandboxSecurityContext{
			NamespaceOptions: &runtimeapi.NamespaceOption{
				Network: runtimeapi.NamespaceMode_NODE,
			},
		}
	// "pod" or "" = default CRI behavior (own network namespace)
	}
}

// BuildContainerConfig creates a ContainerConfig from a ContainerSpec.
func BuildContainerConfig(spec *model.ContainerSpec) *runtimeapi.ContainerConfig {
	config := &runtimeapi.ContainerConfig{
		Metadata: &runtimeapi.ContainerMetadata{
			Name:    spec.Metadata.Name,
			Attempt: 0,
		},
		Image: &runtimeapi.ImageSpec{
			Image: spec.Image,
		},
		Command:    spec.Process.Command,
		WorkingDir: spec.Process.Cwd,
		Envs:       EnvToKeyValues(spec.Process.Env),
		Mounts:     MountsToContainerMounts(spec.Mounts),
		Labels: map[string]string{
			"misbah.dev/managed": "true",
			"misbah.dev/name":    spec.Metadata.Name,
		},
	}

	if spec.Resources != nil {
		config.Linux = &runtimeapi.LinuxContainerConfig{
			Resources: ResourcesToLinuxResources(spec.Resources),
		}
	}

	return config
}

// parseMemoryToBytes converts a memory spec like "2GB" to bytes.
func parseMemoryToBytes(spec string) (int64, error) {
	if len(spec) < 3 {
		return 0, fmt.Errorf("invalid memory spec: %s", spec)
	}

	suffix := strings.ToUpper(spec[len(spec)-2:])
	numStr := spec[:len(spec)-2]

	var num int64
	for _, c := range numStr {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid memory spec number: %s", numStr)
		}
		num = num*10 + int64(c-'0')
	}

	switch suffix {
	case "KB":
		return num * 1024, nil
	case "MB":
		return num * 1024 * 1024, nil
	case "GB":
		return num * 1024 * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("invalid memory suffix: %s", suffix)
	}
}
