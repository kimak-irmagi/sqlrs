package resolver

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const publicationCrashPayload = "complete immutable artifact payload"

func TestPublicationCrashHelper(t *testing.T) {
	kind := os.Getenv("SQLRS_TEST_PUBLICATION_KIND")
	stage := os.Getenv("SQLRS_TEST_PUBLICATION_STAGE")
	if kind == "" || stage == "" {
		return
	}
	marker := os.Getenv("SQLRS_TEST_PUBLICATION_MARKER")
	publicationBarrier = func(gotKind string, gotStage publicationStage) {
		if gotKind != kind || string(gotStage) != stage {
			return
		}
		if err := os.WriteFile(marker, []byte("ready"), 0o600); err != nil {
			panic(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	}

	root := os.Getenv("SQLRS_TEST_PUBLICATION_ROOT")
	switch kind {
	case "cache":
		cache, err := NewDirectoryCache(root)
		if err != nil {
			t.Fatal(err)
		}
		key, resolution := publicationCrashCacheValues(t, os.Getenv("SQLRS_TEST_PUBLICATION_WORKSPACE"))
		if err := cache.Store(context.Background(), key, resolution); err != nil {
			t.Fatal(err)
		}
	case "artifact":
		store, err := NewDirectoryArtifactStore(root)
		if err != nil {
			t.Fatal(err)
		}
		artifact, err := store.PublishVerified(context.Background(), strings.NewReader(publicationCrashPayload), digestOf([]byte(publicationCrashPayload)))
		if err != nil {
			t.Fatal(err)
		}
		_ = artifact.Close()
	default:
		t.Fatalf("unknown publication kind %q", kind)
	}
}

func TestKilledWriterLeavesOnlyCompletePublications(t *testing.T) {
	for _, kind := range []string{"cache", "artifact"} {
		for _, stage := range []publicationStage{
			publicationTemporaryCreated,
			publicationContentWritten,
			publicationFileSynced,
			publicationReplaced,
			publicationDirectorySynced,
		} {
			t.Run(kind+"/"+string(stage), func(t *testing.T) {
				root := t.TempDir()
				workspace := t.TempDir()
				marker := filepath.Join(t.TempDir(), "ready")
				command := exec.Command(os.Args[0], "-test.run=^TestPublicationCrashHelper$")
				command.Env = append(os.Environ(),
					"SQLRS_TEST_PUBLICATION_KIND="+kind,
					"SQLRS_TEST_PUBLICATION_STAGE="+string(stage),
					"SQLRS_TEST_PUBLICATION_MARKER="+marker,
					"SQLRS_TEST_PUBLICATION_ROOT="+root,
					"SQLRS_TEST_PUBLICATION_WORKSPACE="+workspace,
				)
				if err := command.Start(); err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(10 * time.Second)
				for {
					if _, err := os.Stat(marker); err == nil {
						break
					} else if !errors.Is(err, os.ErrNotExist) {
						t.Fatal(err)
					}
					if time.Now().After(deadline) {
						_ = command.Process.Kill()
						_, _ = command.Process.Wait()
						t.Fatal("writer did not reach publication barrier")
					}
					time.Sleep(10 * time.Millisecond)
				}
				if err := command.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				_, _ = command.Process.Wait()

				committed := stage == publicationReplaced || stage == publicationDirectorySynced
				assertPublicationAfterCrash(t, kind, root, workspace, committed)
			})
		}
	}
}

func publicationCrashCacheValues(t *testing.T, workspace string) (CacheKey, Resolution) {
	t.Helper()
	declaration := coverageDeclaration(t)
	key, err := NewCacheKey(
		Workspace{Root: workspace},
		Descriptor{Role: "input", Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", SemanticVersion: "1"},
		NormalizedDeclaration{Declaration: declaration},
	)
	if err != nil {
		t.Fatal(err)
	}
	return key, coverageResolution(t)
}

func assertPublicationAfterCrash(t *testing.T, kind, root, workspace string, committed bool) {
	t.Helper()
	switch kind {
	case "cache":
		cache, err := NewDirectoryCache(root)
		if err != nil {
			t.Fatal(err)
		}
		key, _ := publicationCrashCacheValues(t, workspace)
		load, err := cache.Load(context.Background(), key)
		if err != nil {
			t.Fatalf("partial cache publication: %v", err)
		}
		if load.Hit != committed {
			t.Fatalf("cache hit = %v, want %v", load.Hit, committed)
		}
	case "artifact":
		target := filepath.Join(root, digestOf([]byte(publicationCrashPayload))[7:])
		raw, err := os.ReadFile(target)
		if !committed {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("uncommitted artifact exists or is unreadable: %v", err)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != publicationCrashPayload {
			t.Fatalf("partial artifact publication: %q", raw)
		}
	}
}
