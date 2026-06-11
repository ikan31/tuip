package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ikan31/tuip/internal/fetch"
	"github.com/ikan31/tuip/internal/providers"
	"github.com/ikan31/tuip/internal/status"
	"github.com/ikan31/tuip/internal/statuscache"
)

const (
	ProviderTimeout       = 5 * time.Second
	DefaultMaxConcurrency = 8
	DefaultRetryCount     = 1
	DefaultCacheTTL       = 60 * time.Second
	DefaultErrorCacheTTL  = 10 * time.Second

	retryBackoff = 250 * time.Millisecond
)

// StatusOptions controls status fetch behavior.
type StatusOptions struct {
	Details       bool
	Cache         *statuscache.Cache
	CacheTTL      time.Duration
	ErrorCacheTTL time.Duration
	ForceRefresh  bool
	Logger        *slog.Logger
	MaxConcurrent int
	RetryCount    int
}

// ProviderStatusResult is a single streamed provider status update.
type ProviderStatusResult struct {
	Index        int
	Snapshot     status.Snapshot
	RuntimeError bool
	Done         bool
	Err          error
}

// CheckProviders fetches all requested providers concurrently and returns
// results in the same order as the requested IDs.
func CheckProviders(ctx context.Context, registry *providers.Registry, providerIDs []string, opts StatusOptions) (status.Response, error) {
	checkedAt := time.Now().UTC()
	response := status.Response{
		CheckedAt: checkedAt,
		Results:   make([]status.Snapshot, len(providerIDs)),
	}

	if len(providerIDs) == 0 {
		return response, errors.New("at least one provider is required")
	}

	err := registry.ValidateIDs(providerIDs)
	if err != nil {
		return response, fmt.Errorf("validate provider IDs: %w", err)
	}

	concurrency := opts.MaxConcurrent
	if concurrency <= 0 {
		concurrency = DefaultMaxConcurrency
	}

	if concurrency > len(providerIDs) {
		concurrency = len(providerIDs)
	}

	retryCount := opts.RetryCount
	if retryCount < 0 {
		retryCount = 0
	} else if retryCount == 0 {
		retryCount = DefaultRetryCount
	}

	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = DefaultCacheTTL
	}

	errorCacheTTL := opts.ErrorCacheTTL
	if errorCacheTTL <= 0 {
		errorCacheTTL = DefaultErrorCacheTTL
	}

	logDebug(opts.Logger, "status_check_start",
		slog.Int("provider_count", len(providerIDs)),
		slog.Bool("details", opts.Details),
		slog.Bool("force_refresh", opts.ForceRefresh),
		slog.Int("max_concurrent", concurrency),
		slog.Int("retry_count", retryCount),
	)

	var (
		wg              sync.WaitGroup
		mu              sync.Mutex
		hadRuntimeError bool
		cacheUpdated    bool
	)

	sem := make(chan struct{}, concurrency)

	for i, providerID := range providerIDs {
		provider, _ := registry.Get(providerID)
		metadata := provider.Metadata()

		wg.Go(func() {
			if !acquireProviderSlot(ctx, sem) {
				snapshot := errorSnapshot(metadata, "status check canceled before provider fetch")
				response.Results[i] = snapshot

				mu.Lock()
				hadRuntimeError = true
				mu.Unlock()

				return
			}
			defer releaseProviderSlot(sem)

			now := time.Now().UTC()
			if opts.Cache != nil && !opts.ForceRefresh {
				cached, lookupState, age := opts.Cache.Lookup(metadata.ID, now)
				logDebug(opts.Logger, "status_cache_lookup",
					slog.String("provider", metadata.ID),
					slog.String("state", string(lookupState)),
					slog.Int64("age_ms", age.Milliseconds()),
				)

				if lookupState == statuscache.LookupHit {
					response.Results[i] = snapshotForDetails(cached, opts.Details)

					if cached.State == status.StateError {
						mu.Lock()
						hadRuntimeError = true
						mu.Unlock()
					}

					return
				}
			}

			started := time.Now()

			logDebug(opts.Logger, "provider_fetch_start", slog.String("provider", metadata.ID))

			snapshot, fetchErr := fetchProviderWithRetry(ctx, provider, metadata.ID, retryCount, opts.Logger)
			duration := time.Since(started)

			if fetchErr != nil {
				snapshot = errorSnapshot(metadata, fetchErr.Error())

				logWarn(opts.Logger, "provider_fetch_error",
					slog.String("provider", metadata.ID),
					slog.Int64("duration_ms", duration.Milliseconds()),
					slog.String("error", fetchErr.Error()),
				)

				mu.Lock()
				hadRuntimeError = true
				mu.Unlock()
			} else {
				if snapshot.CheckedAt.IsZero() {
					snapshot.CheckedAt = time.Now().UTC()
				}

				logDebug(opts.Logger, "provider_fetch_done",
					slog.String("provider", metadata.ID),
					slog.String("state", string(snapshot.State)),
					slog.Int64("duration_ms", duration.Milliseconds()),
				)
			}

			if opts.Cache != nil {
				ttl := cacheTTL
				if snapshot.State == status.StateError {
					ttl = errorCacheTTL
				}

				opts.Cache.Set(metadata.ID, snapshot, ttl, time.Now().UTC())
				logDebug(opts.Logger, "status_cache_store",
					slog.String("provider", metadata.ID),
					slog.String("state", string(snapshot.State)),
					slog.Int64("ttl_ms", ttl.Milliseconds()),
				)

				mu.Lock()
				cacheUpdated = true
				mu.Unlock()
			}

			response.Results[i] = snapshotForDetails(snapshot, opts.Details)
		})
	}

	wg.Wait()

	if opts.Cache != nil && cacheUpdated {
		err := opts.Cache.Save()
		if err != nil {
			logWarn(opts.Logger, "status_cache_save_error", slog.String("path", opts.Cache.Path()), slog.String("error", err.Error()))
		} else {
			logDebug(opts.Logger, "status_cache_saved", slog.String("path", opts.Cache.Path()))
		}
	}

	if hadRuntimeError {
		logWarn(opts.Logger, "status_check_done", slog.String("result", "runtime_error"))

		return response, errors.New("one or more providers failed")
	}

	logDebug(opts.Logger, "status_check_done", slog.String("result", "ok"))

	return response, nil
}

// StreamProviders fetches requested providers concurrently and emits each
// provider snapshot as soon as it is available. A final Done result is emitted
// after cache persistence and aggregate error handling are complete.
func StreamProviders(ctx context.Context, registry *providers.Registry, providerIDs []string, opts StatusOptions) (<-chan ProviderStatusResult, error) {
	if len(providerIDs) == 0 {
		return nil, errors.New("at least one provider is required")
	}

	err := registry.ValidateIDs(providerIDs)
	if err != nil {
		return nil, fmt.Errorf("validate provider IDs: %w", err)
	}

	concurrency := opts.MaxConcurrent
	if concurrency <= 0 {
		concurrency = DefaultMaxConcurrency
	}

	if concurrency > len(providerIDs) {
		concurrency = len(providerIDs)
	}

	retryCount := opts.RetryCount
	if retryCount < 0 {
		retryCount = 0
	} else if retryCount == 0 {
		retryCount = DefaultRetryCount
	}

	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = DefaultCacheTTL
	}

	errorCacheTTL := opts.ErrorCacheTTL
	if errorCacheTTL <= 0 {
		errorCacheTTL = DefaultErrorCacheTTL
	}

	logDebug(opts.Logger, "status_stream_start",
		slog.Int("provider_count", len(providerIDs)),
		slog.Bool("details", opts.Details),
		slog.Bool("force_refresh", opts.ForceRefresh),
		slog.Int("max_concurrent", concurrency),
		slog.Int("retry_count", retryCount),
	)

	results := make(chan ProviderStatusResult)

	go func() {
		defer close(results)

		var (
			wg              sync.WaitGroup
			mu              sync.Mutex
			hadRuntimeError bool
			cacheUpdated    bool
		)

		sem := make(chan struct{}, concurrency)

		for i, providerID := range providerIDs {
			provider, _ := registry.Get(providerID)
			metadata := provider.Metadata()

			wg.Go(func() {
				if !acquireProviderSlot(ctx, sem) {
					snapshot := errorSnapshot(metadata, "status check canceled before provider fetch")

					mu.Lock()
					hadRuntimeError = true
					mu.Unlock()

					results <- ProviderStatusResult{Index: i, Snapshot: snapshotForDetails(snapshot, opts.Details), RuntimeError: true}

					return
				}
				defer releaseProviderSlot(sem)

				now := time.Now().UTC()
				if opts.Cache != nil && !opts.ForceRefresh {
					cached, lookupState, age := opts.Cache.Lookup(metadata.ID, now)
					logDebug(opts.Logger, "status_cache_lookup",
						slog.String("provider", metadata.ID),
						slog.String("state", string(lookupState)),
						slog.Int64("age_ms", age.Milliseconds()),
					)

					if lookupState == statuscache.LookupHit {
						runtimeError := cached.State == status.StateError
						if runtimeError {
							mu.Lock()
							hadRuntimeError = true
							mu.Unlock()
						}

						results <- ProviderStatusResult{Index: i, Snapshot: snapshotForDetails(cached, opts.Details), RuntimeError: runtimeError}

						return
					}
				}

				started := time.Now()

				logDebug(opts.Logger, "provider_fetch_start", slog.String("provider", metadata.ID))

				snapshot, fetchErr := fetchProviderWithRetry(ctx, provider, metadata.ID, retryCount, opts.Logger)
				duration := time.Since(started)
				runtimeError := false

				if fetchErr != nil {
					snapshot = errorSnapshot(metadata, fetchErr.Error())
					runtimeError = true

					logWarn(opts.Logger, "provider_fetch_error",
						slog.String("provider", metadata.ID),
						slog.Int64("duration_ms", duration.Milliseconds()),
						slog.String("error", fetchErr.Error()),
					)

					mu.Lock()
					hadRuntimeError = true
					mu.Unlock()
				} else {
					if snapshot.CheckedAt.IsZero() {
						snapshot.CheckedAt = time.Now().UTC()
					}

					logDebug(opts.Logger, "provider_fetch_done",
						slog.String("provider", metadata.ID),
						slog.String("state", string(snapshot.State)),
						slog.Int64("duration_ms", duration.Milliseconds()),
					)
				}

				if opts.Cache != nil {
					ttl := cacheTTL
					if snapshot.State == status.StateError {
						ttl = errorCacheTTL
					}

					opts.Cache.Set(metadata.ID, snapshot, ttl, time.Now().UTC())
					logDebug(opts.Logger, "status_cache_store",
						slog.String("provider", metadata.ID),
						slog.String("state", string(snapshot.State)),
						slog.Int64("ttl_ms", ttl.Milliseconds()),
					)

					mu.Lock()
					cacheUpdated = true
					mu.Unlock()
				}

				results <- ProviderStatusResult{Index: i, Snapshot: snapshotForDetails(snapshot, opts.Details), RuntimeError: runtimeError}
			})
		}

		wg.Wait()

		if opts.Cache != nil && cacheUpdated {
			err := opts.Cache.Save()
			if err != nil {
				logWarn(opts.Logger, "status_cache_save_error", slog.String("path", opts.Cache.Path()), slog.String("error", err.Error()))
			} else {
				logDebug(opts.Logger, "status_cache_saved", slog.String("path", opts.Cache.Path()))
			}
		}

		if hadRuntimeError {
			logWarn(opts.Logger, "status_stream_done", slog.String("result", "runtime_error"))

			results <- ProviderStatusResult{Done: true, Err: errors.New("one or more providers failed")}

			return
		}

		logDebug(opts.Logger, "status_stream_done", slog.String("result", "ok"))

		results <- ProviderStatusResult{Done: true}
	}()

	return results, nil
}

func acquireProviderSlot(ctx context.Context, sem chan struct{}) bool {
	select {
	case sem <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func releaseProviderSlot(sem chan struct{}) { <-sem }

func fetchProviderWithRetry(ctx context.Context, provider providers.Provider, providerID string, retryCount int, logger *slog.Logger) (status.Snapshot, error) {
	attempts := retryCount + 1

	var lastErr error

	for attempt := range attempts {
		attemptStarted := time.Now()
		attemptCtx, cancel := context.WithTimeout(ctx, ProviderTimeout)
		snapshot, err := provider.Fetch(attemptCtx)

		cancel()

		if err == nil {
			if attempt > 0 {
				logDebug(logger, "provider_fetch_retry_succeeded",
					slog.String("provider", providerID),
					slog.Int("attempt", attempt+1),
					slog.Int64("duration_ms", time.Since(attemptStarted).Milliseconds()),
				)
			}

			return snapshot, nil
		}

		lastErr = err
		if attempt == attempts-1 || !isRetryableError(err) || ctx.Err() != nil {
			return status.Snapshot{}, lastErr
		}

		attemptNumber := attempt + 1
		nextAttempt := attemptNumber + 1
		backoff := time.Duration(attemptNumber) * retryBackoff

		logDebug(logger, "provider_fetch_retry",
			slog.String("provider", providerID),
			slog.Int("next_attempt", nextAttempt),
			slog.Int64("backoff_ms", backoff.Milliseconds()),
			slog.String("error", err.Error()),
		)

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}

			return status.Snapshot{}, fmt.Errorf("context canceled during retry backoff: %w", ctx.Err())
		}
	}

	return status.Snapshot{}, lastErr
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	var httpStatusErr *fetch.HTTPStatusError
	if errors.As(err, &httpStatusErr) {
		return httpStatusErr.StatusCode == http.StatusTooManyRequests || httpStatusErr.StatusCode >= http.StatusInternalServerError
	}

	return false
}

func errorSnapshot(metadata providers.Metadata, message string) status.Snapshot {
	return status.Snapshot{
		ProviderID: metadata.ID,
		Name:       metadata.Name,
		State:      status.StateError,
		Summary:    "Failed to fetch status",
		SourceURL:  metadata.SourceURL,
		CheckedAt:  time.Now().UTC(),
		Incidents:  []status.Incident{},
		Components: []status.Component{},
		Error:      message,
	}
}

func snapshotForDetails(snapshot status.Snapshot, details bool) status.Snapshot {
	if !details {
		snapshot.Incidents = []status.Incident{}
		snapshot.Components = []status.Component{}

		return snapshot
	}

	if snapshot.Incidents == nil {
		snapshot.Incidents = []status.Incident{}
	}

	if snapshot.Components == nil {
		snapshot.Components = []status.Component{}
	}

	return snapshot
}

func logDebug(logger *slog.Logger, message string, attrs ...any) {
	if logger != nil {
		logger.Debug(message, attrs...)
	}
}

func logWarn(logger *slog.Logger, message string, attrs ...any) {
	if logger != nil {
		logger.Warn(message, attrs...)
	}
}
