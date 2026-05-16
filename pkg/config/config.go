// Package config defines the YAML configuration types for gopm and provides
// helpers for loading them from disk.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// WatchConfig describes the optional file-watching dev-mode behaviour for an
// app. When Enabled is true, the supervisor will rebuild and restart the
// process whenever a watched file changes.
type WatchConfig struct {
	Enabled    bool     `yaml:"enabled"`
	Dirs       []string `yaml:"dirs"`
	Extensions []string `yaml:"extensions"`
	Ignore     []string `yaml:"ignore"`
	BuildCmd   string   `yaml:"build_cmd"`
	DebounceMs int      `yaml:"debounce_ms"`
}

// HealthCheckConfig describes an optional HTTP health check for a process.
type HealthCheckConfig struct {
	URL      string `yaml:"url"`
	Interval string `yaml:"interval"` // default "5s"
	Timeout  string `yaml:"timeout"`  // default "2s"
	Retries  int    `yaml:"retries"`  // default 3 — consecutive failures before unhealthy
}

// OnCrashConfig describes optional webhook notification when a process crashes.
type OnCrashConfig struct {
	Webhook string `yaml:"webhook"`
}

// AppConfig is the user-facing definition of a single managed process.
type AppConfig struct {
	Name        string            `yaml:"name"`
	Command     string            `yaml:"command"`
	Args        []string          `yaml:"args"`
	Cwd         string            `yaml:"cwd"`
	Env         map[string]string `yaml:"env"`
	EnvFile     string            `yaml:"env_file"`
	AutoRestart bool              `yaml:"auto_restart"`
	MaxRestarts int               `yaml:"max_restarts"`
	LogFile     string            `yaml:"log_file"`
	Watch       WatchConfig       `yaml:"watch"`
	PreStart    string            `yaml:"pre_start"`
	DependsOn   []string          `yaml:"depends_on"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	OnCrash     OnCrashConfig     `yaml:"on_crash"`
	Groups      []string          `yaml:"groups"`
	Port        int               `yaml:"port"`
	// Log rotation
	LogMaxSizeMB  int `yaml:"log_max_size_mb"`
	LogMaxBackups int `yaml:"log_max_backups"`
}

// GopmConfig is the top-level config file structure for gopm.yaml.
type GopmConfig struct {
	Apps []AppConfig `yaml:"apps"`
}

// Load reads and parses a gopm.yaml file from the given path. Relative
// paths in the config (Cwd, LogFile, Watch.Dirs) are left untouched; callers
// should resolve them relative to the config file's directory if needed.
func Load(path string) (*GopmConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg GopmConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate performs basic sanity checks on the parsed configuration.
func (c *GopmConfig) Validate() error {
	if len(c.Apps) == 0 {
		return fmt.Errorf("config has no apps defined")
	}
	seen := make(map[string]bool, len(c.Apps))
	for i := range c.Apps {
		a := &c.Apps[i]
		if a.Name == "" {
			return fmt.Errorf("app[%d]: name is required", i)
		}
		if seen[a.Name] {
			return fmt.Errorf("duplicate app name %q", a.Name)
		}
		seen[a.Name] = true
		if a.Command == "" {
			return fmt.Errorf("app %q: command is required", a.Name)
		}
	}
	return nil
}

// ResolvePath joins a possibly-relative path from the config with the
// directory of the config file itself, so that `cwd: ./services/foo` is
// interpreted relative to the config rather than the daemon's cwd.
func ResolvePath(configPath, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(filepath.Dir(configPath), p)
}
