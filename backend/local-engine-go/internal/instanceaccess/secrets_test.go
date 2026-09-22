package instanceaccess

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Tests the approved protected-publication contract, including durable winners
// and corruption refusal; see managed-database-identity-tests.md.
func TestProtectedSecretsPublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets")
	s, err := OpenSecrets(path)
	if err != nil {
		t.Fatal(err)
	}
	binding := SecretBinding{DomainRef: "store_test", OwnerRef: "lineage_test", IdentityDigest: strings.Repeat("a", 64), Version: "bootstrap-v1", Purpose: "bootstrap"}
	const parallel = 6
	results := make(chan Secret, parallel)
	errs := make(chan error, parallel)
	var wg sync.WaitGroup
	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); v, e := s.Reserve(binding); results <- v; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var winner Secret
	for v := range results {
		if winner.Password != "" && winner != v {
			t.Fatal("reservation changed password")
		}
		winner = v
	}
	if len(winner.Password) != 64 {
		t.Fatal("password entropy format")
	}
	raw, err := json.Marshal(winner)
	if err != nil || bytes.Contains(raw, []byte(winner.Password)) || strings.Contains(fmt.Sprintf("%+v %#v", winner, winner), winner.Password) {
		t.Fatal("secret diagnostic leak")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSecrets(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	restored, err := s.Resolve(binding)
	if err != nil || restored != winner {
		t.Fatal("secret did not survive reopen", err)
	}
	other := binding
	other.OwnerRef = "instance_other"
	other.Purpose = "instance"
	distinct, err := s.Reserve(other)
	if err != nil || distinct.Password == winner.Password {
		t.Fatal("independent owner reused password", err)
	}
	if _, err := s.Resolve(SecretBinding{}); err != ErrInvalid {
		t.Fatal("invalid binding", err)
	}
	if err := os.WriteFile(filepath.Join(path, binding.Ref()+".json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve(binding); err != ErrInvalid {
		t.Fatal("replaced corrupt secret", err)
	}
	if err := os.Remove(filepath.Join(path, other.Ref()+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(other); err != ErrNotFound {
		t.Fatal("resolve regenerated missing secret", err)
	}
}

func TestProtectedSecretsRejectPaths(t *testing.T) {
	if _, err := OpenSecrets("relative-secrets"); err != ErrInvalid {
		t.Fatal("relative root accepted", err)
	}
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSecrets(path); err == nil {
		t.Fatal("file root accepted")
	}
	b := SecretBinding{DomainRef: "store_test", OwnerRef: "../outside", IdentityDigest: strings.Repeat("a", 64), Version: "v1", Purpose: "instance"}
	if b.Validate() == nil {
		t.Fatal("traversal binding accepted")
	}
}
