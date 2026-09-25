package resolver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type stagedCancelContext struct{ calls, allow int }

func (*stagedCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*stagedCancelContext) Done() <-chan struct{}       { return nil }
func (*stagedCancelContext) Value(any) any               { return nil }
func (c *stagedCancelContext) Err() error {
	c.calls++
	if c.calls > c.allow {
		return context.Canceled
	}
	return nil
}

func coverageKey(t *testing.T, version string) (CacheKey, Resolution) {
	t.Helper()
	root := t.TempDir()
	declaration := coverageDeclaration(t)
	key, err := NewCacheKey(Workspace{Root: root}, Descriptor{Role: "input", Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", SemanticVersion: version}, NormalizedDeclaration{Declaration: declaration})
	if err != nil {
		t.Fatal(err)
	}
	return key, coverageResolution(t)
}

func TestDirectoryCacheValidationAndCancellationBranches(t *testing.T) {
	if _, err := NewDirectoryCache(""); !errors.Is(err, ErrCorruptCache) {
		t.Fatalf("empty root: %v", err)
	}
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDirectoryCache(file); err == nil {
		t.Fatalf("file root: %v", err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(t.TempDir(), link); err == nil {
		if _, err := NewDirectoryCache(link); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("link root: %v", err)
		}
	}

	cache, err := NewDirectoryCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, value := coverageKey(t, "1")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.Load(cancelled, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel load: %v", err)
	}
	if err := cache.Store(cancelled, key, value); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel store: %v", err)
	}
	if err := cache.Store(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	if err := cache.Store(&stagedCancelContext{allow: 1}, key, value); !errors.Is(err, context.Canceled) {
		t.Fatalf("late cancellation: %v", err)
	}
	if err := cache.Store(context.Background(), key, Resolution{}); err == nil {
		t.Fatal("invalid resolution serialized")
	}
	oversized := value
	oversized.Evidence = append(append([]byte{'"'}, bytes.Repeat([]byte("x"), runtimeCacheMaxBytes+1)...), '"')
	if err := cache.Store(context.Background(), key, oversized); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("oversized store: %v", err)
	}
	if err := os.Remove(cache.path(key)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cache.path(key), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Load(context.Background(), key); err == nil || errors.Is(err, ErrCorruptCache) {
		t.Fatalf("cache I/O error = %v", err)
	}
	if err := os.Remove(cache.path(key)); err != nil {
		t.Fatal(err)
	}
	if err := cache.Store(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	blockingTarget := cache.path(key)
	if err := os.Remove(blockingTarget); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(blockingTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blockingTarget, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cache.Store(context.Background(), key, value); err == nil {
		t.Fatal("replacement of non-empty directory succeeded")
	}
	if err := os.Remove(filepath.Join(blockingTarget, "child")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(blockingTarget); err != nil {
		t.Fatal(err)
	}
	if err := cache.Store(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	missingRoot, _ := NewDirectoryCache(t.TempDir())
	missingKey, missingValue := coverageKey(t, "1")
	if err := os.Remove(missingRoot.root); err != nil {
		t.Fatal(err)
	}
	if err := missingRoot.Store(context.Background(), missingKey, missingValue); err == nil {
		t.Fatal("store in missing root succeeded")
	}

	original, err := os.ReadFile(cache.path(key))
	if err != nil {
		t.Fatal(err)
	}
	corruptions := [][]byte{
		bytes.Repeat([]byte("x"), runtimeCacheMaxBytes+1),
		[]byte(`{"schema_version":"x","schema_version":"y"}`),
		[]byte(`{"unknown":true}`),
		append(append([]byte{}, original...), []byte(` {}`)...),
		[]byte(`[`),
	}
	for index, raw := range corruptions {
		if err := os.WriteFile(cache.path(key), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := cache.Load(context.Background(), key); !errors.Is(err, ErrCorruptCache) {
			t.Errorf("corruption %d: %v", index, err)
		}
	}

	if err := cache.Store(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	_ = original
}

func TestDuplicateJSONScannerBranches(t *testing.T) {
	valid := []string{`null`, `true`, `123`, `"text"`, `[]`, `[1,{"a":2}]`, `{"a":[1,2],"b":{"c":3}}`}
	for _, raw := range valid {
		if err := rejectDuplicateJSON([]byte(raw)); err != nil {
			t.Errorf("valid %s: %v", raw, err)
		}
	}
	invalid := []string{``, `{`, `{"`, `[`, `}`, `[{"a":`, `{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `{} {}`}
	for _, raw := range invalid {
		if err := rejectDuplicateJSON([]byte(raw)); err == nil {
			t.Errorf("invalid accepted: %s", raw)
		}
	}
}

func TestDirectoryCacheRecordMismatchAndPruneBranches(t *testing.T) {
	cache, _ := NewDirectoryCache(t.TempDir())
	key, value := coverageKey(t, "1")
	if err := cache.Store(context.Background(), key, value); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(cache.path(key))
	var record cacheRecord
	if err := decodeCacheRecord(raw, &record); err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		mutate func(*cacheRecord)
		want   error
	}{
		{func(r *cacheRecord) { r.SchemaVersion = "other" }, ErrIncompatibleCache},
		{func(r *cacheRecord) { r.Key = "other" }, ErrCorruptCache},
		{func(r *cacheRecord) { r.WorkspaceScope = "other" }, ErrCorruptCache},
		{func(r *cacheRecord) { r.Descriptor.Kind = "other" }, ErrCorruptCache},
		{func(r *cacheRecord) { r.Declaration = []byte(`{}`) }, ErrCorruptCache},
		{func(r *cacheRecord) { r.Checksum = "sha256:bad" }, ErrCorruptCache},
	}
	for index, mutation := range mutations {
		candidate := record
		mutation.mutate(&candidate)
		// Preserve envelope integrity for key/scope/descriptor/declaration
		// mismatches so Load reaches its key-binding validation rather than
		// rejecting the record at the checksum boundary.
		if index >= 1 && index <= 4 {
			candidate.Checksum = ""
			candidate.Checksum, _ = cacheChecksum(candidate)
		}
		encoded, _ := jsonMarshal(candidate)
		if err := os.WriteFile(cache.path(key), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := cache.Load(context.Background(), key); !errors.Is(err, mutation.want) {
			t.Errorf("mismatch %d: %v", index, err)
		}
	}

	// Exercise age, semantic-version, count, malformed, ignored, and cancellation paths.
	cache, _ = NewDirectoryCache(t.TempDir())
	key1, value1 := coverageKey(t, "1")
	key2, value2 := coverageKey(t, "1")
	key3, value3 := coverageKey(t, "2")
	for _, item := range []struct {
		k CacheKey
		v Resolution
	}{{key1, value1}, {key2, value2}, {key3, value3}} {
		if err := cache.Store(context.Background(), item.k, item.v); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(cache.path(key1), old, old); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(cache.root, "malformed.json"), []byte("bad"), 0o600)
	_ = os.WriteFile(filepath.Join(cache.root, "oversized.json"), bytes.Repeat([]byte("x"), runtimeCacheMaxBytes+1), 0o600)
	incomplete := record
	incomplete.Key = ""
	incomplete.WorkspaceScope = ""
	incomplete.Checksum = ""
	incomplete.Checksum, _ = cacheChecksum(incomplete)
	incompleteRaw, _ := json.Marshal(incomplete)
	var parsedIncomplete cacheRecord
	if err := parseCacheRecord(incompleteRaw, &parsedIncomplete); err != nil {
		t.Fatalf("incomplete but intact prune envelope: %v", err)
	}
	_ = os.WriteFile(filepath.Join(cache.root, "incomplete.json"), incompleteRaw, 0o600)
	_ = os.WriteFile(filepath.Join(cache.root, "misnamed.json"), raw, 0o600)
	_ = os.WriteFile(filepath.Join(cache.root, "ignored.txt"), []byte("x"), 0o600)
	_ = os.Mkdir(filepath.Join(cache.root, "directory"), 0o700)
	// A temporary publication may disappear or even be a dangling link while
	// pruning scans the directory; its reserved name keeps it out of scope.
	if runtime.GOOS != "windows" {
		_ = os.Symlink(filepath.Join(cache.root, "already-renamed"), filepath.Join(cache.root, ".tmp-vanished"))
	} else {
		_ = os.WriteFile(filepath.Join(cache.root, ".tmp-active"), []byte("partial"), 0o600)
	}
	result, err := cache.Prune(context.Background(), PrunePolicy{SemanticVersion: "1", MaximumAge: time.Hour, MaximumEntries: 0})
	if err != nil || result.Removed != 2 || result.Retained != 0 {
		t.Fatalf("prune = %+v, %v", result, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.Prune(cancelled, PrunePolicy{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel prune: %v", err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(cache.root, "unsafe")
		if err := os.Symlink(t.TempDir(), link); err == nil {
			if _, err := cache.Prune(context.Background(), PrunePolicy{}); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("symlink prune: %v", err)
			}
		}
	}
	removedRoot, _ := NewDirectoryCache(t.TempDir())
	if err := os.Remove(removedRoot.root); err != nil {
		t.Fatal(err)
	}
	if _, err := removedRoot.Prune(context.Background(), PrunePolicy{}); err == nil {
		t.Fatal("missing prune root accepted")
	}

	ordering, _ := NewDirectoryCache(t.TempDir())
	first, firstValue := coverageKey(t, "1")
	second, secondValue := coverageKey(t, "1")
	if err := ordering.Store(context.Background(), first, firstValue); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := ordering.Store(context.Background(), second, secondValue); err != nil {
		t.Fatal(err)
	}
	ordered, err := ordering.Prune(context.Background(), PrunePolicy{MaximumEntries: 1})
	if err != nil || ordered.Removed != 1 || ordered.Retained != 1 {
		t.Fatalf("ordered prune: %+v %v", ordered, err)
	}
}

// Small wrappers keep the assertions above focused while still using the
// production JSON shape.
func decodeCacheRecord(raw []byte, target *cacheRecord) error { return json.Unmarshal(raw, target) }
func jsonMarshal(value any) ([]byte, error)                   { return json.Marshal(value) }

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func digestOf(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestArtifactStoreAndCopyFailureBranches(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	_ = os.WriteFile(file, []byte("x"), 0o600)
	if _, err := NewDirectoryArtifactStore(file); err == nil {
		t.Fatalf("file store: %v", err)
	}
	store, err := NewDirectoryArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishVerified(context.Background(), strings.NewReader("x"), "bad"); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("digest: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.PublishVerified(cancelled, strings.NewReader("x"), digestOf([]byte("x"))); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := store.PublishVerified(context.Background(), strings.NewReader("wrong"), digestOf([]byte("right"))); !errors.Is(err, ErrChanged) {
		t.Fatalf("mismatch: %v", err)
	}
	missingStore, _ := NewDirectoryArtifactStore(t.TempDir())
	if err := os.Remove(missingStore.root); err != nil {
		t.Fatal(err)
	}
	if _, err := missingStore.PublishVerified(context.Background(), strings.NewReader("x"), digestOf([]byte("x"))); err == nil {
		t.Fatal("publish in missing store succeeded")
	}
	finalCheck := &stagedCancelContext{allow: 2}
	if _, err := store.PublishVerified(finalCheck, strings.NewReader("late"), digestOf([]byte("late"))); !errors.Is(err, ErrChanged) {
		t.Fatalf("final verification cancellation: %v", err)
	}

	digest := digestOf([]byte("content"))
	artifact, err := store.PublishVerified(context.Background(), strings.NewReader("content"), digest)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Close()
	artifact, err = store.PublishVerified(context.Background(), strings.NewReader("ignored"), digest)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(artifact)
	artifact.Close()
	if string(got) != "content" {
		t.Fatalf("existing = %q", got)
	}
	winnerRaw := []byte("concurrent winner")
	winnerDigest := digestOf(winnerRaw)
	artifact, err = store.publishVerified(context.Background(), bytes.NewReader(winnerRaw), winnerDigest, func(source, target string) error {
		if renameErr := os.Rename(source, target); renameErr != nil {
			t.Fatalf("publish simulated winner: %v", renameErr)
		}
		return os.ErrPermission
	})
	if err != nil {
		t.Fatalf("verified concurrent winner: %v", err)
	}
	got, _ = io.ReadAll(artifact)
	artifact.Close()
	if !bytes.Equal(got, winnerRaw) {
		t.Fatalf("winner = %q", got)
	}
	failedDigest := digestOf([]byte("failed publication"))
	if _, err := store.publishVerified(context.Background(), strings.NewReader("failed publication"), failedDigest, func(string, string) error {
		return os.ErrPermission
	}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("replacement failure = %v", err)
	}
	target := filepath.Join(store.root, digest[7:])
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact, err = store.PublishVerified(context.Background(), strings.NewReader("content"), digest)
	if err != nil {
		t.Fatal(err)
	}
	artifact.Close()
	blockedDigest := digestOf([]byte("blocked"))
	blockedTarget := filepath.Join(store.root, blockedDigest[7:])
	if err := os.Mkdir(blockedTarget, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blockedTarget, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishVerified(context.Background(), strings.NewReader("blocked"), blockedDigest); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("artifact replaced non-regular object: %v", err)
	}
	if _, err := openVerifiedArtifact(context.Background(), blockedTarget, blockedDigest); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("verified directory object: %v", err)
	}
	if _, err := openVerifiedArtifact(context.Background(), filepath.Join(store.root, "missing"), blockedDigest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("verified missing object: %v", err)
	}
	corruptPath := filepath.Join(store.root, strings.Repeat("a", 64))
	if err := os.WriteFile(corruptPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openVerifiedArtifact(context.Background(), corruptPath, digestOf([]byte("expected"))); !errors.Is(err, ErrChanged) {
		t.Fatalf("verified corrupt object: %v", err)
	}

	boom := errors.New("boom")
	if _, err := hashReader(context.Background(), failingReader{boom}); !errors.Is(err, boom) {
		t.Fatalf("hash reader: %v", err)
	}
	if _, err := copyContext(context.Background(), failingWriter{boom}, strings.NewReader("x")); !errors.Is(err, boom) {
		t.Fatalf("write: %v", err)
	}
	if _, err := copyContext(context.Background(), shortWriter{}, strings.NewReader("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short: %v", err)
	}
	if _, err := copyContext(context.Background(), io.Discard, failingReader{boom}); !errors.Is(err, boom) {
		t.Fatalf("read: %v", err)
	}
	if _, err := copyContext(cancelled, io.Discard, strings.NewReader("x")); !errors.Is(err, context.Canceled) {
		t.Fatalf("copy cancel: %v", err)
	}
	for _, invalid := range []string{"", "sha256:AA" + strings.Repeat("0", 62), "sha256:" + strings.Repeat("g", 64)} {
		if validDigest(invalid) {
			t.Errorf("valid digest %q", invalid)
		}
	}
}

func TestReplaceFileInvalidAndMissingSource(t *testing.T) {
	if err := replaceFile("bad\x00source", "target"); err == nil {
		t.Fatal("NUL source accepted")
	}
	if err := replaceFile("source", "bad\x00target"); err == nil {
		t.Fatal("NUL target accepted")
	}
	if err := replaceFile(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "target")); err == nil {
		t.Fatal("missing source replaced")
	}
}
