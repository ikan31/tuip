package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const CurrentVersion = 1

// Config is tuip's shareable dashboard configuration file.
type Config struct {
	Version          int                  `yaml:"version" json:"version"`
	DefaultDashboard string               `yaml:"default_dashboard,omitempty" json:"default_dashboard,omitempty"`
	Dashboards       map[string]Dashboard `yaml:"dashboards" json:"dashboards"`
}

// Dashboard is a named collection of services.
type Dashboard struct {
	Services []Service `yaml:"services" json:"services"`
}

// Service references a built-in provider and leaves room for future per-service
// display/options.
type Service struct {
	Provider string `yaml:"provider" json:"provider"`
}

// New returns an empty config initialized with the current schema version.
func New() *Config {
	return &Config{
		Version:    CurrentVersion,
		Dashboards: map[string]Dashboard{},
	}
}

// DefaultPath returns the default user config file location.
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "tuip", "config.yaml"), nil
}

// ResolvePath returns overridePath if set, otherwise the default config path.
func ResolvePath(overridePath string) (string, error) {
	if overridePath != "" {
		return overridePath, nil
	}
	return DefaultPath()
}

// Load reads a config file from disk.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return New(), nil
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.normalize()
	return &cfg, nil
}

// LoadOrNew reads a config if it exists or returns a new empty config.
func LoadOrNew(path string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return New(), nil
		}
		return nil, err
	}
	return cfg, nil
}

// Save writes a config to disk, creating parent directories as needed.
func Save(path string, cfg *Config) error {
	cfg.normalize()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// DashboardNames returns dashboard names sorted alphabetically.
func (c *Config) DashboardNames() []string {
	c.normalize()
	names := make([]string, 0, len(c.Dashboards))
	for name := range c.Dashboards {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// CreateDashboard adds an empty dashboard.
func (c *Config) CreateDashboard(name string) error {
	c.normalize()
	if name == "" {
		return fmt.Errorf("dashboard name is required")
	}
	if _, exists := c.Dashboards[name]; exists {
		return fmt.Errorf("dashboard %q already exists", name)
	}
	c.Dashboards[name] = Dashboard{Services: []Service{}}
	if c.DefaultDashboard == "" {
		c.DefaultDashboard = name
	}
	return nil
}

// SetDefaultDashboard sets the configured default dashboard.
func (c *Config) SetDefaultDashboard(name string) error {
	c.normalize()
	if _, exists := c.Dashboards[name]; !exists {
		return fmt.Errorf("dashboard %q does not exist", name)
	}
	c.DefaultDashboard = name
	return nil
}

// GetDashboard returns a dashboard by name.
func (c *Config) GetDashboard(name string) (Dashboard, bool) {
	c.normalize()
	dashboard, ok := c.Dashboards[name]
	return dashboard, ok
}

// AddProviders adds provider IDs to a dashboard, ignoring duplicates.
func (c *Config) AddProviders(name string, providerIDs []string) error {
	c.normalize()
	dashboard, exists := c.Dashboards[name]
	if !exists {
		return fmt.Errorf("dashboard %q does not exist", name)
	}
	existing := map[string]bool{}
	for _, service := range dashboard.Services {
		existing[service.Provider] = true
	}
	for _, providerID := range providerIDs {
		if !existing[providerID] {
			dashboard.Services = append(dashboard.Services, Service{Provider: providerID})
			existing[providerID] = true
		}
	}
	c.Dashboards[name] = dashboard
	return nil
}

// RemoveProviders removes provider IDs from a dashboard.
func (c *Config) RemoveProviders(name string, providerIDs []string) error {
	c.normalize()
	dashboard, exists := c.Dashboards[name]
	if !exists {
		return fmt.Errorf("dashboard %q does not exist", name)
	}
	remove := map[string]bool{}
	for _, providerID := range providerIDs {
		remove[providerID] = true
	}
	services := make([]Service, 0, len(dashboard.Services))
	for _, service := range dashboard.Services {
		if !remove[service.Provider] {
			services = append(services, service)
		}
	}
	dashboard.Services = services
	c.Dashboards[name] = dashboard
	return nil
}

// ProviderIDs returns the provider IDs in a dashboard in configured order.
func (d Dashboard) ProviderIDs() []string {
	ids := make([]string, 0, len(d.Services))
	for _, service := range d.Services {
		ids = append(ids, service.Provider)
	}
	return ids
}

func (c *Config) normalize() {
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	if c.Dashboards == nil {
		c.Dashboards = map[string]Dashboard{}
	}
	for name, dashboard := range c.Dashboards {
		if dashboard.Services == nil {
			dashboard.Services = []Service{}
			c.Dashboards[name] = dashboard
		}
	}
}
