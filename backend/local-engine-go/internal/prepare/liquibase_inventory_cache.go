package prepare

import (
	"context"
	"sync"

	"github.com/sqlrs/engine-local/internal/prepare/driverbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

const inventoryRuleRevision = "1"
const inventoryCacheCapacity = 32

type inventoryCacheKey struct {
	manifest, rules string
	platform        engineRuntime.RunnerPlatform
}

type inventoryCacheEntry struct {
	key   inventoryCacheKey
	value engineRuntime.RunnerImageInspection
}

type inventoryFlight struct {
	done  chan struct{}
	value engineRuntime.RunnerImageInspection
	err   error
}

// liquibaseInventoryCache owns only compatible copied inventory, not artifact
// leases or execution rights. The zero value is usable and each service owns
// its own cache. Ready entries are ordered least-to-most recently used; both
// ready and pending key sets are bounded independently. No lock spans a probe.
// Requirements: docs/architecture/issue89-measured-overhead-reduction.md §3 (O6).
type liquibaseInventoryCache struct {
	mu      sync.Mutex
	ready   []inventoryCacheEntry
	flights map[inventoryCacheKey]*inventoryFlight
}

// get shares one producer per retained key. Waiter cancellation never cancels
// that producer. Failures wake waiters without retrying; saturation takes the
// original uncached path instead of retaining another key or blocking it.
func (c *liquibaseInventoryCache) get(ctx context.Context, image engineRuntime.RunnerImageBinding, rules string, load func(context.Context) (engineRuntime.RunnerImageInspection, error)) (engineRuntime.RunnerImageInspection, error) {
	if err := ctx.Err(); err != nil {
		return engineRuntime.RunnerImageInspection{}, err
	}
	if !canonicalSHA256.MatchString(image.ManifestDigest) || image.DigestRef != "liquibase/liquibase@"+image.ManifestDigest || image.Platform.OS == "" || image.Platform.Architecture == "" || rules == "" {
		return engineRuntime.RunnerImageInspection{}, queue.ErrLiquibaseBinding
	}
	key := inventoryCacheKey{image.ManifestDigest, rules, image.Platform}
	c.mu.Lock()
	for i, entry := range c.ready {
		if entry.key == key {
			copy(c.ready[i:], c.ready[i+1:])
			c.ready[len(c.ready)-1] = entry
			c.mu.Unlock()
			return inventoryForCaller(ctx, entry.value, nil)
		}
	}
	if flight := c.flights[key]; flight != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return engineRuntime.RunnerImageInspection{}, ctx.Err()
		case <-flight.done:
			return inventoryForCaller(ctx, flight.value, flight.err)
		}
	}
	if len(c.flights) == inventoryCacheCapacity {
		c.mu.Unlock()
		return loadCompatibleInventory(ctx, load)
	}
	if c.flights == nil {
		c.flights = make(map[inventoryCacheKey]*inventoryFlight)
	}
	flight := &inventoryFlight{done: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	value, err := loadCompatibleInventory(ctx, load)
	c.mu.Lock()
	// Publication is the producer's linearization point. Cancellation after
	// the loader's last check must not leave a ready entry behind (IC03).
	if err == nil {
		err = ctx.Err()
	}
	delete(c.flights, key)
	flight.value, flight.err = value, err
	if err == nil {
		if len(c.ready) == inventoryCacheCapacity {
			copy(c.ready, c.ready[1:])
			c.ready = c.ready[:len(c.ready)-1]
		}
		c.ready = append(c.ready, inventoryCacheEntry{key, value})
	}
	close(flight.done)
	c.mu.Unlock()
	return inventoryForCaller(ctx, value, err)
}

// loadCompatibleInventory uses the existing driver catalog rather than a second
// compatibility policy. Copy before retaining or returning producer-owned JARs.
func loadCompatibleInventory(ctx context.Context, load func(context.Context) (engineRuntime.RunnerImageInspection, error)) (engineRuntime.RunnerImageInspection, error) {
	value, err := load(ctx)
	if err == nil {
		_, err = selectInventoryDriver(value)
	}
	return inventoryForCaller(ctx, value, err)
}

// inventoryForCaller preserves cancellation/error identity and never exposes
// a cache-owned slice, including to the producer that populated the entry.
func inventoryForCaller(ctx context.Context, value engineRuntime.RunnerImageInspection, err error) (engineRuntime.RunnerImageInspection, error) {
	if cancelled := ctx.Err(); cancelled != nil {
		return engineRuntime.RunnerImageInspection{}, cancelled
	}
	if err != nil {
		return engineRuntime.RunnerImageInspection{}, err
	}
	return cloneRunnerInventory(value), nil
}

func cloneRunnerInventory(value engineRuntime.RunnerImageInspection) engineRuntime.RunnerImageInspection {
	value.PostgreSQLJars = append([]engineRuntime.RunnerJar(nil), value.PostgreSQLJars...)
	return value
}

// selectInventoryDriver remains per binding: mounted artifact publication and
// materialisation are deliberately not retained in the inventory cache.
func selectInventoryDriver(value engineRuntime.RunnerImageInspection) (driverbundle.Binding, error) {
	catalog := driverbundle.ImageInventory{LiquibaseVersion: value.LiquibaseVersion, JavaMajor: value.JavaMajor}
	for _, jar := range value.PostgreSQLJars {
		catalog.PostgreSQLJars = append(catalog.PostgreSQLJars, driverbundle.EmbeddedDriver{Path: jar.Path, Digest: jar.Digest})
	}
	return driverbundle.Select("postgresql", catalog)
}

// inspectLiquibaseInventory observes actual lookup/wait time; the runtime emits
// runner.inventory only when the loader really performs its read-only probe.
func (m *PrepareService) inspectLiquibaseInventory(ctx context.Context, image engineRuntime.RunnerImageBinding) (engineRuntime.RunnerImageInspection, error) {
	return preparecost.Observe(ctx, preparecost.InventoryLookup, func(child context.Context) (engineRuntime.RunnerImageInspection, error) {
		return m.inventoryCache.get(child, image, inventoryRuleRevision, func(probe context.Context) (engineRuntime.RunnerImageInspection, error) {
			return m.containerLiquibase.Runtime.InspectRunnerImage(probe, image)
		})
	})
}
