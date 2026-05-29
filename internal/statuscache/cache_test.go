package statuscache

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/tuipcli/tuip/internal/status"
)

func TestCacheLookupHitMissStale(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	cache := New(filepath.Join(t.TempDir(), "status-cache.json"))

	_, state, _ := cache.Lookup("github", now)
	if state != LookupMiss {
		t.Fatalf("empty Lookup state = %q, want %q", state, LookupMiss)
	}

	cache.Set("github", status.Snapshot{ProviderID: "github", State: status.StateOperational}, time.Minute, now)

	snapshot, state, age := cache.Lookup("github", now.Add(30*time.Second))
	if state != LookupHit {
		t.Fatalf("fresh Lookup state = %q, want %q", state, LookupHit)
	}

	if snapshot.ProviderID != "github" || age != 30*time.Second {
		t.Fatalf("Lookup snapshot = %#v, age = %s", snapshot, age)
	}

	_, state, age = cache.Lookup("github", now.Add(2*time.Minute))
	if state != LookupStale {
		t.Fatalf("expired Lookup state = %q, want %q", state, LookupStale)
	}

	if age != 2*time.Minute {
		t.Fatalf("stale age = %s, want 2m", age)
	}
}

func TestCacheSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "cache", "status-cache.json")
	cache := New(path)
	cache.Set("slack", status.Snapshot{ProviderID: "slack", Name: "Slack", State: status.StateOperational}, time.Minute, now)

	err := cache.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := LoadOrNew(path)
	if err != nil {
		t.Fatalf("LoadOrNew() error = %v", err)
	}

	snapshot, state, _ := loaded.Lookup("slack", now.Add(time.Second))
	if state != LookupHit {
		t.Fatalf("Lookup state = %q, want %q", state, LookupHit)
	}

	if snapshot.Name != "Slack" || snapshot.State != status.StateOperational {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
