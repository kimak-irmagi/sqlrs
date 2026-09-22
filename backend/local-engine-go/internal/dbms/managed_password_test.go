package dbms

import (
	"context"
	"strings"
	"testing"
)

func TestManagedPasswordRejectsInvalidBeforeConnection(t *testing.T) {
	request := managedAccessFixture(t)
	for _, password := range []string{"", "quote'", strings.Repeat("z", 64)} {
		if err := EnsureManagedPassword(context.Background(), request, password); err != ErrManagedAccessUnavailable {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := EnsureManagedPassword(ctx, request, strings.Repeat("c", 64)); err != context.Canceled {
		t.Fatal(err)
	}
}
