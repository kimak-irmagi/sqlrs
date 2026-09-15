package managedidentity

import (
	"bytes"
	"strings"
	"testing"
)

// Runtime evidence must bind the exact operation and physical clone; identity
// validation alone cannot authorize capture or activation (approved internals).
func TestRuntimeBindingValidation(t *testing.T) {
	identity, err := Generate(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	valid := RuntimeBinding{RuntimeRef: "container-1", PhysicalIdentity: "clone-1", IdentityBinding: identity}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*RuntimeBinding){
		func(b *RuntimeBinding) { b.RuntimeRef = "" },
		func(b *RuntimeBinding) { b.RuntimeRef = "../foreign" },
		func(b *RuntimeBinding) { b.RuntimeRef = strings.Repeat("a", 129) },
		func(b *RuntimeBinding) { b.PhysicalIdentity = "" },
		func(b *RuntimeBinding) { b.PhysicalIdentity = "foreign/clone" },
		func(b *RuntimeBinding) { b.Username = "sqlrs" },
		func(b *RuntimeBinding) { b.IdentityDigest = strings.Repeat("0", 64) },
	} {
		binding := valid
		change(&binding)
		if err := binding.Validate(); err != ErrInvalid {
			t.Fatalf("invalid binding accepted: %+v, err=%v", binding, err)
		}
	}
}

// Base keys include SQL-observable identity, while credentials are not an input.
func TestBaseKeyIncludesSelectorAndIdentity(t *testing.T) {
	identity, err := Generate(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	selector := BaseSelector{DomainRef: "store-1", EngineKind: "postgres", ImageDigest: "sha256:" + strings.Repeat("a", 64), InitSpecDigest: strings.Repeat("b", 64), PolicyVersion: PolicyVersion}
	key, err := BaseKey(selector, identity)
	if err != nil || !digestPattern.MatchString(key) {
		t.Fatalf("base key = %q, %v", key, err)
	}
	if again, err := BaseKey(selector, identity); err != nil || again != key {
		t.Fatal("base key is not stable")
	}
	for _, change := range []func(*BaseSelector){
		func(s *BaseSelector) { s.DomainRef = "store-2" },
		func(s *BaseSelector) { s.ImageDigest = "sha256:" + strings.Repeat("c", 64) },
		func(s *BaseSelector) { s.InitSpecDigest = strings.Repeat("d", 64) },
	} {
		other := selector
		change(&other)
		if got, err := BaseKey(other, identity); err != nil || got == key {
			t.Fatalf("selector change reused key: %q, %v", got, err)
		}
	}
	other, _ := Generate(bytes.NewReader(bytes.Repeat([]byte{2}, 16)))
	if got, err := BaseKey(selector, other); err != nil || got == key {
		t.Fatalf("identity change reused key: %q, %v", got, err)
	}
	if got, err := BaseKey(BaseSelector{}, identity); err != ErrInvalid || got != "" {
		t.Fatal("invalid selector accepted")
	}
	if got, err := BaseKey(selector, IdentityBinding{}); err != ErrInvalid || got != "" {
		t.Fatal("invalid identity accepted")
	}
}
