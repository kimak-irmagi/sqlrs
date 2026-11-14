package enginebin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestReadBundledJDBCForWSLInstall(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "libexec", "linux-amd64", "sqlrs-engine")
	if err := os.MkdirAll(filepath.Dir(exe), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("engine"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := readBundledJDBCWithDigest(exe, "sha256:"+hex.EncodeToString(make([]byte, 32)))
	if err != nil || data != nil {
		t.Fatalf("missing optional bundle data=%q err=%v", data, err)
	}
	jar := filepath.Join(root, "lib", JDBCDriverFileName)
	if err := os.MkdirAll(filepath.Dir(jar), 0o700); err != nil {
		t.Fatal(err)
	}
	bytes := []byte("driver fixture")
	if err := os.WriteFile(jar, bytes, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bytes)
	want := "sha256:" + hex.EncodeToString(sum[:])
	data, err = readBundledJDBCWithDigest(exe, want)
	if err != nil || string(data) != string(bytes) {
		t.Fatalf("bundle data=%q err=%v", data, err)
	}
	if _, err := readBundledJDBCWithDigest(exe, "sha256:"+hex.EncodeToString(make([]byte, 32))); err == nil {
		t.Fatal("corrupt JDBC bundle was accepted")
	}
}

// D01b: the release-adjacent JAR used by the WSL installer is the exact
// checksum-locked artifact from the standard dependency resolver. The
// container driver-delivery ADR forbids replacing missing or changed bytes.
func TestReadBundledJDBCReleaseArtifact(t *testing.T) {
	source := os.Getenv("SQLRS_JDBC_JAR_PATH")
	if source == "" {
		t.Skip("set SQLRS_JDBC_JAR_PATH to the resolved release artifact")
	}
	jar, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(jar)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != JDBCDriverDigest {
		t.Fatalf("resolved JDBC artifact digest=%s, want %s", got, JDBCDriverDigest)
	}
	root := t.TempDir()
	exe := filepath.Join(root, "libexec", "linux-amd64", "sqlrs-engine")
	if err := os.MkdirAll(filepath.Dir(exe), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("engine fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "lib", JDBCDriverFileName)
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, jar, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBundledJDBC(exe)
	if err != nil || !bytes.Equal(got, jar) {
		t.Fatalf("verified release JAR read: length=%d err=%v", len(got), err)
	}
	if err := os.WriteFile(destination, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBundledJDBC(exe); err == nil {
		t.Fatal("changed release JAR was accepted")
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadBundledJDBC(exe); err != nil || got != nil {
		t.Fatalf("missing release JAR was replaced: bytes=%d err=%v", len(got), err)
	}
}
