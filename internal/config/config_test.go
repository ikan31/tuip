package config

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestDefaultPathUsesXDGStyleConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	if runtime.GOOS == "windows" {
		if path == "" {
			t.Fatalf("DefaultPath() is empty")
		}

		return
	}

	want := filepath.Join(home, ".config", "tuip", "config.yaml")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestDefaultPathHonorsXDGConfigHome(t *testing.T) {
	xdgConfigHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)

	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath() error = %v", err)
	}

	want := filepath.Join(xdgConfigHome, "tuip", "config.yaml")
	if path != want {
		t.Fatalf("DefaultPath() = %q, want %q", path, want)
	}
}

func TestRuntimePathsLiveBesideConfig(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "nested", "config.yaml")

	logPath, err := LogPath(configPath)
	if err != nil {
		t.Fatalf("LogPath() error = %v", err)
	}

	wantLogPath := filepath.Join(filepath.Dir(configPath), "logs", "tuip.jsonl")
	if logPath != wantLogPath {
		t.Fatalf("LogPath() = %q, want %q", logPath, wantLogPath)
	}

	cachePath, err := StatusCachePath(configPath)
	if err != nil {
		t.Fatalf("StatusCachePath() error = %v", err)
	}

	wantCachePath := filepath.Join(filepath.Dir(configPath), "cache", "status-cache.json")
	if cachePath != wantCachePath {
		t.Fatalf("StatusCachePath() = %q, want %q", cachePath, wantCachePath)
	}
}

func TestDashboardConfigRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := New()

	err := cfg.CreateDashboard("work")
	if err != nil {
		t.Fatalf("CreateDashboard() error = %v", err)
	}

	err = cfg.AddProviders("work", []string{"slack", "github"})
	if err != nil {
		t.Fatalf("AddProviders() error = %v", err)
	}

	err = cfg.SetDefaultDashboard("work")
	if err != nil {
		t.Fatalf("SetDefaultDashboard() error = %v", err)
	}

	err = Save(path, cfg)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.DefaultDashboard != "work" {
		t.Fatalf("DefaultDashboard = %q", loaded.DefaultDashboard)
	}

	dashboard, ok := loaded.GetDashboard("work")
	if !ok {
		t.Fatalf("work dashboard missing")
	}

	if got, want := dashboard.ProviderIDs(), []string{"slack", "github"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProviderIDs() = %#v, want %#v", got, want)
	}
}

func TestRenameDashboardUpdatesDefault(t *testing.T) {
	t.Parallel()

	cfg := New()

	err := cfg.CreateDashboard("work")
	if err != nil {
		t.Fatalf("CreateDashboard(work) error = %v", err)
	}

	err = cfg.AddProviders("work", []string{"slack"})
	if err != nil {
		t.Fatalf("AddProviders() error = %v", err)
	}

	err = cfg.RenameDashboard("work", "ops")
	if err != nil {
		t.Fatalf("RenameDashboard() error = %v", err)
	}

	if cfg.DefaultDashboard != "ops" {
		t.Fatalf("DefaultDashboard = %q, want ops", cfg.DefaultDashboard)
	}

	if _, ok := cfg.GetDashboard("work"); ok {
		t.Fatalf("old dashboard still exists")
	}

	dashboard, ok := cfg.GetDashboard("ops")
	if !ok {
		t.Fatalf("renamed dashboard missing")
	}

	if got, want := dashboard.ProviderIDs(), []string{"slack"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProviderIDs() = %#v, want %#v", got, want)
	}
}

func TestDashboardNamesRejectReservedAll(t *testing.T) {
	t.Parallel()

	cfg := New()

	err := cfg.CreateDashboard(AllDashboard)
	if err == nil {
		t.Fatalf("CreateDashboard(all) error = nil, want reserved-name error")
	}

	err = cfg.CreateDashboard("work")
	if err != nil {
		t.Fatalf("CreateDashboard(work) error = %v", err)
	}

	err = cfg.RenameDashboard("work", "ALL")
	if err == nil {
		t.Fatalf("RenameDashboard(work, ALL) error = nil, want reserved-name error")
	}
}

func TestSetDefaultDashboardAllowsAllVirtualDashboard(t *testing.T) {
	t.Parallel()

	cfg := New()

	err := cfg.SetDefaultDashboard(AllDashboard)
	if err != nil {
		t.Fatalf("SetDefaultDashboard(all) error = %v", err)
	}

	if cfg.DefaultDashboard != AllDashboard {
		t.Fatalf("DefaultDashboard = %q, want all", cfg.DefaultDashboard)
	}
}

func TestDeleteDashboardSelectsNewDefault(t *testing.T) {
	t.Parallel()

	cfg := New()

	err := cfg.CreateDashboard("work")
	if err != nil {
		t.Fatalf("CreateDashboard(work) error = %v", err)
	}

	err = cfg.CreateDashboard("ops")
	if err != nil {
		t.Fatalf("CreateDashboard(ops) error = %v", err)
	}

	err = cfg.DeleteDashboard("work")
	if err != nil {
		t.Fatalf("DeleteDashboard() error = %v", err)
	}

	if _, ok := cfg.GetDashboard("work"); ok {
		t.Fatalf("deleted dashboard still exists")
	}

	if cfg.DefaultDashboard != "ops" {
		t.Fatalf("DefaultDashboard = %q, want ops", cfg.DefaultDashboard)
	}
}

func TestAddProvidersIgnoresDuplicatesAndRemoveProviders(t *testing.T) {
	t.Parallel()

	cfg := New()

	err := cfg.CreateDashboard("work")
	if err != nil {
		t.Fatalf("CreateDashboard() error = %v", err)
	}

	err = cfg.AddProviders("work", []string{"slack", "slack", "github"})
	if err != nil {
		t.Fatalf("AddProviders() error = %v", err)
	}

	err = cfg.RemoveProviders("work", []string{"slack"})
	if err != nil {
		t.Fatalf("RemoveProviders() error = %v", err)
	}

	dashboard, _ := cfg.GetDashboard("work")
	if got, want := dashboard.ProviderIDs(), []string{"github"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProviderIDs() = %#v, want %#v", got, want)
	}
}
