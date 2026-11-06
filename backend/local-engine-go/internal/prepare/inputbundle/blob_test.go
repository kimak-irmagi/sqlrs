package inputbundle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// C01a/C01b: helper bytes are reacquired by digest rather than from whichever
// binary a replacement worker currently runs.
func TestPublishAndReacquireHelperBlob(t *testing.T) {
	ctx := context.Background()
	store := Store{Root: t.TempDir()}
	data := []byte("verified helper bytes")
	digest, err := store.PublishBlob(ctx, data)
	if err != nil || digest != hash(data) {
		t.Fatalf("digest=%q err=%v", digest, err)
	}
	if again, err := store.PublishBlob(ctx, data); err != nil || again != digest {
		t.Fatalf("republication=%q err=%v", again, err)
	}
	read, err := store.ReadBlob(ctx, digest, 1024)
	if err != nil || string(read) != string(data) {
		t.Fatalf("reacquired=%q err=%v", read, err)
	}
	destination := filepath.Join(t.TempDir(), "helper", "capture.sh")
	if err := store.MaterializeBlob(ctx, digest, destination, 1024); err != nil {
		t.Fatal(err)
	}
	if mounted, err := os.ReadFile(destination); err != nil || string(mounted) != string(data) {
		t.Fatalf("mount source=%q err=%v", mounted, err)
	}
	if err := store.MaterializeBlob(ctx, digest, destination, 1024); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.MaterializeBlob(ctx, digest, destination, 1024); err == nil {
		t.Fatal("overwrote changed mount source")
	}
	if err := os.WriteFile(store.blobPath(digest), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadBlob(ctx, digest, 1024); err == nil {
		t.Fatal("accepted corrupt helper bytes")
	}
}
