package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

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
