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

func TestDashboardConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := New()
	if err := cfg.CreateDashboard("work"); err != nil {
		t.Fatalf("CreateDashboard() error = %v", err)
	}
	if err := cfg.AddProviders("work", []string{"slack", "github"}); err != nil {
		t.Fatalf("AddProviders() error = %v", err)
	}
	if err := cfg.SetDefaultDashboard("work"); err != nil {
		t.Fatalf("SetDefaultDashboard() error = %v", err)
	}
	if err := Save(path, cfg); err != nil {
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
	cfg := New()
	if err := cfg.CreateDashboard("work"); err != nil {
		t.Fatalf("CreateDashboard(work) error = %v", err)
	}
	if err := cfg.AddProviders("work", []string{"slack"}); err != nil {
		t.Fatalf("AddProviders() error = %v", err)
	}
	if err := cfg.RenameDashboard("work", "ops"); err != nil {
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

func TestSetDefaultDashboardAllowsAllVirtualDashboard(t *testing.T) {
	cfg := New()
	if err := cfg.SetDefaultDashboard(AllDashboard); err != nil {
		t.Fatalf("SetDefaultDashboard(all) error = %v", err)
	}
	if cfg.DefaultDashboard != AllDashboard {
		t.Fatalf("DefaultDashboard = %q, want all", cfg.DefaultDashboard)
	}
}

func TestDeleteDashboardSelectsNewDefault(t *testing.T) {
	cfg := New()
	if err := cfg.CreateDashboard("work"); err != nil {
		t.Fatalf("CreateDashboard(work) error = %v", err)
	}
	if err := cfg.CreateDashboard("ops"); err != nil {
		t.Fatalf("CreateDashboard(ops) error = %v", err)
	}
	if err := cfg.DeleteDashboard("work"); err != nil {
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
	cfg := New()
	if err := cfg.CreateDashboard("work"); err != nil {
		t.Fatalf("CreateDashboard() error = %v", err)
	}
	if err := cfg.AddProviders("work", []string{"slack", "slack", "github"}); err != nil {
		t.Fatalf("AddProviders() error = %v", err)
	}
	if err := cfg.RemoveProviders("work", []string{"slack"}); err != nil {
		t.Fatalf("RemoveProviders() error = %v", err)
	}
	dashboard, _ := cfg.GetDashboard("work")
	if got, want := dashboard.ProviderIDs(), []string{"github"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProviderIDs() = %#v, want %#v", got, want)
	}
}
