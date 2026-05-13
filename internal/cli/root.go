package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tuipcli/tuip/internal/app"
	"github.com/tuipcli/tuip/internal/config"
	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/output"
	"github.com/tuipcli/tuip/internal/providers"
	"github.com/tuipcli/tuip/internal/providers/builtin"
)

// NewRootCommand builds the tuip CLI command tree.
func NewRootCommand() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:           "tuip",
		Short:         "Terminal SaaS status dashboards",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "config file path (defaults to OS user config dir)")

	root.AddCommand(newStatusCommand(&configPath))
	root.AddCommand(newProvidersCommand())
	root.AddCommand(newDashboardsCommand(&configPath))

	return root
}

func newStatusCommand(configPath *string) *cobra.Command {
	var jsonOutput bool
	var details bool
	var dashboardName string
	var failOnDegraded bool

	cmd := &cobra.Command{
		Use:   "status [provider...]",
		Short: "Fetch provider statuses",
		Long: strings.TrimSpace(`Fetch provider statuses.

Examples:
  tuip status slack github cloudflare
  tuip status --details slack
  tuip status --json slack github
  tuip status                 # later/config mode: default dashboard
  tuip status --dashboard work`),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := newRegistry()
			if err != nil {
				return err
			}

			providerIDs, err := resolveStatusProviderIDs(*configPath, dashboardName, args)
			if err != nil {
				return err
			}

			response, checkErr := app.CheckProviders(context.Background(), registry, providerIDs, app.StatusOptions{Details: details})
			if jsonOutput {
				if err := output.WriteJSON(cmd.OutOrStdout(), response); err != nil {
					return err
				}
			} else {
				output.WriteHuman(cmd.OutOrStdout(), response, details)
			}
			if checkErr != nil {
				return checkErr
			}
			if failOnDegraded && app.HasUnhealthyProvider(response) {
				return fmt.Errorf("one or more providers are not operational")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "write standardized JSON output")
	cmd.Flags().BoolVar(&details, "details", false, "include active incidents, scheduled maintenance, and components")
	cmd.Flags().StringVar(&dashboardName, "dashboard", "", "dashboard name from config")
	cmd.Flags().BoolVar(&failOnDegraded, "fail-on-degraded", false, "exit non-zero when any checked provider is not operational")
	return cmd
}

func newProvidersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "Manage built-in providers",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List built-in providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := newRegistry()
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "ID\tNAME\tSOURCE\tAPI")
			for _, metadata := range registry.Metadata() {
				fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", metadata.ID, metadata.Name, metadata.SourceURL, metadata.APIURL)
			}
			return writer.Flush()
		},
	})
	return cmd
}

func newDashboardsCommand(configPath *string) *cobra.Command {
	dashboards := &cobra.Command{
		Use:   "dashboards",
		Short: "Manage YAML dashboards",
	}

	dashboards.AddCommand(&cobra.Command{
		Use:   "create <name>",
		Short: "Create a dashboard",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadOrNewConfig(*configPath)
			if err != nil {
				return err
			}
			name := args[0]
			if err := cfg.CreateDashboard(name); err != nil {
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created dashboard %q\n", name)
			return nil
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List dashboards",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadOrNewConfig(*configPath)
			if err != nil {
				return err
			}
			names := cfg.DashboardNames()
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no dashboards configured")
				return nil
			}
			for _, name := range names {
				marker := " "
				if name == cfg.DefaultDashboard {
					marker = "*"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", marker, name)
			}
			return nil
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:   "show <name>",
		Short: "Show dashboard providers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadConfig(*configPath)
			if err != nil {
				return err
			}
			name := args[0]
			dashboard, ok := cfg.GetDashboard(name)
			if !ok {
				return fmt.Errorf("dashboard %q does not exist", name)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "dashboard: %s\n", name)
			if cfg.DefaultDashboard == name {
				fmt.Fprintln(cmd.OutOrStdout(), "default:   true")
			}
			providerIDs := dashboard.ProviderIDs()
			if len(providerIDs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "services:  none")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "services:")
			for _, providerID := range providerIDs {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", providerID)
			}
			return nil
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:   "use <name>",
		Short: "Set the default dashboard",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig(*configPath)
			if err != nil {
				return err
			}
			name := args[0]
			if err := cfg.SetDefaultDashboard(name); err != nil {
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "default dashboard set to %q\n", name)
			return nil
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:   "add <name> <provider...>",
		Short: "Add providers to a dashboard",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig(*configPath)
			if err != nil {
				return err
			}
			registry, err := newRegistry()
			if err != nil {
				return err
			}
			name := args[0]
			providerIDs := normalizeProviderIDs(args[1:])
			if err := registry.ValidateIDs(providerIDs); err != nil {
				return err
			}
			if err := cfg.AddProviders(name, providerIDs); err != nil {
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s to dashboard %q\n", strings.Join(providerIDs, ", "), name)
			return nil
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:   "remove <name> <provider...>",
		Short: "Remove providers from a dashboard",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadConfig(*configPath)
			if err != nil {
				return err
			}
			registry, err := newRegistry()
			if err != nil {
				return err
			}
			name := args[0]
			providerIDs := normalizeProviderIDs(args[1:])
			if err := registry.ValidateIDs(providerIDs); err != nil {
				return err
			}
			if err := cfg.RemoveProviders(name, providerIDs); err != nil {
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s from dashboard %q\n", strings.Join(providerIDs, ", "), name)
			return nil
		},
	})

	return dashboards
}

func resolveStatusProviderIDs(configPath, dashboardName string, args []string) ([]string, error) {
	if len(args) > 0 && dashboardName != "" {
		return nil, fmt.Errorf("pass either explicit providers or --dashboard, not both")
	}
	if len(args) > 0 {
		return normalizeProviderIDs(args), nil
	}

	_, cfg, err := loadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no config found; pass providers explicitly, e.g. `tuip status slack github`, or create a dashboard")
		}
		return nil, err
	}

	name := dashboardName
	if name == "" {
		name = cfg.DefaultDashboard
	}
	if name == "" {
		return nil, fmt.Errorf("no default dashboard configured; pass providers explicitly or run `tuip dashboards use <name>`")
	}
	dashboard, ok := cfg.GetDashboard(name)
	if !ok {
		return nil, fmt.Errorf("dashboard %q does not exist", name)
	}
	providerIDs := dashboard.ProviderIDs()
	if len(providerIDs) == 0 {
		return nil, fmt.Errorf("dashboard %q has no providers", name)
	}
	return normalizeProviderIDs(providerIDs), nil
}

func normalizeProviderIDs(ids []string) []string {
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id != "" {
			normalized = append(normalized, id)
		}
	}
	return normalized
}

func loadConfig(pathOverride string) (string, *config.Config, error) {
	path, err := config.ResolvePath(pathOverride)
	if err != nil {
		return "", nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return path, nil, err
	}
	return path, cfg, nil
}

func loadOrNewConfig(pathOverride string) (string, *config.Config, error) {
	path, err := config.ResolvePath(pathOverride)
	if err != nil {
		return "", nil, err
	}
	cfg, err := config.LoadOrNew(path)
	if err != nil {
		return path, nil, err
	}
	return path, cfg, nil
}

func newRegistry() (*providers.Registry, error) {
	client := fetch.NewClient(5 * time.Second)
	return builtin.NewRegistry(client)
}
