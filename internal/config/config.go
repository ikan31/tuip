package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	CurrentVersion = 1
	AllDashboard   = "all"

	configDirPerm  = 0o750
	configFilePerm = 0o600
)

// Config is tuip's shareable dashboard configuration file.
type Config struct {
	Version          int                  `json:"version"                     yaml:"version"`
	DefaultDashboard string               `json:"default_dashboard,omitempty" yaml:"default_dashboard,omitempty"`
	Dashboards       map[string]Dashboard `json:"dashboards"                  yaml:"dashboards"`
}

// Dashboard is a named collection of services.
type Dashboard struct {
	Services []Service `json:"services" yaml:"services"`
}

// Service references a built-in provider and leaves room for future per-service
// display/options.
type Service struct {
	Provider string `json:"provider" yaml:"provider"`
}

// New returns an empty config initialized with the current schema version.
func New() *Config {
	return &Config{
		Version:    CurrentVersion,
		Dashboards: map[string]Dashboard{},
	}
}

// DefaultPath returns the default user config file location.
//
// tuip is a terminal-first developer tool, so on Unix-like systems it follows
// the XDG-style config location instead of macOS's GUI-oriented
// ~/Library/Application Support path. Windows keeps the native OS config dir.
func DefaultPath() (string, error) {
	base, err := defaultConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(base, "tuip", "config.yaml"), nil
}

func defaultConfigDir() (string, error) {
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return xdgConfigHome, nil
	}

	if runtime.GOOS == "windows" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("get user config dir: %w", err)
		}

		return configDir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home dir: %w", err)
	}

	return filepath.Join(home, ".config"), nil
}

// ResolvePath returns overridePath if set, otherwise the default config path.
func ResolvePath(overridePath string) (string, error) {
	if overridePath != "" {
		return overridePath, nil
	}

	return DefaultPath()
}

// RuntimeDir returns the directory where tuip should write runtime files that
// belong with the configured config file, such as logs and status cache files.
func RuntimeDir(overridePath string) (string, error) {
	path, err := ResolvePath(overridePath)
	if err != nil {
		return "", err
	}

	return filepath.Dir(path), nil
}

// LogPath returns the structured diagnostics log path for the configured tuip
// runtime directory.
func LogPath(overridePath string) (string, error) {
	dir, err := RuntimeDir(overridePath)
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "logs", "tuip.jsonl"), nil
}

// StatusCachePath returns the persistent provider status cache path for the
// configured tuip runtime directory.
func StatusCachePath(overridePath string) (string, error) {
	dir, err := RuntimeDir(overridePath)
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "cache", "status-cache.json"), nil
}

// Load reads a config file from disk.
func Load(path string) (*Config, error) {
	// #nosec G304 -- path is the configured tuip config file path.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if len(data) == 0 {
		return New(), nil
	}

	var cfg Config

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
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

	err = os.MkdirAll(filepath.Dir(path), configDirPerm)
	if err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	err = os.WriteFile(path, data, configFilePerm)
	if err != nil {
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

	err := validateUserDashboardName(name)
	if err != nil {
		return err
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

// RenameDashboard renames a dashboard while preserving its services.
func (c *Config) RenameDashboard(oldName, newName string) error {
	c.normalize()

	if strings.TrimSpace(oldName) == "" {
		return errors.New("dashboard name is required")
	}

	err := validateUserDashboardName(newName)
	if err != nil {
		return err
	}

	dashboard, exists := c.Dashboards[oldName]
	if !exists {
		return fmt.Errorf("dashboard %q does not exist", oldName)
	}

	if _, exists := c.Dashboards[newName]; exists {
		return fmt.Errorf("dashboard %q already exists", newName)
	}

	delete(c.Dashboards, oldName)

	c.Dashboards[newName] = dashboard
	if c.DefaultDashboard == oldName {
		c.DefaultDashboard = newName
	}

	return nil
}

// DeleteDashboard removes a dashboard.
func (c *Config) DeleteDashboard(name string) error {
	c.normalize()

	if _, exists := c.Dashboards[name]; !exists {
		return fmt.Errorf("dashboard %q does not exist", name)
	}

	delete(c.Dashboards, name)

	if c.DefaultDashboard == name {
		c.DefaultDashboard = ""
		if names := c.DashboardNames(); len(names) > 0 {
			c.DefaultDashboard = names[0]
		}
	}

	return nil
}

// SetDefaultDashboard sets the configured default dashboard.
func (c *Config) SetDefaultDashboard(name string) error {
	c.normalize()

	if name == AllDashboard {
		c.DefaultDashboard = name

		return nil
	}

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

func validateUserDashboardName(name string) error {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return errors.New("dashboard name is required")
	}

	if normalized == AllDashboard {
		return fmt.Errorf("dashboard name %q is reserved", AllDashboard)
	}

	return nil
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
