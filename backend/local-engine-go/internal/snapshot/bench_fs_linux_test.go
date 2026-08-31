//go:build linux

package snapshot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Simulated PostgreSQL-like data directory: 100 files × 1 MiB = 100 MiB total.
const benchFileCount = 100
const benchFileSize = 1 << 20 // 1 MiB

// benchWriteTestData fills dir with benchFileCount files of benchFileSize bytes.
func benchWriteTestData(b *testing.B, dir string) {
	b.Helper()
	payload := make([]byte, benchFileSize)
	for i := range payload {
		payload[i] = byte(i)
	}
	for i := 0; i < benchFileCount; i++ {
		p := filepath.Join(dir, fmt.Sprintf("data%04d.bin", i))
		if err := os.WriteFile(p, payload, 0o600); err != nil {
			b.Fatalf("benchWriteTestData: write %s: %v", p, err)
		}
	}
}

// BenchmarkZfsWarmStart measures the time to clone an existing ZFS dataset (warm start:
// state is already prepared, just clone for the next run).
// Requires BENCH_ZFS_ROOT pointing to a ZFS dataset mountpoint (e.g. /mnt/zfs-bench).
func BenchmarkZfsWarmStart(b *testing.B) {
	root := os.Getenv("BENCH_ZFS_ROOT")
	if root == "" {
		b.Skip("BENCH_ZFS_ROOT not set; skipping ZFS warm-start benchmark")
	}

	ctx := context.Background()
	mgr := zfsManager{runner: execRunner{}}
	srcDir := filepath.Join(root, "warm-src")

	if err := mgr.EnsureDataset(ctx, srcDir); err != nil {
		b.Fatalf("EnsureDataset: %v", err)
	}
	benchWriteTestData(b, srcDir)
	b.Cleanup(func() {
		_ = mgr.Destroy(context.Background(), srcDir)
		_ = os.Remove(srcDir) // ZFS leaves the mount-point dir behind after destroy
	})

	b.SetBytes(int64(benchFileCount) * benchFileSize)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		destDir := filepath.Join(root, fmt.Sprintf("warm-clone-%d", i))
		result, err := mgr.Clone(ctx, srcDir, destDir)
		if err != nil {
			b.Fatalf("Clone iteration %d: %v", i, err)
		}
		b.StopTimer()
		if err := result.Cleanup(); err != nil {
			b.Fatalf("Cleanup iteration %d: %v", i, err)
		}
		_ = os.Remove(destDir)
		b.StartTimer()
	}
}

// BenchmarkZfsColdStart measures the full cold-start pipeline on ZFS:
// create dataset → write data (simulate DB init) → snapshot.
// Requires BENCH_ZFS_ROOT.
func BenchmarkZfsColdStart(b *testing.B) {
	root := os.Getenv("BENCH_ZFS_ROOT")
	if root == "" {
		b.Skip("BENCH_ZFS_ROOT not set; skipping ZFS cold-start benchmark")
	}

	ctx := context.Background()
	mgr := zfsManager{runner: execRunner{}}

	b.SetBytes(int64(benchFileCount) * benchFileSize)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		srcDir := filepath.Join(root, fmt.Sprintf("cold-src-%d", i))
		snapDir := filepath.Join(root, fmt.Sprintf("cold-snap-%d", i))

		if err := mgr.EnsureDataset(ctx, srcDir); err != nil {
			b.Fatalf("EnsureDataset iter %d: %v", i, err)
		}
		benchWriteTestData(b, srcDir)
		if err := mgr.Snapshot(ctx, srcDir, snapDir); err != nil {
			b.Fatalf("Snapshot iter %d: %v", i, err)
		}

		b.StopTimer()
		_ = mgr.Destroy(ctx, snapDir)
		_ = os.Remove(snapDir)
		_ = mgr.Destroy(ctx, srcDir)
		_ = os.Remove(srcDir)
		b.StartTimer()
	}
}

// BenchmarkBtrfsWarmStart measures the time to clone an existing BTRFS subvolume (warm start).
// Requires BENCH_BTRFS_ROOT pointing to a BTRFS-mounted directory (e.g. /mnt/btrfs-bench).
func BenchmarkBtrfsWarmStart(b *testing.B) {
	root := os.Getenv("BENCH_BTRFS_ROOT")
	if root == "" {
		b.Skip("BENCH_BTRFS_ROOT not set; skipping BTRFS warm-start benchmark")
	}

	ctx := context.Background()
	mgr := btrfsManager{runner: execRunner{}}
	srcDir := filepath.Join(root, "warm-src")

	if err := mgr.EnsureSubvolume(ctx, srcDir); err != nil {
		b.Fatalf("EnsureSubvolume: %v", err)
	}
	benchWriteTestData(b, srcDir)
	b.Cleanup(func() {
		_ = mgr.Destroy(context.Background(), srcDir)
		_ = os.Remove(srcDir) // btrfs subvolume delete leaves the mount point dir
	})

	b.SetBytes(int64(benchFileCount) * benchFileSize)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		destDir := filepath.Join(root, fmt.Sprintf("warm-clone-%d", i))
		result, err := mgr.Clone(ctx, srcDir, destDir)
		if err != nil {
			b.Fatalf("Clone iteration %d: %v", i, err)
		}
		b.StopTimer()
		if err := result.Cleanup(); err != nil {
			b.Fatalf("Cleanup iteration %d: %v", i, err)
		}
		_ = os.Remove(destDir)
		b.StartTimer()
	}
}

// BenchmarkBtrfsColdStart measures the full cold-start pipeline on BTRFS:
// create subvolume → write data → snapshot.
// Requires BENCH_BTRFS_ROOT.
func BenchmarkBtrfsColdStart(b *testing.B) {
	root := os.Getenv("BENCH_BTRFS_ROOT")
	if root == "" {
		b.Skip("BENCH_BTRFS_ROOT not set; skipping BTRFS cold-start benchmark")
	}

	ctx := context.Background()
	mgr := btrfsManager{runner: execRunner{}}

	b.SetBytes(int64(benchFileCount) * benchFileSize)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		srcDir := filepath.Join(root, fmt.Sprintf("cold-src-%d", i))
		snapDir := filepath.Join(root, fmt.Sprintf("cold-snap-%d", i))

		if err := mgr.EnsureSubvolume(ctx, srcDir); err != nil {
			b.Fatalf("EnsureSubvolume iter %d: %v", i, err)
		}
		benchWriteTestData(b, srcDir)
		if err := mgr.Snapshot(ctx, srcDir, snapDir); err != nil {
			b.Fatalf("Snapshot iter %d: %v", i, err)
		}

		b.StopTimer()
		_ = mgr.Destroy(ctx, snapDir)
		_ = os.Remove(snapDir)
		_ = mgr.Destroy(ctx, srcDir)
		_ = os.Remove(srcDir)
		b.StartTimer()
	}
}
