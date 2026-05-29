package cli

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tuipcli/tuip/internal/app"
	"github.com/tuipcli/tuip/internal/config"
	"github.com/tuipcli/tuip/internal/diagnostics"
	"github.com/tuipcli/tuip/internal/fetch"
	"github.com/tuipcli/tuip/internal/output"
	"github.com/tuipcli/tuip/internal/providers"
	"github.com/tuipcli/tuip/internal/providers/builtin"
	"github.com/tuipcli/tuip/internal/tui"
)

const (
	version           = "0.1.0"
	providersArgCount = 2
	registryTimeout   = 5 * time.Second
	tablePadding      = 2
)

// NewRootCommand builds the tuip CLI command tree.
func NewRootCommand() *cobra.Command {
	var (
		configPath string
		logLevel   string
	)

	root := &cobra.Command{
		Use:   "tuip",
		Short: "Terminal SaaS status dashboards",
		Long: strings.TrimSpace(`tuip checks SaaS service status from your terminal.

Use explicit providers for quick checks, configure YAML dashboards for reusable groups of services, or run the interactive TUI with no subcommand.`),
		Example:       strings.TrimSpace("tuip status slack github cloudflare\ntuip status --json github jira asana\ntuip providers search github eu\ntuip dashboard create work slack github jira asana\nTUIP_LOG_LEVEL=debug tuip"),
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, closeLogger, err := newLogger(configPath, logLevel)
			if err != nil {
				return err
			}
			defer closeLogger()

			return tui.Run(cmd.Context(), configPath, logger)
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "config file path (defaults to OS user config dir)")
	root.PersistentFlags().StringVar(&logLevel, "log-level", "", "diagnostics log level: off, debug, info, warn, or error (defaults to TUIP_LOG_LEVEL or off)")

	root.AddCommand(newStatusCommand(&configPath, &logLevel))
	root.AddCommand(newProvidersCommand())
	root.AddCommand(newDashboardsCommand(&configPath))

	return root
}

func newStatusCommand(configPath *string, logLevel *string) *cobra.Command {
	var (
		jsonOutput     bool
		details        bool
		dashboardName  string
		failOnDegraded bool
	)

	cmd := &cobra.Command{
		Use:   "status [provider...]",
		Short: "Fetch provider statuses",
		Long: strings.TrimSpace(`Fetch SaaS provider statuses.

Pass provider IDs directly for an ad-hoc check. With no provider IDs, tuip checks the configured default dashboard. Use --dashboard to check a named dashboard.`),
		Example: strings.TrimSpace(`tuip status slack github cloudflare
tuip status --details cloudflare
tuip status --json github jira asana
tuip status --fail-on-degraded github cloudflare
tuip status
tuip status --dashboard work`),
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: providerCompletionFunc,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, closeLogger, err := newLogger(*configPath, *logLevel)
			if err != nil {
				return err
			}
			defer closeLogger()

			registry, err := newRegistry()
			if err != nil {
				return fmt.Errorf("initialize provider registry: %w", err)
			}

			providerIDs, err := resolveStatusProviderIDs(*configPath, registry, dashboardName, args)
			if err != nil {
				return fmt.Errorf("resolve status providers: %w", err)
			}

			response, checkErr := app.CheckProviders(cmd.Context(), registry, providerIDs, app.StatusOptions{Details: details, Logger: logger})
			if jsonOutput {
				err := output.WriteJSON(cmd.OutOrStdout(), response)
				if err != nil {
					return fmt.Errorf("write JSON status: %w", err)
				}
			} else {
				err := output.WriteHuman(cmd.OutOrStdout(), response, details)
				if err != nil {
					return fmt.Errorf("write human status: %w", err)
				}
			}

			if checkErr != nil {
				return fmt.Errorf("check providers: %w", checkErr)
			}

			if failOnDegraded && app.HasUnhealthyProvider(response) {
				return errors.New("one or more providers are not operational")
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
		Example: strings.TrimSpace(`tuip dashboard create work slack github jira asana
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
				var registry *providers.Registry

				registry, err = newRegistry()
				if err != nil {
					return fmt.Errorf("initialize provider registry: %w", err)
				}

				providerIDs, err = registry.CanonicalIDs(providerIDs)
				if err != nil {
					return fmt.Errorf("resolve dashboard providers: %w", err)
				}
			}

			err = cfg.CreateDashboard(name)
			if err != nil {
				return fmt.Errorf("create dashboard %q: %w", name, err)
			}

			if len(providerIDs) > 0 {
				err = cfg.AddProviders(name, providerIDs)
				if err != nil {
					return fmt.Errorf("add providers to dashboard %q: %w", name, err)
				}
			}

			err = config.Save(path, cfg)
			if err != nil {
				return fmt.Errorf("save config: %w", err)
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

				err := writef(cmd.OutOrStdout(), "%s %s\n", marker, name)
				if err != nil {
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

			err = writef(cmd.OutOrStdout(), "dashboard: %s\n", name)
			if err != nil {
				return err
			}

			if cfg.DefaultDashboard == name {
				err = writeln(cmd.OutOrStdout(), "default:   true")
				if err != nil {
					return err
				}
			}

			providerIDs := dashboard.ProviderIDs()
			if len(providerIDs) == 0 {
				return writeln(cmd.OutOrStdout(), "services:  none")
			}

			err = writeln(cmd.OutOrStdout(), "services:")
			if err != nil {
				return err
			}

			for _, providerID := range providerIDs {
				err := writef(cmd.OutOrStdout(), "  - %s\n", providerID)
				if err != nil {
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

			err = cfg.SetDefaultDashboard(name)
			if err != nil {
				return fmt.Errorf("set default dashboard %q: %w", name, err)
			}

			err = config.Save(path, cfg)
			if err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			return writef(cmd.OutOrStdout(), "default dashboard set to %q\n", name)
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:               "add <name> <provider...>",
		Short:             "Add providers to a dashboard",
		Example:           "tuip dashboard add work slack github cloudflare",
		Args:              cobra.MinimumNArgs(providersArgCount),
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
				return fmt.Errorf("resolve dashboard providers: %w", err)
			}

			err = cfg.AddProviders(name, providerIDs)
			if err != nil {
				return fmt.Errorf("add providers to dashboard %q: %w", name, err)
			}

			err = config.Save(path, cfg)
			if err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			return writef(cmd.OutOrStdout(), "added %s to dashboard %q\n", strings.Join(providerIDs, ", "), name)
		},
	})

	dashboards.AddCommand(&cobra.Command{
		Use:               "remove <name> <provider...>",
		Aliases:           []string{"rm"},
		Short:             "Remove providers from a dashboard",
		Example:           "tuip dashboard remove work github",
		Args:              cobra.MinimumNArgs(providersArgCount),
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
				return fmt.Errorf("resolve dashboard providers: %w", err)
			}

			err = cfg.RemoveProviders(name, providerIDs)
			if err != nil {
				return fmt.Errorf("remove providers from dashboard %q: %w", name, err)
			}

			err = config.Save(path, cfg)
			if err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			return writef(cmd.OutOrStdout(), "removed %s from dashboard %q\n", strings.Join(providerIDs, ", "), name)
		},
	})

	return dashboards
}

func resolveStatusProviderIDs(configPath string, registry *providers.Registry, dashboardName string, args []string) ([]string, error) {
	if len(args) > 0 && dashboardName != "" {
		return nil, errors.New("pass either explicit providers or --dashboard, not both")
	}

	if len(args) > 0 {
		return normalizeProviderIDs(args), nil
	}

	_, cfg, err := loadConfig(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("no config found; pass providers explicitly, e.g. `tuip status slack github`, or create a dashboard")
		}

		return nil, err
	}

	name := dashboardName
	if name == "" {
		name = cfg.DefaultDashboard
	}

	if name == "" {
		return nil, errors.New("no default dashboard configured; pass providers explicitly or run `tuip dashboard use <name>`")
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
		return "", nil, fmt.Errorf("resolve config path: %w", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		return path, nil, fmt.Errorf("load config: %w", err)
	}

	return path, cfg, nil
}

func loadOrNewConfig(pathOverride string) (string, *config.Config, error) {
	path, err := config.ResolvePath(pathOverride)
	if err != nil {
		return "", nil, fmt.Errorf("resolve config path: %w", err)
	}

	cfg, err := config.LoadOrNew(path)
	if err != nil {
		return path, nil, fmt.Errorf("load config: %w", err)
	}

	return path, cfg, nil
}

func newRegistry() (*providers.Registry, error) {
	client := fetch.NewClient(registryTimeout)

	registry, err := builtin.NewRegistry(client)
	if err != nil {
		return nil, fmt.Errorf("create built-in provider registry: %w", err)
	}

	return registry, nil
}

func newLogger(configPath, logLevel string) (*slog.Logger, func(), error) {
	logger, closer, _, err := diagnostics.NewLogger(configPath, logLevel, version)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize diagnostics logger: %w", err)
	}

	return logger, func() { _ = closer.Close() }, nil
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
	writer := tabwriter.NewWriter(w, 0, 0, tablePadding, ' ', 0)

	err := writeln(writer, "ID\tALIASES\tCATEGORY\tNAME\tSOURCE\tAPI")
	if err != nil {
		return err
	}

	for _, item := range metadata {
		aliases := strings.Join(item.Aliases, ", ")

		err = writef(writer, "%s\t%s\t%s\t%s\t%s\t%s\n", item.ID, aliases, item.Category, item.Name, item.SourceURL, item.APIURL)
		if err != nil {
			return err
		}
	}

	err = writer.Flush()
	if err != nil {
		return fmt.Errorf("flush provider metadata table: %w", err)
	}

	return nil
}

func writeln(w io.Writer, args ...any) error {
	_, err := fmt.Fprintln(w, args...)
	if err != nil {
		return fmt.Errorf("write line: %w", err)
	}

	return nil
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	if err != nil {
		return fmt.Errorf("write formatted output: %w", err)
	}

	return nil
}
