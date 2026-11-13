package prepare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/sqlrs/engine-local/internal/store"
)

const (
	defaultCapacityHighWatermark = 0.90
	defaultCapacityLowWatermark  = 0.80
	defaultCapacityMinStateAge   = 10 * time.Minute
	defaultReserveMinBytes       = int64(10 << 30) // 10 GiB
	evictLockFileName            = ".evict.lock"
	evictLockStaleAfter          = 5 * time.Minute
)

type capacitySettings struct {
	MaxBytes      int64
	ReserveBytes  int64
	HighWatermark float64
	LowWatermark  float64
	MinStateAge   time.Duration
	EffectiveMax  int64
	Enabled       bool
}

type evictCandidate struct {
	StateID    string
	ImageID    string
	CreatedAt  time.Time
	LastUsedAt time.Time
	SizeBytes  int64
}

var (
	filesystemStatsFn = filesystemStats
	cacheUsageFn      = measureCacheUsage
	storeUsageFn      = measureStoreUsage
	nowUTCFn          = func() time.Time { return time.Now().UTC() }
)

// ensureCacheCapacity applies strict cache budget enforcement before/after snapshot
// phases as defined in docs/architecture/state-cache-capacity-control.md.
func (m *PrepareService) ensureCacheCapacity(ctx context.Context, jobID string, phase string, protectedStateIDs ...string) *ErrorResponse {
	if m == nil || m.config == nil || strings.TrimSpace(m.stateStoreRoot) == "" {
		return nil
	}
	totalBytes, freeBytes, err := filesystemStatsFn(m.stateStoreRoot)
	if err != nil {
		return capacityError("cache_enforcement_unavailable", "cannot measure state store filesystem", map[string]any{
			"phase": phase,
			"error": err.Error(),
		})
	}
	settings, err := m.loadCapacitySettings(totalBytes)
	if err != nil {
		return capacityError("cache_enforcement_unavailable", "cannot load cache capacity settings", map[string]any{
			"phase": phase,
			"error": err.Error(),
		})
	}
	if !settings.Enabled {
		return nil
	}
	if settings.EffectiveMax <= 0 {
		return capacityError("cache_limit_too_small", "effective cache limit is too small", map[string]any{
			"phase":               phase,
			"effective_max_bytes": settings.EffectiveMax,
			"store_total_bytes":   totalBytes,
			"reserve_bytes":       settings.ReserveBytes,
		})
	}

	usageBytes, err := cacheUsageFn(m.stateStoreRoot)
	if err != nil {
		return capacityError("cache_enforcement_unavailable", "cannot measure cache usage", map[string]any{
			"phase": phase,
			"error": err.Error(),
		})
	}
	if !isCapacityPressure(usageBytes, freeBytes, settings) {
		return nil
	}

	eviction := evictionSummary{}
	lockErr := withEvictLock(ctx, m.stateStoreRoot, func() error {
		summary, runErr := m.runEviction(ctx, settings, usageBytes, freeBytes, protectedStateIDs...)
		eviction = summary
		return runErr
	})
	if lockErr != nil {
		return capacityError("cache_enforcement_unavailable", "cannot acquire cache eviction lock", map[string]any{
			"phase": phase,
			"error": lockErr.Error(),
		})
	}

	usageAfter, usageErr := cacheUsageFn(m.stateStoreRoot)
	if usageErr != nil {
		return capacityError("cache_enforcement_unavailable", "cannot measure cache usage after eviction", map[string]any{
			"phase": phase,
			"error": usageErr.Error(),
		})
	}
	_, freeAfter, freeErr := filesystemStatsFn(m.stateStoreRoot)
	if freeErr != nil {
		return capacityError("cache_enforcement_unavailable", "cannot measure free space after eviction", map[string]any{
			"phase": phase,
			"error": freeErr.Error(),
		})
	}
	m.recordCacheEviction(CacheEvictionSummary{
		CompletedAt:      nowUTCFn().Format(time.RFC3339Nano),
		Trigger:          phase,
		EvictedCount:     eviction.EvictedCount,
		FreedBytes:       eviction.FreedBytes,
		BlockedCount:     eviction.BlockedCount,
		ReclaimableBytes: eviction.ReclaimableBytes,
		UsageBytesBefore: usageBytes,
		UsageBytesAfter:  usageAfter,
		FreeBytesBefore:  freeBytes,
		FreeBytesAfter:   freeAfter,
	})

	if !isCapacityPressure(usageAfter, freeAfter, settings) {
		return nil
	}

	reasons := pressureReasons(usageAfter, freeAfter, settings)
	observedRequired := m.observedMinStateBytes(ctx)
	if observedRequired > 0 && settings.EffectiveMax < observedRequired {
		return capacityError("cache_limit_too_small", "effective cache limit is below observed minimum state size", map[string]any{
			"phase":                   phase,
			"effective_max_bytes":     settings.EffectiveMax,
			"observed_required_bytes": observedRequired,
			"recommended_min_bytes":   observedRequired,
			"reasons":                 reasons,
		})
	}
	if eviction.ReclaimableBytes == 0 && eviction.BlockedCount == 0 {
		return capacityError("cache_limit_too_small", "effective cache limit is too small to materialize a state", map[string]any{
			"phase":                   phase,
			"effective_max_bytes":     settings.EffectiveMax,
			"observed_required_bytes": observedRequired,
			"recommended_min_bytes":   maxInt64(observedRequired, settings.EffectiveMax+1),
			"reasons":                 reasons,
		})
	}

	return capacityError("cache_full_unreclaimable", "cache cannot be reclaimed to required thresholds", map[string]any{
		"phase":               phase,
		"reasons":             reasons,
		"usage_bytes":         usageAfter,
		"free_bytes":          freeAfter,
		"effective_max_bytes": settings.EffectiveMax,
		"reserve_bytes":       settings.ReserveBytes,
		"high_watermark":      settings.HighWatermark,
		"low_watermark":       settings.LowWatermark,
		"evicted_count":       eviction.EvictedCount,
		"freed_bytes":         eviction.FreedBytes,
		"blocked_count":       eviction.BlockedCount,
		"reclaimable_bytes":   eviction.ReclaimableBytes,
	})
}

func (m *PrepareService) loadCapacitySettings(storeTotalBytes int64) (capacitySettings, error) {
	high := defaultCapacityHighWatermark
	low := defaultCapacityLowWatermark
	maxBytes := int64(0)
	minAge := defaultCapacityMinStateAge
	var reserveOverride *int64

	if m.config != nil {
		if value, err := m.config.Get("cache.capacity.maxBytes", true); err == nil {
			if value == nil {
				maxBytes = 0
			} else {
				parsed, ok := asInt64(value)
				if !ok {
					return capacitySettings{}, fmt.Errorf("cache.capacity.maxBytes is invalid")
				}
				maxBytes = parsed
			}
		}
		if value, err := m.config.Get("cache.capacity.reserveBytes", true); err == nil {
			if value != nil {
				parsed, ok := asInt64(value)
				if !ok {
					return capacitySettings{}, fmt.Errorf("cache.capacity.reserveBytes is invalid")
				}
				reserveOverride = &parsed
			}
		}
		if value, err := m.config.Get("cache.capacity.highWatermark", true); err == nil {
			if value != nil {
				parsed, ok := asFloat64(value)
				if !ok {
					return capacitySettings{}, fmt.Errorf("cache.capacity.highWatermark is invalid")
				}
				high = parsed
			}
		}
		if value, err := m.config.Get("cache.capacity.lowWatermark", true); err == nil {
			if value != nil {
				parsed, ok := asFloat64(value)
				if !ok {
					return capacitySettings{}, fmt.Errorf("cache.capacity.lowWatermark is invalid")
				}
				low = parsed
			}
		}
		if value, err := m.config.Get("cache.capacity.minStateAge", true); err == nil {
			if value != nil {
				str, ok := value.(string)
				if !ok {
					return capacitySettings{}, fmt.Errorf("cache.capacity.minStateAge is invalid")
				}
				parsed, parseErr := time.ParseDuration(strings.TrimSpace(str))
				if parseErr != nil {
					return capacitySettings{}, fmt.Errorf("cache.capacity.minStateAge is invalid")
				}
				minAge = parsed
			}
		}
	}

	if maxBytes < 0 {
		return capacitySettings{}, fmt.Errorf("cache.capacity.maxBytes is invalid")
	}
	if reserveOverride != nil && *reserveOverride < 0 {
		return capacitySettings{}, fmt.Errorf("cache.capacity.reserveBytes is invalid")
	}
	if high <= 0 || high > 1 {
		return capacitySettings{}, fmt.Errorf("cache.capacity.highWatermark is invalid")
	}
	if low <= 0 || low >= high {
		return capacitySettings{}, fmt.Errorf("cache.capacity.lowWatermark is invalid")
	}
	if minAge < 0 {
		return capacitySettings{}, fmt.Errorf("cache.capacity.minStateAge is invalid")
	}

	if storeTotalBytes < 0 {
		storeTotalBytes = 0
	}
	reserve := defaultReserveBytes(storeTotalBytes)
	if reserveOverride != nil {
		reserve = *reserveOverride
	}
	if reserve < 0 {
		reserve = 0
	}
	effectiveFromStore := storeTotalBytes - reserve
	if effectiveFromStore < 0 {
		effectiveFromStore = 0
	}
	effectiveMax := effectiveFromStore
	if maxBytes > 0 && maxBytes < effectiveMax {
		effectiveMax = maxBytes
	}
	return capacitySettings{
		MaxBytes:      maxBytes,
		ReserveBytes:  reserve,
		HighWatermark: high,
		LowWatermark:  low,
		MinStateAge:   minAge,
		EffectiveMax:  effectiveMax,
		Enabled:       true,
	}, nil
}

func defaultReserveBytes(total int64) int64 {
	if total <= 0 {
		return defaultReserveMinBytes
	}
	percent := int64(float64(total) * 0.10)
	if percent < defaultReserveMinBytes {
		return defaultReserveMinBytes
	}
	return percent
}

func isCapacityPressure(usageBytes, freeBytes int64, settings capacitySettings) bool {
	highBytes := int64(float64(settings.EffectiveMax) * settings.HighWatermark)
	if usageBytes > highBytes {
		return true
	}
	return freeBytes < settings.ReserveBytes
}

func pressureReasons(usageBytes, freeBytes int64, settings capacitySettings) []string {
	reasons := []string{}
	highBytes := int64(float64(settings.EffectiveMax) * settings.HighWatermark)
	if usageBytes > highBytes {
		reasons = append(reasons, "usage_above_high_watermark")
	}
	if freeBytes < settings.ReserveBytes {
		reasons = append(reasons, "physical_free_below_reserve")
	}
	return reasons
}

type evictionSummary struct {
	EvictedCount     int
	FreedBytes       int64
	BlockedCount     int
	ReclaimableBytes int64
}

func (m *PrepareService) runEviction(ctx context.Context, settings capacitySettings, usageBytes int64, freeBytes int64, protectedStateIDs ...string) (evictionSummary, error) {
	summary := evictionSummary{}
	candidates, blocked, reclaimable, err := m.listEvictionCandidates(ctx, settings, protectedStateIDs...)
	if err != nil {
		return summary, err
	}
	summary.BlockedCount = blocked
	summary.ReclaimableBytes = reclaimable

	lowBytes := int64(float64(settings.EffectiveMax) * settings.LowWatermark)
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			return summary, ctx.Err()
		}
		if usageBytes <= lowBytes && freeBytes >= settings.ReserveBytes {
			break
		}
		if err := m.deleteEvictionCandidate(ctx, candidate); err != nil {
			continue
		}
		summary.EvictedCount++
		summary.FreedBytes += candidate.SizeBytes
		updatedUsage, usageErr := cacheUsageFn(m.stateStoreRoot)
		if usageErr != nil {
			return summary, usageErr
		}
		usageBytes = updatedUsage
		_, updatedFree, freeErr := filesystemStatsFn(m.stateStoreRoot)
		if freeErr != nil {
			return summary, freeErr
		}
		freeBytes = updatedFree
	}
	return summary, nil
}

func (m *PrepareService) listEvictionCandidates(ctx context.Context, settings capacitySettings, protectedStateIDs ...string) ([]evictCandidate, int, int64, error) {
	entries, err := m.store.ListStates(ctx, store.StateFilters{})
	if err != nil {
		return nil, 0, 0, err
	}
	protected := map[string]struct{}{}
	for _, stateID := range protectedStateIDs {
		trimmed := strings.TrimSpace(stateID)
		if trimmed == "" {
			continue
		}
		protected[trimmed] = struct{}{}
	}
	now := nowUTCFn()
	blocked := 0
	reclaimable := int64(0)
	out := make([]evictCandidate, 0, len(entries))
	for _, entry := range entries {
		if _, skip := protected[entry.StateID]; skip {
			blocked++
			continue
		}
		if entry.RefCount > 0 {
			blocked++
			continue
		}
		if held, err := store.PrepareProtectsState(ctx, m.store, entry.StateID); err != nil {
			return nil, 0, 0, err
		} else if held {
			blocked++
			continue
		}
		children, childErr := m.store.ListStates(ctx, store.StateFilters{ParentID: entry.StateID})
		if childErr != nil {
			return nil, 0, 0, childErr
		}
		if len(children) > 0 {
			blocked++
			continue
		}
		createdAt, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(entry.CreatedAt))
		if parseErr != nil {
			createdAt = now
		}
		if entry.MinRetentionUntil != nil {
			if minRetention, ok := parseRFC3339Any(*entry.MinRetentionUntil); ok && minRetention.After(now) {
				blocked++
				continue
			}
		}
		if settings.MinStateAge > 0 && now.Sub(createdAt) < settings.MinStateAge {
			blocked++
			continue
		}
		lastUsedAt := createdAt
		if entry.LastUsedAt != nil {
			if parsed, ok := parseRFC3339Any(*entry.LastUsedAt); ok {
				lastUsedAt = parsed
			}
		}
		size := int64(0)
		if entry.SizeBytes != nil && *entry.SizeBytes > 0 {
			size = *entry.SizeBytes
		} else {
			paths, resolveErr := resolveStatePaths(m.stateStoreRoot, entry.ImageID, entry.StateID, m.statefs)
			if resolveErr == nil {
				if measured, measureErr := storeUsageFn(paths.stateDir); measureErr == nil {
					size = measured
				}
			}
		}
		if size < 0 {
			size = 0
		}
		reclaimable += size
		out = append(out, evictCandidate{
			StateID:    entry.StateID,
			ImageID:    entry.ImageID,
			CreatedAt:  createdAt,
			LastUsedAt: lastUsedAt,
			SizeBytes:  size,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if !left.LastUsedAt.Equal(right.LastUsedAt) {
			return left.LastUsedAt.Before(right.LastUsedAt)
		}
		if left.SizeBytes != right.SizeBytes {
			return left.SizeBytes > right.SizeBytes
		}
		return left.StateID < right.StateID
	})
	return out, blocked, reclaimable, nil
}

func (m *PrepareService) deleteEvictionCandidate(ctx context.Context, candidate evictCandidate) error {
	// Recheck the durable owner before touching a previously listed candidate.
	// PG07: a later SQL refusal cannot restore already removed source bytes.
	if held, err := store.PrepareProtectsState(ctx, m.store, candidate.StateID); err != nil {
		return err
	} else if held {
		return fmt.Errorf("state is retained by prepare")
	}
	paths, err := resolveStatePaths(m.stateStoreRoot, candidate.ImageID, candidate.StateID, m.statefs)
	if err != nil {
		return err
	}
	if err := m.statefs.RemovePath(ctx, paths.stateDir); err != nil {
		return err
	}
	if err := m.store.DeleteState(ctx, candidate.StateID); err != nil {
		return err
	}
	return nil
}

func (m *PrepareService) observedMinStateBytes(ctx context.Context) int64 {
	entries, err := m.store.ListStates(ctx, store.StateFilters{})
	if err != nil {
		return 0
	}
	min := int64(0)
	for _, entry := range entries {
		if entry.SizeBytes == nil || *entry.SizeBytes <= 0 {
			continue
		}
		if min == 0 || *entry.SizeBytes < min {
			min = *entry.SizeBytes
		}
	}
	return min
}

func withEvictLock(ctx context.Context, stateStoreRoot string, fn func() error) error {
	if fn == nil {
		return fmt.Errorf("eviction callback is required")
	}
	if err := os.MkdirAll(stateStoreRoot, 0o700); err != nil {
		return err
	}
	lockPath := filepath.Join(stateStoreRoot, evictLockFileName)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		handle, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = handle.Close()
			defer os.Remove(lockPath)
			return fn()
		}
		if !os.IsExist(err) && !errors.Is(err, os.ErrExist) {
			return err
		}
		removed, cleanupErr := removeStaleEvictLock(lockPath, evictLockStaleAfter)
		if cleanupErr != nil {
			return cleanupErr
		}
		if removed {
			continue
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func removeStaleEvictLock(lockPath string, staleAfter time.Duration) (bool, error) {
	if staleAfter <= 0 {
		return false, nil
	}
	info, err := os.Stat(lockPath)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return false, fmt.Errorf("eviction lock path is a directory")
	}
	if time.Since(info.ModTime()) < staleAfter {
		return false, nil
	}
	if err := os.Remove(lockPath); err != nil {
		if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func capacityError(code string, message string, details map[string]any) *ErrorResponse {
	if len(details) == 0 {
		return errorResponse(code, message, "")
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return errorResponse(code, message, "")
	}
	return errorResponse(code, message, string(raw))
}

func asInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if float64(int64(v)) != v {
			return 0, false
		}
		return int64(v), true
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			return 0, false
		}
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func asFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func measureStoreUsage(path string) (int64, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return 0, nil
	}
	var total int64
	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		total += info.Size()
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return total, err
}

func measureCacheUsage(stateStoreRoot string) (int64, error) {
	stateStoreRoot = strings.TrimSpace(stateStoreRoot)
	if stateStoreRoot == "" {
		return 0, nil
	}
	enginesDir := filepath.Join(stateStoreRoot, "engines")
	info, err := os.Stat(enginesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		// Treat a non-directory engines path as missing cache root.
		// This can happen due to corruption or external interference and should not
		// fail capacity reporting.
		return 0, nil
	}
	engines, err := os.ReadDir(enginesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	var total int64
	for _, engine := range engines {
		if !engine.IsDir() {
			continue
		}
		engineDir := filepath.Join(enginesDir, engine.Name())
		versions, versionErr := os.ReadDir(engineDir)
		if versionErr != nil {
			if errors.Is(versionErr, os.ErrNotExist) {
				continue
			}
			return 0, versionErr
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			statesDir := filepath.Join(engineDir, version.Name(), "states")
			usage, usageErr := storeUsageFn(statesDir)
			if usageErr != nil {
				if errors.Is(usageErr, os.ErrNotExist) {
					continue
				}
				return 0, usageErr
			}
			total += usage
		}
	}
	return total, nil
}

func isNoSpaceError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ENOSPC) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no space left on device") ||
		strings.Contains(message, "not enough space on the disk")
}

func noSpaceErrorResponse(message string, phase string, err error) *ErrorResponse {
	if !isNoSpaceError(err) {
		return nil
	}
	return capacityError("cache_limit_too_small", message, map[string]any{
		"phase": phase,
		"error": err.Error(),
	})
}

func parseRFC3339Any(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
