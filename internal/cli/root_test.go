package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tuipcli/tuip/internal/config"
	"github.com/tuipcli/tuip/internal/providers"
)

func TestDashboardCreateWithProviders(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	stdout, _, err := executeCommand("--config", configPath, "dashboard", "create", "work", "slack", "github-eu")
	if err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}
	if !strings.Contains(stdout, `created dashboard "work" with slack, github-enterprise-cloud-eu`) {
		t.Fatalf("stdout = %q", stdout)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DefaultDashboard != "work" {
		t.Fatalf("DefaultDashboard = %q, want work", cfg.DefaultDashboard)
	}
	dashboard, ok := cfg.GetDashboard("work")
	if !ok {
		t.Fatalf("work dashboard missing")
	}
	if got, want := dashboard.ProviderIDs(), []string{"slack", "github-enterprise-cloud-eu"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProviderIDs() = %#v, want %#v", got, want)
	}
}

func TestDashboardsAliasStillWorks(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	_, _, err := executeCommand("--config", configPath, "dashboards", "create", "work", "slack")
	if err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	dashboard, ok := cfg.GetDashboard("work")
	if !ok {
		t.Fatalf("work dashboard missing")
	}
	if got, want := dashboard.ProviderIDs(), []string{"slack"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProviderIDs() = %#v, want %#v", got, want)
	}
}

func TestDashboardCreateRejectsUnknownProvider(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	_, _, err := executeCommand("--config", configPath, "dashboard", "create", "work", "nope")
	if err == nil {
		t.Fatalf("executeCommand() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown provider(s): nope") {
		t.Fatalf("error = %q, want unknown provider", err.Error())
	}
	if _, statErr := os.Stat(configPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("config file exists after failed create; statErr = %v", statErr)
	}
}

func TestDashboardAddRemoveCommands(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if _, _, err := executeCommand("--config", configPath, "dashboard", "create", "work"); err != nil {
		t.Fatalf("create error = %v", err)
	}
	if _, _, err := executeCommand("--config", configPath, "dashboard", "add", "work", "slack", "github"); err != nil {
		t.Fatalf("add error = %v", err)
	}
	if _, _, err := executeCommand("--config", configPath, "dashboard", "remove", "work", "slack"); err != nil {
		t.Fatalf("remove error = %v", err)
	}

	stdout, _, err := executeCommand("--config", configPath, "dashboard", "show", "work")
	if err != nil {
		t.Fatalf("show error = %v", err)
	}
	if strings.Contains(stdout, "  - slack") {
		t.Fatalf("stdout contains removed provider slack: %q", stdout)
	}
	if !strings.Contains(stdout, "  - github") {
		t.Fatalf("stdout missing github: %q", stdout)
	}
}

func TestStatusDefaultAllDashboard(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.New()
	if err := cfg.SetDefaultDashboard(config.AllDashboard); err != nil {
		t.Fatalf("SetDefaultDashboard(all) error = %v", err)
	}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	providerIDs, err := resolveStatusProviderIDs(configPath, testRegistry(t), "", nil)
	if err != nil {
		t.Fatalf("resolveStatusProviderIDs() error = %v", err)
	}
	if len(providerIDs) == 0 {
		t.Fatalf("providerIDs is empty")
	}
	if !containsString(providerIDs, "slack") || !containsString(providerIDs, "cloudflare") {
		t.Fatalf("providerIDs = %#v, want built-in providers", providerIDs)
	}
}

func TestStatusRejectsExplicitProvidersAndDashboardTogether(t *testing.T) {
	t.Parallel()

	_, _, err := executeCommand("status", "--dashboard", "work", "slack")
	if err == nil {
		t.Fatalf("executeCommand() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "pass either explicit providers or --dashboard") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestProvidersSearchCommand(t *testing.T) {
	t.Parallel()

	stdout, _, err := executeCommand("providers", "search", "ghec", "eu")
	if err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}
	if !strings.Contains(stdout, "github-enterprise-cloud-eu") {
		t.Fatalf("stdout = %q, want GitHub Enterprise Cloud EU", stdout)
	}
	if strings.Contains(stdout, "slack") {
		t.Fatalf("stdout = %q, did not expect slack", stdout)
	}
}

func TestProvidersListRejectsArgs(t *testing.T) {
	t.Parallel()

	_, _, err := executeCommand("providers", "list", "extra")
	if err == nil {
		t.Fatalf("executeCommand() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("error = %q", err.Error())
	}
}

func testRegistry(t *testing.T) *providers.Registry {
	t.Helper()
	registry, err := newRegistry()
	if err != nil {
		t.Fatalf("newRegistry() error = %v", err)
	}
	return registry
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func executeCommand(args ...string) (string, string, error) {
	cmd := NewRootCommand()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}
