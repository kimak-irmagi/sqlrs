package prepare

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// IC01–IC05: the cache only owns copied, compatible inventory, never a job's
// driver lease or result. See issue89-inventory-reuse-tests.md.
func inventoryCacheImage(n int) engineRuntime.RunnerImageBinding {
	digest := fmt.Sprintf("sha256:%064x", n)
	return engineRuntime.RunnerImageBinding{RequestedTag: "latest", ManifestDigest: digest,
		DigestRef: "liquibase/liquibase@" + digest,
		Platform:  engineRuntime.RunnerPlatform{OS: "linux", Architecture: "amd64"}}
}

func inventoryCacheValue() engineRuntime.RunnerImageInspection {
	return engineRuntime.RunnerImageInspection{LiquibaseVersion: "5.0.4", JavaMajor: 21,
		PostgreSQLJars: []engineRuntime.RunnerJar{{Path: "/liquibase/lib/postgresql.jar", Digest: "sha256:" + strings.Repeat("b", 64)}}}
}

func TestLiquibaseInventoryCacheCopiesAndServiceLifetime(t *testing.T) {
	for _, mounted := range []bool{false, true} {
		t.Run(fmt.Sprint(mounted), func(t *testing.T) {
			var first, second PrepareService
			value := inventoryCacheValue()
			if mounted {
				value.PostgreSQLJars = nil
			}
			want := cloneRunnerInventory(value)
			calls := 0
			load := func(context.Context) (engineRuntime.RunnerImageInspection, error) { calls++; return value, nil }
			got, err := first.inventoryCache.get(t.Context(), inventoryCacheImage(1), "1", load)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal(got, err)
			}
			if !mounted {
				got.PostgreSQLJars[0].Path = "consumer mutation"
				value.PostgreSQLJars[0].Digest = "producer mutation"
			}
			got, err = first.inventoryCache.get(t.Context(), inventoryCacheImage(1), "1", load)
			if err != nil || calls != 1 || !reflect.DeepEqual(got, want) {
				t.Fatal("hit is not an immutable copy", calls, got, err)
			}
			value = want
			if _, err := second.inventoryCache.get(t.Context(), inventoryCacheImage(1), "1", load); err != nil || calls != 2 {
				t.Fatal("another service was not cold", calls, err)
			}
		})
	}
}

func TestLiquibaseInventoryCacheExactKey(t *testing.T) {
	var cache liquibaseInventoryCache
	calls := 0
	load := func(context.Context) (engineRuntime.RunnerImageInspection, error) {
		calls++
		return inventoryCacheValue(), nil
	}
	base := inventoryCacheImage(1)
	for _, dimension := range []string{"base", "manifest", "os", "architecture", "variant", "rules"} {
		image, rules := base, "1"
		switch dimension {
		case "manifest":
			image = inventoryCacheImage(2)
		case "os":
			image.Platform.OS = "windows"
		case "architecture":
			image.Platform.Architecture = "arm64"
		case "variant":
			image.Platform.Variant = "v8"
		case "rules":
			rules = "2"
		}
		for range 2 {
			if _, err := cache.get(t.Context(), image, rules, load); err != nil {
				t.Fatal(dimension, err)
			}
		}
	}
	if calls != 6 {
		t.Fatal("key dimensions aliased", calls)
	}
	for _, dimension := range []string{"digest", "reference", "os", "architecture", "rules"} {
		image, rules := base, "1"
		switch dimension {
		case "digest":
			image.ManifestDigest = "bad"
		case "reference":
			image.DigestRef = "untrusted@" + image.ManifestDigest
		case "os":
			image.Platform.OS = ""
		case "architecture":
			image.Platform.Architecture = ""
		case "rules":
			rules = ""
		}
		if _, err := cache.get(t.Context(), image, rules, load); err == nil {
			t.Fatal("invalid identity accepted", dimension)
		}
	}
	if calls != 6 {
		t.Fatal("invalid identity reached loader", calls)
	}
}

// IC02's independent oracle must not use the production copy helper to build
// its expected JAR: doing so could let the same aliasing bug affect both sides.
func TestLiquibaseInventoryCacheIndependentOwnershipOracle(t *testing.T) {
	var cache liquibaseInventoryCache
	producer := inventoryCacheValue()
	calls := 0
	load := func(context.Context) (engineRuntime.RunnerImageInspection, error) {
		calls++
		return producer, nil
	}
	first, err := cache.get(t.Context(), inventoryCacheImage(1), "1", load)
	if err != nil {
		t.Fatal(err)
	}
	first.PostgreSQLJars[0].Path = "consumer mutation"
	producer.PostgreSQLJars[0].Digest = "producer mutation"
	for range 2 {
		got, err := cache.get(t.Context(), inventoryCacheImage(1), "1", load)
		if err != nil || len(got.PostgreSQLJars) != 1 || got.PostgreSQLJars[0].Path != "/liquibase/lib/postgresql.jar" || got.PostgreSQLJars[0].Digest != "sha256:"+strings.Repeat("b", 64) || calls != 1 {
			t.Fatal("cache ownership differs from independent identity", calls, err)
		}
		got.PostgreSQLJars[0].Digest = "later consumer mutation"
	}
}

func TestLiquibaseInventoryCacheFailuresAndCancellation(t *testing.T) {
	for _, mode := range []string{"probe", "java", "liquibase", "duplicate", "provenance", "producer-cancel"} {
		t.Run(mode, func(t *testing.T) {
			var cache liquibaseInventoryCache
			calls := 0
			denied := errors.New("private-probe-failed")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			load := func(context.Context) (engineRuntime.RunnerImageInspection, error) {
				calls++
				value := inventoryCacheValue()
				if calls > 1 {
					return value, nil
				}
				switch mode {
				case "probe":
					return value, denied
				case "java":
					value.JavaMajor = 7
				case "liquibase":
					value.LiquibaseVersion = "unknown"
				case "duplicate":
					value.PostgreSQLJars = append(value.PostgreSQLJars, value.PostgreSQLJars[0])
				case "provenance":
					value.PostgreSQLJars[0].Path = "/outside/postgresql.jar"
				case "producer-cancel":
					cancel()
				}
				return value, nil
			}
			if _, err := cache.get(ctx, inventoryCacheImage(1), "1", load); err == nil || (mode == "probe" && !errors.Is(err, denied)) || (mode == "producer-cancel" && !errors.Is(err, context.Canceled)) {
				t.Fatal("failed inventory accepted or error lost", err)
			}
			if len(cache.ready) != 0 || len(cache.flights) != 0 {
				t.Fatal("failure retained cache state")
			}
			if _, err := cache.get(t.Context(), inventoryCacheImage(1), "1", load); err != nil || calls != 2 {
				t.Fatal("independent retry failed", calls, err)
			}
			cancelled, stop := context.WithCancel(t.Context())
			stop()
			if _, err := cache.get(cancelled, inventoryCacheImage(1), "1", load); !errors.Is(err, context.Canceled) || calls != 2 {
				t.Fatal("cancelled hit succeeded", calls, err)
			}
		})
	}
}

// Done signals that get has reached its cancellable wait, not merely that a
// goroutine was scheduled. It introduces no production hook or timing sleeps.
type inventoryWaitContext struct {
	context.Context
	reached chan struct{}
	once    sync.Once
}

// Sample Err before allowing cancellation to complete. This exercises the
// publication boundary deterministically, without a product timing hook.
type inventoryPublishContext struct {
	context.Context
	checks           atomic.Int32
	sampled, release chan struct{}
}

func (c *inventoryPublishContext) Err() error {
	err := c.Context.Err()
	if c.checks.Add(1) == 2 {
		close(c.sampled)
		<-c.release
	}
	return err
}

// IC03 includes cancellation between successful probing and cache publication,
// not only cancellation inside the loader itself.
func TestLiquibaseInventoryCacheCancellationBeforePublication(t *testing.T) {
	var cache liquibaseInventoryCache
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	ctx := &inventoryPublishContext{Context: parent, sampled: make(chan struct{}), release: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := cache.get(ctx, inventoryCacheImage(1), "1", func(context.Context) (engineRuntime.RunnerImageInspection, error) { return inventoryCacheValue(), nil })
		result <- err
	}()
	<-ctx.sampled
	cancel()
	close(ctx.release)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled producer succeeded", err)
	}
	if len(cache.ready) != 0 || len(cache.flights) != 0 {
		t.Fatal("cancelled producer published an entry", len(cache.ready), len(cache.flights))
	}
}

func (c *inventoryWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.reached) })
	return c.Context.Done()
}

func TestLiquibaseInventoryCacheConcurrency(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			var cache liquibaseInventoryCache
			var calls atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			denied := errors.New("producer-denied")
			load := func(context.Context) (engineRuntime.RunnerImageInspection, error) {
				calls.Add(1)
				close(entered)
				<-release
				if fail {
					return engineRuntime.RunnerImageInspection{}, denied
				}
				return inventoryCacheValue(), nil
			}
			results := make(chan error, 2)
			go func() { _, err := cache.get(t.Context(), inventoryCacheImage(1), "1", load); results <- err }()
			<-entered
			wait := &inventoryWaitContext{Context: t.Context(), reached: make(chan struct{})}
			go func() { _, err := cache.get(wait, inventoryCacheImage(1), "1", load); results <- err }()
			<-wait.reached
			cancelCtx, cancel := context.WithCancel(t.Context())
			cancelWait := &inventoryWaitContext{Context: cancelCtx, reached: make(chan struct{})}
			cancelResult := make(chan error, 1)
			go func() { _, err := cache.get(cancelWait, inventoryCacheImage(1), "1", load); cancelResult <- err }()
			<-cancelWait.reached
			cancel()
			if err := <-cancelResult; !errors.Is(err, context.Canceled) {
				t.Error("waiter cancellation lost", err)
			}
			if _, err := cache.get(t.Context(), inventoryCacheImage(2), "1", func(context.Context) (engineRuntime.RunnerImageInspection, error) { return inventoryCacheValue(), nil }); err != nil {
				t.Error("unrelated key blocked/failed", err)
			}
			close(release)
			for range 2 {
				err := <-results
				if (fail && !errors.Is(err, denied)) || (!fail && err != nil) {
					t.Error("shared producer result differs", err)
				}
			}
			if calls.Load() != 1 {
				t.Fatal("same-key probe repeated", calls.Load())
			}
		})
	}
}

func TestLiquibaseInventoryCacheBoundedLRU(t *testing.T) {
	var cache liquibaseInventoryCache
	calls := make(map[int]int)
	get := func(n int) {
		t.Helper()
		_, err := cache.get(t.Context(), inventoryCacheImage(n), "1", func(context.Context) (engineRuntime.RunnerImageInspection, error) {
			calls[n]++
			return inventoryCacheValue(), nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for n := 1; n <= 32; n++ {
		get(n)
	}
	get(1)
	get(33)
	get(1)
	get(2)
	if calls[1] != 1 || calls[2] != 2 || len(cache.ready) != 32 || len(cache.flights) != 0 {
		t.Fatal("LRU/capacity violated", calls, len(cache.ready), len(cache.flights))
	}
}

func TestLiquibaseInventoryCacheBoundedProducers(t *testing.T) {
	var cache liquibaseInventoryCache
	entered, release := make(chan struct{}, 32), make(chan struct{})
	results := make(chan error, 32)
	for n := 1; n <= 32; n++ {
		go func(n int) {
			_, err := cache.get(t.Context(), inventoryCacheImage(n), "1", func(context.Context) (engineRuntime.RunnerImageInspection, error) {
				entered <- struct{}{}
				<-release
				return inventoryCacheValue(), nil
			})
			results <- err
		}(n)
	}
	for range 32 {
		<-entered
	}
	cache.mu.Lock()
	flights := len(cache.flights)
	cache.mu.Unlock()
	if flights != 32 {
		t.Error("producer bookkeeping is not bounded", flights)
	}
	for range 2 {
		if _, err := cache.get(t.Context(), inventoryCacheImage(33), "1", func(context.Context) (engineRuntime.RunnerImageInspection, error) { return inventoryCacheValue(), nil }); err != nil {
			t.Error("overflow introduced a failure", err)
		}
	}
	cache.mu.Lock()
	flights, ready := len(cache.flights), len(cache.ready)
	cache.mu.Unlock()
	if flights != 32 || ready != 0 {
		t.Error("overflow retained another key", flights, ready)
	}
	close(release)
	for range 32 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	if len(cache.ready) != 32 || len(cache.flights) != 0 {
		t.Fatal("completion exceeded bounds")
	}
}
