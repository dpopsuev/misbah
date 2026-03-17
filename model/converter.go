package model

import (
	"fmt"
	"path/filepath"
)

// WorkspaceToContainerSpec converts a Workspace (old model) to a ContainerSpec (new model).
// This provides backward compatibility during the transition period.
func WorkspaceToContainerSpec(w *Workspace, command []string) (*ContainerSpec, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("command is required")
	}

	// Convert workspace to container spec
	spec := &ContainerSpec{
		Version: "1.0",
		Metadata: ContainerMetadata{
			Name:        w.Name,
			Description: w.Description,
			Labels:      make(map[string]string),
		},
		Process: ProcessSpec{
			Command: command,
			Env:     []string{fmt.Sprintf("MISBAH_CONTAINER=%s", w.Name)},
			Cwd:     filepath.Join("/tmp/misbah", w.Name),
		},
		Namespaces: NamespaceSpec{
			User:  true,
			Mount: true,
			PID:   true,
		},
		Mounts: make([]MountSpec, 0, len(w.Sources)),
	}

	// Convert tags to labels
	for i, tag := range w.Tags {
		spec.Metadata.Labels[fmt.Sprintf("tag%d", i)] = tag
	}

	// Convert sources to bind mounts
	for _, source := range w.Sources {
		mount := MountSpec{
			Type:        "bind",
			Source:      source.Path,
			Destination: filepath.Join("/tmp/misbah", w.Name, source.Mount),
			Options:     []string{"rw"}, // Default to read-write
		}
		spec.Mounts = append(spec.Mounts, mount)
	}

	return spec, nil
}

// ManifestToContainerSpec converts a Manifest (old model) to a ContainerSpec (new model).
// This is a higher-level conversion that takes a manifest file and generates a container spec.
func ManifestToContainerSpec(manifest *Manifest, command []string) (*ContainerSpec, error) {
	// Convert manifest to workspace first
	sources, err := manifest.ToSources()
	if err != nil {
		return nil, fmt.Errorf("failed to convert sources: %w", err)
	}

	workspace := &Workspace{
		Name:        manifest.Name,
		Description: manifest.Description,
		Sources:     sources,
		Providers:   manifest.Providers,
		Tags:        manifest.Tags,
	}

	// Convert workspace to container spec
	return WorkspaceToContainerSpec(workspace, command)
}

// ContainerSpecToWorkspace converts a ContainerSpec back to a Workspace (for backward compatibility).
// Note: This conversion is lossy - process, namespaces, and resources information is discarded.
func ContainerSpecToWorkspace(spec *ContainerSpec) (*Workspace, error) {
	workspace := &Workspace{
		Name:        spec.Metadata.Name,
		Description: spec.Metadata.Description,
		Sources:     make([]Source, 0, len(spec.Mounts)),
		Providers:   make(map[string]interface{}),
		Tags:        make([]string, 0),
	}

	// Extract tags from labels
	for key, value := range spec.Metadata.Labels {
		if len(key) > 3 && key[:3] == "tag" {
			workspace.Tags = append(workspace.Tags, value)
		}
	}

	// Convert bind mounts to sources
	for _, mount := range spec.Mounts {
		if mount.Type == "bind" {
			// Extract mount name from destination
			mountName := filepath.Base(mount.Destination)

			source := Source{
				Path:  mount.Source,
				Mount: mountName,
			}
			workspace.Sources = append(workspace.Sources, source)
		}
	}

	return workspace, nil
}
