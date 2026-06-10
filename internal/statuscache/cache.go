package statuscache

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ikan31/tuip/internal/status"
)

const (
	CurrentVersion = 1

	cacheDirPerm  = 0o750
	cacheFilePerm = 0o600
)

// LookupState describes whether a cache lookup can satisfy a provider status
// request.
type LookupState string

const (
	LookupHit   LookupState = "hit"
	LookupMiss  LookupState = "miss"
	LookupStale LookupState = "stale"
)

// Cache stores the latest normalized provider snapshots in a small JSON file.
// It is intentionally keyed by provider ID so dashboards can share results.
type Cache struct {
	path    string
	mu      sync.Mutex
	entries map[string]Entry
}

// Entry is one provider snapshot plus freshness metadata.
type Entry struct {
	FetchedAt time.Time       `json:"fetched_at"`
	ExpiresAt time.Time       `json:"expires_at"`
	Snapshot  status.Snapshot `json:"snapshot"`
}

type filePayload struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

// New returns an empty cache bound to path.
func New(path string) *Cache {
	return &Cache{path: path, entries: map[string]Entry{}}
}

// LoadOrNew loads an existing cache file or returns an empty cache if it does
// not exist.
func LoadOrNew(path string) (*Cache, error) {
	// #nosec G304 -- path is derived from tuip's configured runtime directory.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return New(path), nil
		}

		return nil, fmt.Errorf("read status cache %s: %w", path, err)
	}

	if len(data) == 0 {
		return New(path), nil
	}

	var payload filePayload

	err = json.Unmarshal(data, &payload)
	if err != nil {
		return nil, fmt.Errorf("parse status cache %s: %w", path, err)
	}

	if payload.Entries == nil {
		payload.Entries = map[string]Entry{}
	}

	return &Cache{path: path, entries: payload.Entries}, nil
}

// Path returns the backing JSON file path.
func (c *Cache) Path() string {
	if c == nil {
		return ""
	}

	return c.path
}

// Lookup returns a fresh cached snapshot if available. Stale and missing entries
// are reported separately for diagnostics.
func (c *Cache) Lookup(providerID string, now time.Time) (status.Snapshot, LookupState, time.Duration) {
	if c == nil {
		return status.Snapshot{}, LookupMiss, 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[providerID]
	if !ok {
		return status.Snapshot{}, LookupMiss, 0
	}

	age := now.Sub(entry.FetchedAt)
	if now.After(entry.ExpiresAt) {
		return entry.Snapshot, LookupStale, age
	}

	return entry.Snapshot, LookupHit, age
}

// Set stores a snapshot with a TTL. Non-positive TTLs skip caching.
func (c *Cache) Set(providerID string, snapshot status.Snapshot, ttl time.Duration, now time.Time) {
	if c == nil || ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil {
		c.entries = map[string]Entry{}
	}

	c.entries[providerID] = Entry{
		FetchedAt: now,
		ExpiresAt: now.Add(ttl),
		Snapshot:  snapshot,
	}
}

// Save writes the current cache contents to disk.
func (c *Cache) Save() error {
	if c == nil {
		return nil
	}

	c.mu.Lock()

	payload := filePayload{
		Version: CurrentVersion,
		Entries: make(map[string]Entry, len(c.entries)),
	}
	maps.Copy(payload.Entries, c.entries)

	c.mu.Unlock()

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal status cache: %w", err)
	}

	err = os.MkdirAll(filepath.Dir(c.path), cacheDirPerm)
	if err != nil {
		return fmt.Errorf("create status cache directory: %w", err)
	}

	err = os.WriteFile(c.path, append(data, '\n'), cacheFilePerm)
	if err != nil {
		return fmt.Errorf("write status cache %s: %w", c.path, err)
	}

	return nil
}
