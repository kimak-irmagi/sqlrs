package managedidentity

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestOwnerSelectionAndRecovery(t *testing.T) {
	r := testRecord(t)
	writes := 0
	service, _ := NewService(fakeStore{
		find:    func(context.Context, BaseSelector) (Record, error) { return r, nil },
		reserve: func(context.Context, Record) (Record, error) { writes++; return Record{}, ErrUnavailable },
	})
	service.entropy = failingEntropy{}
	owner, err := NewOwner(service, r.Selector.DomainRef, r.Selector.InitSpecDigest)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := owner.ResolveBase(context.Background(), r.Selector.ImageDigest)
	if err != nil || selected.IdentityBinding != r.Binding {
		t.Fatalf("selection: %+v %v", selected, err)
	}
	expectedKey, err := BaseKey(r.Selector, r.Binding)
	if err != nil || selected.Key != expectedKey {
		t.Fatal("selection key does not bind identity", err)
	}
	restored, err := owner.RestoreBase(context.Background(), r.Selector.ImageDigest, r.Binding)
	if err != nil || restored != selected || writes != 0 {
		t.Fatalf("recovery reselected base: %+v %v", restored, err)
	}
	other := r.Binding
	other.LineageRef = "other"
	other.IdentityDigest = other.Digest()
	if _, err := owner.RestoreBase(context.Background(), r.Selector.ImageDigest, other); err != ErrInvalid {
		t.Fatalf("recovery changed binding: %v", err)
	}
	other = r.Binding
	other.Username = "sqlrs_admin_" + strings.Repeat("a", 32)
	other.IdentityDigest = other.Digest()
	if _, err := owner.RestoreBase(context.Background(), r.Selector.ImageDigest, other); err != ErrInvalid {
		t.Fatalf("recovery changed administrator under same lineage: %v", err)
	}
}

func TestOwnerRecoveryNeverReservesMissingOrCorruptLineage(t *testing.T) {
	r := testRecord(t)
	for _, failure := range []error{ErrNotFound, errors.New("private storage failure"), context.Canceled} {
		service, _ := NewService(fakeStore{find: func(context.Context, BaseSelector) (Record, error) { return Record{}, failure }})
		service.entropy = failingEntropy{}
		owner, err := NewOwner(service, r.Selector.DomainRef, r.Selector.InitSpecDigest)
		if err != nil {
			t.Fatal(err)
		}
		_, err = owner.RestoreBase(context.Background(), r.Selector.ImageDigest, r.Binding)
		want := safeStoreError(failure)
		if err != want {
			t.Fatalf("restore diagnostic: %v", err)
		}
	}
	service, _ := NewService(fakeStore{find: func(context.Context, BaseSelector) (Record, error) {
		bad := r
		bad.Selector.DomainRef = "foreign"
		return bad, nil
	}})
	owner, _ := NewOwner(service, r.Selector.DomainRef, r.Selector.InitSpecDigest)
	if _, err := owner.RestoreBase(context.Background(), r.Selector.ImageDigest, r.Binding); err != ErrInvalid {
		t.Fatalf("foreign selector: %v", err)
	}
}

func TestOwnerRejectsInvalidInputsBeforeStoreAccess(t *testing.T) {
	r := testRecord(t)
	service, _ := NewService(fakeStore{})
	for _, input := range []struct {
		service      *Service
		domain, init string
	}{
		{nil, r.Selector.DomainRef, r.Selector.InitSpecDigest},
		{service, "", r.Selector.InitSpecDigest},
		{service, r.Selector.DomainRef, "bad"},
	} {
		if _, err := NewOwner(input.service, input.domain, input.init); err != ErrInvalid {
			t.Fatalf("invalid owner: %v", err)
		}
	}
	owner, _ := NewOwner(service, r.Selector.DomainRef, r.Selector.InitSpecDigest)
	for _, image := range []string{"", "postgres:17", "sha256:" + strings.Repeat("g", 64)} {
		if _, err := owner.ResolveBase(context.Background(), image); err != ErrInvalid {
			t.Fatalf("mutable image selected: %v", err)
		}
		if _, err := owner.RestoreBase(context.Background(), image, r.Binding); err != ErrInvalid {
			t.Fatalf("mutable image restored: %v", err)
		}
	}
	if _, err := owner.RestoreBase(context.Background(), r.Selector.ImageDigest, IdentityBinding{}); err != ErrInvalid {
		t.Fatalf("empty binding: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := owner.RestoreBase(ctx, r.Selector.ImageDigest, r.Binding); err != context.Canceled {
		t.Fatalf("canceled recovery: %v", err)
	}
}
