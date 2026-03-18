package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dpopsuev/misbah/metrics"
	"github.com/dpopsuev/misbah/model"
	"gopkg.in/yaml.v3"
)

// WhitelistStore manages persistent permission rules.
type WhitelistStore struct {
	mu       sync.RWMutex
	rules    map[string]Decision
	filePath string
	logger   *metrics.Logger
}

// NewWhitelistStore creates a new whitelist store.
func NewWhitelistStore(filePath string, logger *metrics.Logger) *WhitelistStore {
	if logger == nil {
		logger = metrics.GetDefaultLogger()
	}
	return &WhitelistStore{
		rules:    make(map[string]Decision),
		filePath: filePath,
		logger:   logger,
	}
}

func ruleKey(resourceType ResourceType, resourceID string) string {
	return fmt.Sprintf("%s:%s", resourceType, resourceID)
}

// Check returns the decision for a resource, and whether it was found.
func (w *WhitelistStore) Check(resourceType ResourceType, resourceID string) (Decision, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	d, ok := w.rules[ruleKey(resourceType, resourceID)]
	return d, ok
}

// Set stores a decision for a resource.
func (w *WhitelistStore) Set(resourceType ResourceType, resourceID string, decision Decision) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.rules[ruleKey(resourceType, resourceID)] = decision
}

// whitelistFile is the YAML representation on disk.
type whitelistFile struct {
	Rules map[string]string `yaml:"rules"`
}

// Load reads whitelist rules from disk.
func (w *WhitelistStore) Load() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := os.ReadFile(w.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read whitelist: %w", err)
	}

	var f whitelistFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("failed to parse whitelist: %w", err)
	}

	for k, v := range f.Rules {
		w.rules[k] = Decision(v)
	}

	w.logger.Debugf("Loaded %d whitelist rules from %s", len(f.Rules), w.filePath)
	return nil
}

// Save writes whitelist rules to disk.
func (w *WhitelistStore) Save() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	f := whitelistFile{
		Rules: make(map[string]string, len(w.rules)),
	}
	for k, v := range w.rules {
		f.Rules[k] = string(v)
	}

	data, err := yaml.Marshal(&f)
	if err != nil {
		return fmt.Errorf("failed to marshal whitelist: %w", err)
	}

	dir := filepath.Dir(w.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create whitelist directory: %w", err)
	}

	if err := os.WriteFile(w.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write whitelist: %w", err)
	}

	w.logger.Debugf("Saved %d whitelist rules to %s", len(w.rules), w.filePath)
	return nil
}

// LoadFromSpec pre-populates whitelist rules from a container spec.
func (w *WhitelistStore) LoadFromSpec(spec *model.ContainerSpec) {
	if spec.Permissions == nil {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, domain := range spec.Permissions.NetworkWhitelist {
		w.rules[ruleKey(ResourceNetwork, domain)] = DecisionAlways
	}
	for _, tool := range spec.Permissions.MCPWhitelist {
		w.rules[ruleKey(ResourceMCP, tool)] = DecisionAlways
	}
	for _, pkg := range spec.Permissions.PackageWhitelist {
		w.rules[ruleKey(ResourcePackage, pkg)] = DecisionAlways
	}

	w.logger.Debugf("Loaded %d rules from container spec",
		len(spec.Permissions.NetworkWhitelist)+len(spec.Permissions.MCPWhitelist)+len(spec.Permissions.PackageWhitelist))
}

// Rules returns a snapshot of all rules for inspection.
func (w *WhitelistStore) Rules() map[string]Decision {
	w.mu.RLock()
	defer w.mu.RUnlock()

	snapshot := make(map[string]Decision, len(w.rules))
	for k, v := range w.rules {
		snapshot[k] = v
	}
	return snapshot
}
