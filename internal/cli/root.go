package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/tuipcli/tuip/internal/tui"
)

const version = "0.1.0"

// NewRootCommand builds the tuip CLI command tree.
func NewRootCommand() *cobra.Command {
	var configPath string

	root := &cobra.Command{
		Use:   "tuip",
		Short: "Terminal SaaS status dashboards",
		Long: strings.TrimSpace(`tuip checks SaaS service status from your terminal.

Use explicit providers for quick checks, or configure YAML dashboards for reusable groups of services.`),
		Example: strings.TrimSpace(`tuip status slack github cloudflare
tuip status --json slack github
tuip dashboard create work
tuip dashboard add work slack github cloudflare`),
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.Run(cmd.Context(), configPath)
		},
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
		Long: strings.TrimSpace(`Fetch SaaS provider statuses.

Pass provider IDs directly for an ad-hoc check. With no provider IDs, tuip checks the configured default dashboard. Use --dashboard to check a named dashboard.`),
		Example: strings.TrimSpace(`tuip status slack github cloudflare
tuip status --details cloudflare
tuip status --json slack github
tuip status
tuip status --dashboard work`),
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: providerCompletionFunc,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := newRegistry()
			if err != nil {
				return err
			}

			providerIDs, err := resolveStatusProviderIDs(*configPath, registry, dashboardName, args)
			if err != nil {
				return err
			}

			response, checkErr := app.CheckProviders(context.Background(), registry, providerIDs, app.StatusOptions{Details: details})
			if jsonOutput {
				if err := output.WriteJSON(cmd.OutOrStdout(), response); err != nil {
					return err
				}
			} else {
				if err := output.WriteHuman(cmd.OutOrStdout(), response, details); err != nil {
					return err
				}
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
		Long:  "List and inspect the built-in SaaS status providers that tuip can check.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "List built-in providers",
		Example: "tuip providers list",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := newRegistry()
			if err != nil {
				return err
			}
			return writeProviderMetadataTable(cmd.OutOrStdout(), registry.Metadata())
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:     "search <query>",
		Aliases: []string{"find"},
		Short:   "Fuzzy-search built-in providers",
		Example: "tuip providers search github eu",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			registry, err := newRegistry()
			if err != nil {
				return err
			}
			matches := registry.Search(strings.Join(args, " "))
			if len(matches) == 0 {
				return writeln(cmd.OutOrStdout(), "no providers matched")
			}
			return writeProviderMetadataTable(cmd.OutOrStdout(), matches)
		},
	})
	return cmd
}

func newDashboardsCommand(configPath *string) *cobra.Command {
	dashboards := &cobra.Command{
		Use:     "dashboard",
		Aliases: []string{"dashboards"},
		Short:   "Manage YAML dashboards",
		Long: strings.TrimSpace(`Manage shareable YAML dashboards.

Dashboards are named collections of provider IDs. The default dashboard is used when running tuip status without explicit providers.`),
		Example: strings.TrimSpace(`tuip dashboard create work slack github cloudflare
tuip dashboard add work cloudflare
tuip dashboard use work
tuip status`),
	}

	dashboards.AddCommand(&cobra.Command{
		Use:               "create <name> [provider...]",
		Short:             "Create a dashboard",
		Example:           "tuip dashboard create work slack github cloudflare",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: createDashboardCompletionFunc,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, cfg, err := loadOrNewConfig(*configPath)
			if err != nil {
				return err
			}

			name := args[0]
			providerIDs := normalizeProviderIDs(args[1:])
			if len(providerIDs) > 0 {
				registry, err := newRegistry()
				if err != nil {
					return err
				}
				providerIDs, err = registry.CanonicalIDs(providerIDs)
				if err != nil {
					return err
				}
			}

			if err := cfg.CreateDashboard(name); err != nil {
				return err
			}
			if len(providerIDs) > 0 {
				if err := cfg.AddProviders(name, providerIDs); err != nil {
					return err
				}
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			if len(providerIDs) == 0 {
				return writef(cmd.OutOrStdout(), "created dashboard %q\n", name)
			}
			return writef(cmd.OutOrStdout(), "created dashboard %q with %s\n", name, strings.Join(providerIDs, ", "))
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:     "list",
		Short:   "List dashboards",
		Example: "tuip dashboard list",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, cfg, err := loadOrNewConfig(*configPath)
			if err != nil {
				return err
			}
			names := cfg.DashboardNames()
			if len(names) == 0 {
				return writeln(cmd.OutOrStdout(), "no dashboards configured")
			}
			for _, name := range names {
				marker := " "
				if name == cfg.DefaultDashboard {
					marker = "*"
				}
				if err := writef(cmd.OutOrStdout(), "%s %s\n", marker, name); err != nil {
					return err
				}
			}
			return nil
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:               "show <name>",
		Short:             "Show dashboard providers",
		Example:           "tuip dashboard show work",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: dashboardCompletionFunc(configPath),
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
			if err := writef(cmd.OutOrStdout(), "dashboard: %s\n", name); err != nil {
				return err
			}
			if cfg.DefaultDashboard == name {
				if err := writeln(cmd.OutOrStdout(), "default:   true"); err != nil {
					return err
				}
			}
			providerIDs := dashboard.ProviderIDs()
			if len(providerIDs) == 0 {
				return writeln(cmd.OutOrStdout(), "services:  none")
			}
			if err := writeln(cmd.OutOrStdout(), "services:"); err != nil {
				return err
			}
			for _, providerID := range providerIDs {
				if err := writef(cmd.OutOrStdout(), "  - %s\n", providerID); err != nil {
					return err
				}
			}
			return nil
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:               "use <name>",
		Short:             "Set the default dashboard",
		Example:           "tuip dashboard use work",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: dashboardCompletionFunc(configPath),
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
			return writef(cmd.OutOrStdout(), "default dashboard set to %q\n", name)
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:               "add <name> <provider...>",
		Short:             "Add providers to a dashboard",
		Example:           "tuip dashboard add work slack github cloudflare",
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: dashboardProviderCompletionFunc(configPath),
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
			providerIDs, err = registry.CanonicalIDs(providerIDs)
			if err != nil {
				return err
			}
			if err := cfg.AddProviders(name, providerIDs); err != nil {
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			return writef(cmd.OutOrStdout(), "added %s to dashboard %q\n", strings.Join(providerIDs, ", "), name)
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:               "remove <name> <provider...>",
		Aliases:           []string{"rm"},
		Short:             "Remove providers from a dashboard",
		Example:           "tuip dashboard remove work github",
		Args:              cobra.MinimumNArgs(2),
		ValidArgsFunction: dashboardProviderCompletionFunc(configPath),
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
			providerIDs, err = registry.CanonicalIDs(providerIDs)
			if err != nil {
				return err
			}
			if err := cfg.RemoveProviders(name, providerIDs); err != nil {
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			return writef(cmd.OutOrStdout(), "removed %s from dashboard %q\n", strings.Join(providerIDs, ", "), name)
		},
	})

	return dashboards
}

func resolveStatusProviderIDs(configPath string, registry *providers.Registry, dashboardName string, args []string) ([]string, error) {
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
		return nil, fmt.Errorf("no default dashboard configured; pass providers explicitly or run `tuip dashboard use <name>`")
	}
	if name == config.AllDashboard {
		return allProviderIDs(registry), nil
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

func allProviderIDs(registry *providers.Registry) []string {
	metadata := registry.Metadata()
	ids := make([]string, 0, len(metadata))
	for _, item := range metadata {
		ids = append(ids, item.ID)
	}
	return ids
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

func providerCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return providerCompletions(args, toComplete)
}

func dashboardCompletionFunc(configPath *string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		path, err := config.ResolvePath(*configPath)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		cfg, err := config.LoadOrNew(path)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return stringCompletions(cfg.DashboardNames(), args, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func dashboardProviderCompletionFunc(configPath *string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return dashboardCompletionFunc(configPath)(cmd, args, toComplete)
		}
		return providerCompletions(args[1:], toComplete)
	}
}

func createDashboardCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return providerCompletions(args[1:], toComplete)
}

func providerCompletions(args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	registry, err := newRegistry()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	used := map[string]bool{}
	for _, arg := range args {
		arg = strings.ToLower(strings.TrimSpace(arg))
		used[arg] = true
		if canonicalID, ok := registry.CanonicalID(arg); ok {
			used[canonicalID] = true
		}
	}

	matches := make([]string, 0)
	toComplete = strings.ToLower(toComplete)
	for _, metadata := range registry.Metadata() {
		if !used[metadata.ID] && strings.HasPrefix(metadata.ID, toComplete) {
			matches = append(matches, metadata.ID+"\t"+metadata.Description)
		}
		for _, alias := range metadata.Aliases {
			if used[alias] || used[metadata.ID] || !strings.HasPrefix(alias, toComplete) {
				continue
			}
			matches = append(matches, alias+"\tAlias for "+metadata.ID)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

func stringCompletions(values []string, args []string, toComplete string) []string {
	used := map[string]bool{}
	for _, arg := range args {
		used[arg] = true
	}

	matches := make([]string, 0, len(values))
	for _, value := range values {
		if used[value] || !strings.HasPrefix(value, toComplete) {
			continue
		}
		matches = append(matches, value)
	}
	return matches
}

func writeProviderMetadataTable(w io.Writer, metadata []providers.Metadata) error {
	writer := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if err := writeln(writer, "ID\tALIASES\tCATEGORY\tNAME\tSOURCE\tAPI"); err != nil {
		return err
	}
	for _, item := range metadata {
		aliases := strings.Join(item.Aliases, ", ")
		if err := writef(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", item.ID, aliases, item.Category, item.Name, item.SourceURL, item.APIURL); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func writeln(w io.Writer, args ...any) error {
	_, err := fmt.Fprintln(w, args...)
	return err
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}
