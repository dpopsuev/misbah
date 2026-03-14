package provider

import (
	"fmt"

	"github.com/dpopsuev/misbah/model"
)

// registry holds all registered providers.
var registry = make(map[string]model.Provider)

// Register registers a provider.
func Register(provider model.Provider) {
	registry[provider.Name()] = provider
}

// Get returns a provider by name.
func Get(name string) (model.Provider, error) {
	provider, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", model.ErrProviderNotFound, name)
	}
	return provider, nil
}

// List returns all registered provider names.
func List() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// Exists checks if a provider is registered.
func Exists(name string) bool {
	_, ok := registry[name]
	return ok
}

// GetProviderInfo returns information about all registered providers.
func GetProviderInfo() []model.ProviderInfo {
	infos := make([]model.ProviderInfo, 0, len(registry))
	for _, provider := range registry {
		infos = append(infos, model.ProviderInfo{
			Name:        provider.Name(),
			BinaryName:  provider.BinaryName(),
			ConfigDir:   provider.ConfigDir(),
			Description: fmt.Sprintf("%s provider", provider.Name()),
			Supported:   true,
		})
	}
	return infos
}

func init() {
	// Register built-in providers
	Register(&ClaudeProvider{})
	Register(&AiderProvider{})
	Register(&CursorProvider{})
}
