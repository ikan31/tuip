package builtin

import (
	"testing"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
)

func TestNewRegistryIncludesWave1StatuspageProviders(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fetch.NewClient(time.Second))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	wantCanonicalIDs := []string{
		"cloudflare",
		"sigma",
		"matillion",
		"snowflake",
		"fivetran",
		"dbt-cloud",
		"atlan",
		"clickhouse-cloud",
		"hex",
		"metabase-cloud",
		"omni",
		"preset",
		"astronomer",
		"hevo",
		"bigeye",
		"confluent-cloud",
		"dremio-cloud",
		"mongodb-cloud",
		"elastic-cloud",
		"aiven",
		"supabase",
		"planetscale",
		"pinecone",
		"zilliz",
		"vercel",
		"netlify",
		"render",
		"fly",
		"digitalocean",
		"sentry",
		"datadog-us1",
		"datadog-eu",
		"datadog-us5",
		"datadog-ap1",
		"datadog-ap2",
		"datadog-govcloud",
		"datadog-us2-gov",
		"new-relic",
		"twilio",
		"openai",
		"anthropic",
	}

	for _, id := range wantCanonicalIDs {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			if !registry.Has(id) {
				t.Fatalf("registry.Has(%q) = false, want true", id)
			}

			provider, ok := registry.Get(id)
			if !ok {
				t.Fatalf("registry.Get(%q) ok = false, want true", id)
			}

			metadata := provider.Metadata()
			if metadata.SourceURL == "" {
				t.Fatalf("provider %q SourceURL is empty", id)
			}

			if metadata.APIURL == "" {
				t.Fatalf("provider %q APIURL is empty", id)
			}

			if metadata.Category == "" {
				t.Fatalf("provider %q Category is empty", id)
			}
		})
	}
}

func TestNewRegistryIncludesAWSProvider(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fetch.NewClient(time.Second))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	provider, ok := registry.Get("aws")
	if !ok {
		t.Fatalf("registry.Get(%q) ok = false, want true", "aws")
	}

	metadata := provider.Metadata()
	if metadata.Name != "Amazon Web Services" {
		t.Fatalf("aws Name = %q, want Amazon Web Services", metadata.Name)
	}

	if metadata.Category != "Cloud & Hosting" {
		t.Fatalf("aws Category = %q, want Cloud & Hosting", metadata.Category)
	}

	got, ok := registry.CanonicalID("amazon-web-services")
	if !ok {
		t.Fatalf("CanonicalID(%q) ok = false, want true", "amazon-web-services")
	}

	if got != "aws" {
		t.Fatalf("CanonicalID(%q) = %q, want aws", "amazon-web-services", got)
	}
}

func TestNewRegistryIncludesAzureProvider(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fetch.NewClient(time.Second))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	provider, ok := registry.Get("azure")
	if !ok {
		t.Fatalf("registry.Get(%q) ok = false, want true", "azure")
	}

	metadata := provider.Metadata()
	if metadata.Name != "Microsoft Azure" {
		t.Fatalf("azure Name = %q, want Microsoft Azure", metadata.Name)
	}

	if metadata.Category != "Cloud & Hosting" {
		t.Fatalf("azure Category = %q, want Cloud & Hosting", metadata.Category)
	}

	aliases := []string{"microsoft-azure", "msazure"}
	for _, alias := range aliases {
		got, ok := registry.CanonicalID(alias)
		if !ok {
			t.Fatalf("CanonicalID(%q) ok = false, want true", alias)
		}

		if got != "azure" {
			t.Fatalf("CanonicalID(%q) = %q, want azure", alias, got)
		}
	}
}

func TestNewRegistryIncludesDockerProvider(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fetch.NewClient(time.Second))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	provider, ok := registry.Get("docker")
	if !ok {
		t.Fatalf("registry.Get(%q) ok = false, want true", "docker")
	}

	metadata := provider.Metadata()
	if metadata.Name != "Docker" {
		t.Fatalf("docker Name = %q, want Docker", metadata.Name)
	}

	if metadata.Category != "Package Registries" {
		t.Fatalf("docker Category = %q, want Package Registries", metadata.Category)
	}

	aliases := []string{"docker-hub", "dockerhub"}
	for _, alias := range aliases {
		got, ok := registry.CanonicalID(alias)
		if !ok {
			t.Fatalf("CanonicalID(%q) ok = false, want true", alias)
		}

		if got != "docker" {
			t.Fatalf("CanonicalID(%q) = %q, want docker", alias, got)
		}
	}
}

func TestNewRegistryIncludesGCPProvider(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fetch.NewClient(time.Second))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	provider, ok := registry.Get("gcp")
	if !ok {
		t.Fatalf("registry.Get(%q) ok = false, want true", "gcp")
	}

	metadata := provider.Metadata()
	if metadata.Name != "Google Cloud" {
		t.Fatalf("gcp Name = %q, want Google Cloud", metadata.Name)
	}

	if metadata.Category != "Cloud & Hosting" {
		t.Fatalf("gcp Category = %q, want Cloud & Hosting", metadata.Category)
	}

	aliases := []string{"google-cloud", "google-cloud-platform"}
	for _, alias := range aliases {
		got, ok := registry.CanonicalID(alias)
		if !ok {
			t.Fatalf("CanonicalID(%q) ok = false, want true", alias)
		}

		if got != "gcp" {
			t.Fatalf("CanonicalID(%q) = %q, want gcp", alias, got)
		}
	}
}

func TestNewRegistryIncludesGitHubProviders(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fetch.NewClient(time.Second))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	tests := map[string][]string{
		"github":                     nil,
		"github-enterprise-cloud-au": {"github-au", "ghec-au"},
		"github-enterprise-cloud-eu": {"github-eu", "ghec-eu"},
		"github-enterprise-cloud-jp": {"github-jp", "ghec-jp"},
		"github-enterprise-cloud-us": {"github-us", "ghec-us"},
	}

	for id, aliases := range tests {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			provider, ok := registry.Get(id)
			if !ok {
				t.Fatalf("registry.Get(%q) ok = false, want true", id)
			}

			metadata := provider.Metadata()
			if metadata.Category != "Developer Tools" {
				t.Fatalf("provider %q Category = %q, want Developer Tools", id, metadata.Category)
			}

			for _, alias := range aliases {
				got, ok := registry.CanonicalID(alias)
				if !ok {
					t.Fatalf("CanonicalID(%q) ok = false, want true", alias)
				}

				if got != id {
					t.Fatalf("CanonicalID(%q) = %q, want %q", alias, got, id)
				}
			}
		})
	}
}

func TestNewRegistryIncludesDeveloperSaaSProviders(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fetch.NewClient(time.Second))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	ids := []string{
		"1password",
		"ably",
		"airbyte",
		"airtable",
		"akeyless",
		"amplitude",
		"apigee",
		"axiom",
		"baseten",
		"bitrise",
		"braze",
		"bugsnag",
		"census",
		"censys",
		"chargebee",
		"circleci",
		"clerk",
		"codemagic",
		"cloudsmith",
		"coda",
		"codeberg",
		"codefresh",
		"cohere",
		"convex",
		"coralogix",
		"courier",
		"crates-io",
		"cursor",
		"depot",
		"discord",
		"doppler",
		"drata",
		"expo",
		"figma",
		"flagsmith",
		"forgejo",
		"fullstory",
		"gitguardian",
		"gocardless",
		"grafana-cloud",
		"harness",
		"hashicorp",
		"harvey",
		"heap",
		"hightouch",
		"honeycomb",
		"incident-io",
		"infisical",
		"influxdb-cloud",
		"inngest",
		"jetbrains-ai",
		"jfrog",
		"keeper",
		"kong",
		"launchdarkly",
		"linear",
		"logzio",
		"lovable",
		"mailgun",
		"maven-central",
		"mend",
		"mintlify",
		"miro",
		"mixpanel",
		"motherduck",
		"notion",
		"npm",
		"nx-cloud",
		"onesignal",
		"opsgenie",
		"optimizely",
		"pagerduty",
		"pantheon",
		"pendo",
		"perplexity",
		"postman",
		"proton",
		"pubnub",
		"pusher",
		"pypi",
		"quay",
		"readme",
		"replicate",
		"revenuecat",
		"rollbar",
		"rubygems",
		"rudderstack",
		"segment",
		"semaphore",
		"semgrep",
		"sendgrid",
		"shopify",
		"shortcut",
		"smartsheet",
		"snyk",
		"sparkpost",
		"splunk-cloud",
		"splunk-observability-au0",
		"splunk-observability-eu0",
		"splunk-observability-eu2",
		"splunk-observability-jp0",
		"splunk-observability-sg0",
		"splunk-observability-us0",
		"splunk-observability-us1",
		"splunk-observability-us2",
		"splunk-on-call",
		"square",
		"statuscake",
		"stoplight",
		"stream",
		"swagger",
		"svix",
		"tailscale",
		"teleport-cloud",
		"temporal-cloud",
		"tidb-cloud",
		"tinybird",
		"travis-ci",
		"tyk",
		"upstash",
		"vanta",
		"veracode",
		"windsurf",
		"wiz",
		"workos",
		"zoom",
		"zuora",
		"zuplo",
	}

	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			provider, ok := registry.Get(id)
			if !ok {
				t.Fatalf("registry.Get(%q) ok = false, want true", id)
			}

			metadata := provider.Metadata()
			if metadata.SourceURL == "" || metadata.APIURL == "" || metadata.Category == "" {
				t.Fatalf("provider %q has incomplete metadata: %#v", id, metadata)
			}
		})
	}

	aliases := map[string]string{
		"onepassword":         "1password",
		"circle-ci":           "circleci",
		"eas":                 "expo",
		"grafana":             "grafana-cloud",
		"harvey-ai":           "harvey",
		"hcp":                 "hashicorp",
		"jetbrains":           "jetbrains-ai",
		"maven":               "maven-central",
		"signalfx":            "splunk-observability-us0",
		"signalfx-us1":        "splunk-observability-us1",
		"smartbear-swagger":   "swagger",
		"splunkcloud":         "splunk-cloud",
		"terraform":           "hashicorp",
		"terraform-cloud":     "hashicorp",
		"tidb":                "tidb-cloud",
		"turborepo":           "vercel",
		"vercel-remote-cache": "vercel",
		"victorops":           "splunk-on-call",
	}

	for alias, want := range aliases {
		t.Run(alias, func(t *testing.T) {
			t.Parallel()

			got, ok := registry.CanonicalID(alias)
			if !ok {
				t.Fatalf("CanonicalID(%q) ok = false, want true", alias)
			}

			if got != want {
				t.Fatalf("CanonicalID(%q) = %q, want %q", alias, got, want)
			}
		})
	}
}

func TestNewRegistryIncludesWave1Aliases(t *testing.T) {
	t.Parallel()

	registry, err := NewRegistry(fetch.NewClient(time.Second))
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	tests := map[string]string{
		"dbt":             "dbt-cloud",
		"clickhouse":      "clickhouse-cloud",
		"metabase":        "metabase-cloud",
		"confluent":       "confluent-cloud",
		"dremio":          "dremio-cloud",
		"mongodb-atlas":   "mongodb-cloud",
		"fly.io":          "fly",
		"datadog":         "datadog-us1",
		"ddog-eu":         "datadog-eu",
		"ddog-us5":        "datadog-us5",
		"ddog-ap1":        "datadog-ap1",
		"ddog-ap2":        "datadog-ap2",
		"ddog-gov":        "datadog-govcloud",
		"ddog-us2-gov":    "datadog-us2-gov",
		"newrelic":        "new-relic",
		"chatgpt":         "openai",
		"claude":          "anthropic",
		"sigma-computing": "sigma",
	}

	for alias, want := range tests {
		t.Run(alias, func(t *testing.T) {
			t.Parallel()

			got, ok := registry.CanonicalID(alias)
			if !ok {
				t.Fatalf("CanonicalID(%q) ok = false, want true", alias)
			}

			if got != want {
				t.Fatalf("CanonicalID(%q) = %q, want %q", alias, got, want)
			}
		})
	}
}
