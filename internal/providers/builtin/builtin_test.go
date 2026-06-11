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
		"chargebee",
		"circleci",
		"clerk",
		"cloudsmith",
		"coda",
		"codefresh",
		"cohere",
		"convex",
		"coralogix",
		"courier",
		"crates-io",
		"depot",
		"discord",
		"doppler",
		"drata",
		"expo",
		"figma",
		"fullstory",
		"gitguardian",
		"gocardless",
		"grafana-cloud",
		"harness",
		"heap",
		"honeycomb",
		"incident-io",
		"infisical",
		"influxdb-cloud",
		"inngest",
		"jfrog",
		"kong",
		"launchdarkly",
		"logzio",
		"mailgun",
		"maven-central",
		"mend",
		"mintlify",
		"miro",
		"mixpanel",
		"npm",
		"nx-cloud",
		"onesignal",
		"opsgenie",
		"pagerduty",
		"pendo",
		"postman",
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
		"shortcut",
		"snyk",
		"sparkpost",
		"splunk-on-call",
		"square",
		"statuscake",
		"stoplight",
		"stream",
		"svix",
		"tailscale",
		"teleport-cloud",
		"temporal-cloud",
		"travis-ci",
		"tyk",
		"upstash",
		"vanta",
		"veracode",
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
		"maven":               "maven-central",
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
