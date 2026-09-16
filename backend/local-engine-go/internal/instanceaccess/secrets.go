// Package instanceaccess owns private credentials and publication fences.
// See docs/architecture/managed-database-identity-internals.md.
package instanceaccess

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrInvalid       = errors.New("invalid managed access record")
	ErrNotFound      = errors.New("managed access record not found")
	ErrUnavailable   = errors.New("managed access unavailable")
	ErrConflict      = errors.New("managed access binding conflict")
	accessRefPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
)

// SecretBinding contains only provenance, never a password or a password hash.
// Bootstrap and public-instance records use disjoint purposes and owners.
type SecretBinding struct {
	DomainRef      string
	OwnerRef       string
	IdentityDigest string
	Version        string
	Purpose        string
}

func (b SecretBinding) Validate() error {
	if !accessRefPattern.MatchString(b.DomainRef) || !accessRefPattern.MatchString(b.OwnerRef) || !accessRefPattern.MatchString(b.Version) || len(b.IdentityDigest) != 64 || strings.Trim(b.IdentityDigest, "0123456789abcdef") != "" || (b.Purpose != "bootstrap" && b.Purpose != "instance") {
		return ErrInvalid
	}
	return nil
}

// Ref derives an opaque file name solely from non-secret provenance.
func (b SecretBinding) Ref() string {
	raw, _ := json.Marshal(b)
	sum := sha256.Sum256(append([]byte("local-access-secret.v1\x00"), raw...))
	return hex.EncodeToString(sum[:])
}

// Secret is ephemeral and redacted in diagnostics/JSON. Only protected record
// encoding and authorized connection construction may expose Password.
type Secret struct {
	Password string `json:"-"`
}

func (Secret) String() string   { return "[managed access secret]" }
func (Secret) GoString() string { return "[managed access secret]" }

type secretRecord struct {
	Format   string
	Binding  SecretBinding
	Password string
}

// Secrets pins an owner-only directory outside recipe mounts. Publication uses
// a synced temporary file and a no-replace hard link, returning the race winner.
type Secrets struct {
	root secretRoot
	path string
}

// The confined filesystem boundary permits deterministic I/O-failure tests
// without weakening production fsync, no-replace publication or os.Root access.
type secretFile interface {
	io.Reader
	io.Writer
	Stat() (os.FileInfo, error)
	Sync() error
	Close() error
}
type secretRoot interface {
	Lstat(string) (os.FileInfo, error)
	Open(string) (secretFile, error)
	OpenFile(string, int, os.FileMode) (secretFile, error)
	Link(string, string) error
	Remove(string) error
	Close() error
}
type confinedSecretRoot struct{ *os.Root }

func (r confinedSecretRoot) Open(name string) (secretFile, error) { return r.Root.Open(name) }
func (r confinedSecretRoot) OpenFile(name string, flags int, mode os.FileMode) (secretFile, error) {
	return r.Root.OpenFile(name, flags, mode)
}

func OpenSecrets(path string) (*Secrets, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrInvalid
	}
	path = filepath.Clean(path)
	// Refuse symlink/reparse ancestry rather than silently moving the trust root.
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrInvalid
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	if err := createPrivateDirectory(path); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, ErrUnavailable
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalid
	}
	if err := checkPrivatePath(path, info); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Secrets{root: confinedSecretRoot{root}, path: path}, nil
}

func (s *Secrets) Close() error { return s.root.Close() }

// Resolve never generates a replacement, including after loss/corruption.
func (s *Secrets) Resolve(binding SecretBinding) (Secret, error) {
	if binding.Validate() != nil {
		return Secret{}, ErrInvalid
	}
	name := binding.Ref() + ".json"
	info, err := s.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return Secret{}, ErrNotFound
	}
	if err != nil {
		return Secret{}, ErrUnavailable
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 || checkPrivatePath(filepath.Join(s.path, name), info) != nil {
		return Secret{}, ErrInvalid
	}
	f, err := s.root.Open(name)
	if err != nil {
		return Secret{}, ErrUnavailable
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return Secret{}, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil {
		return Secret{}, ErrUnavailable
	}
	var record secretRecord
	if json.Unmarshal(data, &record) != nil || record.Format != "local-access-secret.v1" || record.Binding != binding || len(record.Password) != 64 || strings.Trim(record.Password, "0123456789abcdef") != "" {
		return Secret{}, ErrInvalid
	}
	return Secret{Password: record.Password}, nil
}

// Reserve may be used only before an intent is persisted. Existing intents must
// use Resolve, so a lost secret cannot be replaced with new credentials.
func (s *Secrets) Reserve(binding SecretBinding) (Secret, error) {
	secret, err := s.Resolve(binding)
	if err == nil {
		return secret, syncPrivateDirectory(s.root, s.path)
	}
	if !errors.Is(err, ErrNotFound) {
		return Secret{}, err
	}
	var entropy [32]byte
	_, _ = rand.Read(entropy[:])
	raw, _ := json.Marshal(secretRecord{Format: "local-access-secret.v1", Binding: binding, Password: hex.EncodeToString(entropy[:])})
	name := ".pending-" + rand.Text()
	f, err := s.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Secret{}, ErrUnavailable
	}
	defer s.root.Remove(name)
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return Secret{}, ErrUnavailable
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return Secret{}, ErrUnavailable
	}
	if err = f.Close(); err != nil {
		return Secret{}, ErrUnavailable
	}
	if err = s.root.Link(name, binding.Ref()+".json"); err != nil && !errors.Is(err, os.ErrExist) {
		return Secret{}, ErrUnavailable
	}
	secret, err = s.Resolve(binding)
	if err != nil {
		return Secret{}, err
	}
	if err := syncPrivateDirectory(s.root, s.path); err != nil {
		return Secret{}, err
	}
	return secret, nil
}
