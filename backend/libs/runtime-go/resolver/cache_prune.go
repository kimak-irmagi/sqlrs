package resolver

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type PrunePolicy struct {
	SemanticVersion string
	MaximumAge      time.Duration
	MaximumEntries  int
}
type PruneResult struct{ Removed, Retained int }
type PrunableCache interface {
	Prune(context.Context, PrunePolicy) (PruneResult, error)
}
type pruneEntry struct {
	name, path string
	modified   time.Time
}

func (c *DirectoryCache) Prune(ctx context.Context, policy PrunePolicy) (PruneResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.root)
	if err != nil {
		return PruneResult{}, err
	}
	var eligible []pruneEntry
	now := time.Now()
	aged := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return PruneResult{}, err
		}
		// In-progress publications are never pruning candidates. Check their
		// reserved prefix before metadata lookup so a writer may rename or remove
		// one concurrently without turning normal publication into a prune error.
		if strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return PruneResult{}, infoErr
		}
		if isLinkLike(info) {
			return PruneResult{}, ErrUnsafePath
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(c.root, entry.Name())
		if info.Size() > runtimeCacheMaxBytes {
			continue
		}
		file, readErr := os.Open(path)
		if readErr != nil {
			return PruneResult{}, readErr
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, runtimeCacheMaxBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return PruneResult{}, readErr
		}
		if closeErr != nil {
			return PruneResult{}, closeErr
		}
		if len(raw) > runtimeCacheMaxBytes {
			continue
		}
		var record cacheRecord
		if parseErr := parseCacheRecord(raw, &record); parseErr != nil {
			continue
		}
		if record.Key == "" || record.WorkspaceScope == "" {
			continue
		}
		if entry.Name() != record.Key+".json" {
			continue
		}
		if policy.SemanticVersion != "" && record.Descriptor.SemanticVersion != policy.SemanticVersion {
			continue
		}
		if policy.MaximumAge > 0 && now.Sub(info.ModTime()) > policy.MaximumAge {
			if err := os.Remove(path); err != nil {
				return PruneResult{}, err
			}
			aged++
			continue
		}
		eligible = append(eligible, pruneEntry{entry.Name(), path, info.ModTime()})
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].modified.Before(eligible[j].modified) })
	remove := 0
	if policy.MaximumEntries >= 0 && len(eligible) > policy.MaximumEntries {
		remove = len(eligible) - policy.MaximumEntries
	}
	for index := 0; index < remove; index++ {
		if err := os.Remove(eligible[index].path); err != nil {
			return PruneResult{}, err
		}
	}
	return PruneResult{Removed: aged + remove, Retained: len(eligible) - remove}, nil
}
