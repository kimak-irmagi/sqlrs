package managedidentity

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
)

// Tests exercise the approved naming and binding requirements in
// docs/architecture/managed-database-identity-tests.md, groups 1 and 3.
func TestGenerateUsesExactly128Bits(t *testing.T) {
	input := bytes.NewReader(append(bytes.Repeat([]byte{0xab}, 16), 0xff))
	got, err := Generate(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "sqlrs_admin_"+strings.Repeat("ab", 16) {
		t.Fatalf("wrong generated name: %s", got.Username)
	}
	if input.Len() != 1 {
		t.Fatalf("entropy bytes consumed: %d", 17-input.Len())
	}
	if got.LineageRef == "" || got.EngineKind != "postgres" || got.PolicyVersion != PolicyVersion {
		t.Fatalf("invalid identity: %+v", got)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	if raw, err := hex.DecodeString(got.IdentityDigest); err != nil || len(raw) != 32 {
		t.Fatal("invalid binding digest")
	}
}

func TestGenerateRejectsUnavailableEntropy(t *testing.T) {
	for _, source := range []io.Reader{nil, bytes.NewReader(nil), bytes.NewReader(make([]byte, 15)), failingEntropy{}} {
		got, err := Generate(source)
		if !errors.Is(err, ErrUnavailable) || got != (IdentityBinding{}) {
			t.Fatalf("partial identity or missing error: %+v %v", got, err)
		}
		if strings.Contains(err.Error(), "private entropy detail") {
			t.Fatal("underlying error leaked")
		}
	}
}

type failingEntropy struct{}

func (failingEntropy) Read([]byte) (int, error) { return 0, errors.New("private entropy detail") }

func TestIdentityRejectsChangedBinding(t *testing.T) {
	good, err := Generate(bytes.NewReader(bytes.Repeat([]byte{0x12}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*IdentityBinding){
		"empty lineage":        func(b *IdentityBinding) { b.LineageRef = "" },
		"foreign lineage":      func(b *IdentityBinding) { b.LineageRef = "lineage_foreign" },
		"engine":               func(b *IdentityBinding) { b.EngineKind = "mysql" },
		"policy":               func(b *IdentityBinding) { b.PolicyVersion = "unknown" },
		"fixed postgres":       func(b *IdentityBinding) { b.Username = "postgres" },
		"fixed sqlrs":          func(b *IdentityBinding) { b.Username = "sqlrs" },
		"other valid username": func(b *IdentityBinding) { b.Username = "sqlrs_admin_" + strings.Repeat("34", 16) },
		"uppercase":            func(b *IdentityBinding) { b.Username = strings.ToUpper(b.Username) },
		"SQL punctuation":      func(b *IdentityBinding) { b.Username += "';" },
		"empty digest":         func(b *IdentityBinding) { b.IdentityDigest = "" },
		"different digest":     func(b *IdentityBinding) { b.IdentityDigest = strings.Repeat("0", 64) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			got := good
			mutate(&got)
			if !errors.Is(got.Validate(), ErrInvalid) {
				t.Fatal("changed binding accepted")
			}
		})
	}
}

func validSelector() BaseSelector {
	return BaseSelector{DomainRef: "organization-1", EngineKind: "postgres", ImageDigest: "sha256:" + strings.Repeat("a", 64), InitSpecDigest: strings.Repeat("b", 64), PolicyVersion: PolicyVersion}
}
func TestSelectorValidation(t *testing.T) {
	good := validSelector()
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*BaseSelector){
		"missing domain":     func(s *BaseSelector) { s.DomainRef = "" },
		"whitespace":         func(s *BaseSelector) { s.DomainRef = " org" },
		"oversized domain":   func(s *BaseSelector) { s.DomainRef = strings.Repeat("a", 129) },
		"path domain":        func(s *BaseSelector) { s.DomainRef = "../other" },
		"engine":             func(s *BaseSelector) { s.EngineKind = "other" },
		"image tag":          func(s *BaseSelector) { s.ImageDigest = "postgres:17" },
		"image hex":          func(s *BaseSelector) { s.ImageDigest = "sha256:" + strings.Repeat("g", 64) },
		"missing init":       func(s *BaseSelector) { s.InitSpecDigest = "" },
		"unsupported policy": func(s *BaseSelector) { s.PolicyVersion = "v0" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			value := good
			mutate(&value)
			if !errors.Is(value.Validate(), ErrInvalid) {
				t.Fatal("invalid selector accepted")
			}
		})
	}
}

func TestIdentityDigestBindsEveryField(t *testing.T) {
	good, err := Generate(bytes.NewReader(bytes.Repeat([]byte{0x42}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	if good.IdentityDigest != good.ManagedIdentity.Digest() {
		t.Fatal("unstable digest")
	}
	for _, mutate := range []func(*ManagedIdentity){
		func(i *ManagedIdentity) { i.LineageRef += "x" },
		func(i *ManagedIdentity) { i.EngineKind += "x" },
		func(i *ManagedIdentity) { i.PolicyVersion += "x" },
		func(i *ManagedIdentity) { i.Username += "x" },
	} {
		changed := good.ManagedIdentity
		mutate(&changed)
		if changed.Digest() == good.IdentityDigest {
			t.Fatal("identity field missing from digest")
		}
	}
}
