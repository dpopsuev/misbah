package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	DefaultDaemonConfigPath = "/etc/misbah/daemon.yaml"
	DefaultSocketGroup      = "misbah"
)

// DaemonConfig is the daemon configuration loaded from /etc/misbah/daemon.yaml.
type DaemonConfig struct {
	Daemon      DaemonSection      `yaml:"daemon"`
	Kata        KataSection        `yaml:"kata"`
	Permissions PermissionsSection `yaml:"permissions"`
}

// DaemonSection configures the daemon process itself.
type DaemonSection struct {
	Socket         string `yaml:"socket"`
	Group          string `yaml:"group"`
	LogLevel       string `yaml:"log_level"`
	NonInteractive bool   `yaml:"non_interactive"`
}

// KataSection configures the Kata/CRI backend.
type KataSection struct {
	Endpoint    string            `yaml:"endpoint"`
	Handler     string            `yaml:"handler"`
	Annotations map[string]string `yaml:"annotations"`
}

// PermissionsSection configures the permission broker.
type PermissionsSection struct {
	Whitelist string `yaml:"whitelist"`
	AuditLog  string `yaml:"audit_log"`
}

// LoadDaemonConfig loads the daemon config from disk, applies defaults, then env overrides.
func LoadDaemonConfig(path string) (*DaemonConfig, error) {
	cfg := DefaultDaemonConfig()

	if path == "" {
		path = DefaultDaemonConfigPath
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file — use defaults + env overrides
			applyDaemonEnvOverrides(cfg)
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read daemon config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse daemon config: %w", err)
	}

	applyDaemonEnvOverrides(cfg)
	return cfg, nil
}

// DefaultDaemonConfig returns the built-in defaults.
func DefaultDaemonConfig() *DaemonConfig {
	return &DaemonConfig{
		Daemon: DaemonSection{
			Socket:   DefaultDaemonSocket,
			Group:    DefaultSocketGroup,
			LogLevel: "info",
		},
		Kata: KataSection{
			Endpoint: DefaultCRIEndpoint,
			Handler:  DefaultRuntimeHandler,
			Annotations: map[string]string{
				"io.katacontainers.config.runtime.internetworking_model": "none",
				"io.katacontainers.config.runtime.disable_new_netns":    "true",
			},
		},
		Permissions: PermissionsSection{
			Whitelist: GetWhitelistPath(),
			AuditLog:  GetAuditLogPath(),
		},
	}
}

// applyDaemonEnvOverrides applies environment variable overrides (highest priority).
func applyDaemonEnvOverrides(cfg *DaemonConfig) {
	if v := os.Getenv(EnvDaemonSocket); v != "" {
		cfg.Daemon.Socket = v
	}
	if v := os.Getenv(EnvCRIEndpoint); v != "" {
		cfg.Kata.Endpoint = v
	}
	if v := os.Getenv(EnvRuntimeHandler); v != "" {
		cfg.Kata.Handler = v
	}
}
