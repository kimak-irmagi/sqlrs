package instanceaccess

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingRoot struct {
	secretRoot
	fault string
}
type failingFile struct {
	secretFile
	fault string
}

func (f failingFile) Write(p []byte) (int, error) {
	if f.fault == "write" {
		return 0, errors.New("private canary")
	}
	return f.secretFile.Write(p)
}
func (f failingFile) Read(p []byte) (int, error) {
	if f.fault == "read" {
		return 0, errors.New("private canary")
	}
	return f.secretFile.Read(p)
}
func (f failingFile) Sync() error {
	if f.fault == "sync" {
		return errors.New("private canary")
	}
	return f.secretFile.Sync()
}
func (f failingFile) Close() error {
	err := f.secretFile.Close()
	if f.fault == "close" {
		return errors.New("private canary")
	}
	return err
}
func (f failingFile) Stat() (os.FileInfo, error) {
	if f.fault == "stat" {
		return nil, errors.New("private canary")
	}
	return f.secretFile.Stat()
}
func (r failingRoot) OpenFile(name string, flags int, mode os.FileMode) (secretFile, error) {
	if r.fault == "create" {
		return nil, errors.New("private canary")
	}
	f, err := r.secretRoot.OpenFile(name, flags, mode)
	if err != nil {
		return nil, err
	}
	return failingFile{secretFile: f, fault: r.fault}, nil
}
func (r failingRoot) Open(name string) (secretFile, error) {
	if r.fault == "open" {
		return nil, errors.New("private canary")
	}
	f, err := r.secretRoot.Open(name)
	if err != nil {
		return nil, err
	}
	return failingFile{secretFile: f, fault: r.fault}, nil
}
func (r failingRoot) Link(old, new string) error {
	if r.fault == "link" {
		return errors.New("private canary")
	}
	return r.secretRoot.Link(old, new)
}

func TestSecretStorageFailuresAreBoundedAndDoNotPublish(t *testing.T) {
	for _, fault := range []string{"create", "write", "sync", "close", "link", "open", "read", "stat"} {
		t.Run(fault, func(t *testing.T) {
			secrets, err := OpenSecrets(filepath.Join(t.TempDir(), "secrets"))
			if err != nil {
				t.Fatal(err)
			}
			defer secrets.Close()
			binding := SecretBinding{DomainRef: "domain", OwnerRef: "instance", IdentityDigest: strings.Repeat("a", 64), Version: "v1", Purpose: "instance"}
			original := secrets.root
			if fault == "open" || fault == "read" || fault == "stat" {
				if _, err := secrets.Reserve(binding); err != nil {
					t.Fatal(err)
				}
			}
			secrets.root = failingRoot{secretRoot: original, fault: fault}
			if _, err := secrets.Reserve(binding); err == nil || strings.Contains(err.Error(), "canary") {
				t.Fatal("storage error not bounded", err)
			}
			secrets.root = original
			if fault == "create" || fault == "write" || fault == "sync" || fault == "close" || fault == "link" {
				if _, err := secrets.Resolve(binding); err != ErrNotFound {
					t.Fatal("failed write published credentials", err)
				}
			}
		})
	}
}

func TestSecretsRejectUnexpectedRecordAndMissingParent(t *testing.T) {
	if _, err := OpenSecrets(filepath.Join(t.TempDir(), "missing", "child")); err != ErrInvalid {
		t.Fatal(err)
	}
	s, err := OpenSecrets(filepath.Join(t.TempDir(), "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	b := SecretBinding{DomainRef: "domain", OwnerRef: "instance", IdentityDigest: strings.Repeat("a", 64), Version: "v1", Purpose: "instance"}
	if err := os.Mkdir(filepath.Join(s.path, b.Ref()+".json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(b); err != ErrInvalid {
		t.Fatal("directory record accepted", err)
	}
}
