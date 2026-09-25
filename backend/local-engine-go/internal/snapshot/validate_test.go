package snapshot

import "testing"

func TestValidateStoreNoKind(t *testing.T) {
	if err := ValidateStore("", "/data"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateStoreCopyKind(t *testing.T) {
	if err := ValidateStore("copy", "/data"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateStoreBtrfsMissingRoot(t *testing.T) {
	if err := ValidateStore("btrfs", ""); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateStoreBtrfsSupported(t *testing.T) {
	prev := btrfsSupportedFn
	btrfsSupportedFn = func(string) bool { return true }
	t.Cleanup(func() { btrfsSupportedFn = prev })

	if err := ValidateStore("btrfs", "/data"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateStoreBtrfsUnsupported(t *testing.T) {
	prev := btrfsSupportedFn
	btrfsSupportedFn = func(string) bool { return false }
	t.Cleanup(func() { btrfsSupportedFn = prev })

	if err := ValidateStore("btrfs", "/data"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateStoreZfsMissingRoot(t *testing.T) {
	if err := ValidateStore("zfs", ""); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateStoreZfsSupported(t *testing.T) {
	prev := zfsSupportedFn
	zfsSupportedFn = func(string) bool { return true }
	t.Cleanup(func() { zfsSupportedFn = prev })

	if err := ValidateStore("zfs", "/data"); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestValidateStoreZfsUnsupported(t *testing.T) {
	prev := zfsSupportedFn
	zfsSupportedFn = func(string) bool { return false }
	t.Cleanup(func() { zfsSupportedFn = prev })

	if err := ValidateStore("zfs", "/data"); err == nil {
		t.Fatalf("expected error")
	}
}
