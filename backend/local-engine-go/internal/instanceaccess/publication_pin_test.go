package instanceaccess

import (
	"context"
	"errors"
	"testing"
)

type publicationPinProbe struct {
	pinned bool
	err    error
}

func (p *publicationPinProbe) HasPublicationPin(_ context.Context, ref string) (bool, error) {
	return p.pinned && ref == "one", p.err
}

// LC14: pin checks happen under the existing instance fence, BEFORE stage
// changes/native callbacks. Missing/unavailable dependency fails closed when
// the finalization component is enabled. Other instances can still retire.
func TestPublicationPinPreventsRetirementBeforeEffects(t *testing.T) {
	for _, variant := range []string{"pinned", "unavailable", "missing_lookup", "released", "unrelated", "partial_component", "unknown_component", "older_component"} {
		t.Run(variant, func(t *testing.T) {
			ctx := context.Background()
			s, binding := accessFixture(t)
			if _, err := s.db.Exec(`CREATE TABLE prepare_liquibase_finalization_store_format(slot INTEGER PRIMARY KEY,format_version INTEGER); INSERT INTO prepare_liquibase_finalization_store_format VALUES(1,2)`); err != nil {
				t.Fatal(err)
			}
			probe := &publicationPinProbe{pinned: variant != "released"}
			if variant == "unavailable" {
				probe.err = errors.New("pin lookup unavailable")
			}
			if variant != "missing_lookup" {
				var err error
				s, err = NewServiceWithPublicationPins(s.db, s.secrets, s.domain, probe)
				if err != nil {
					t.Fatal(err)
				}
			}
			if variant == "partial_component" {
				if _, err := s.db.Exec(`DROP TABLE prepare_liquibase_finalization_store_format; CREATE TABLE prepare_liquibase_job_finalizations(job_id TEXT)`); err != nil {
					t.Fatal(err)
				}
			}
			if variant == "unknown_component" {
				if _, err := s.db.Exec(`UPDATE prepare_liquibase_finalization_store_format SET format_version=3`); err != nil {
					t.Fatal(err)
				}
			}
			if variant == "older_component" {
				if _, err := s.db.Exec(`UPDATE prepare_liquibase_finalization_store_format SET format_version=1`); err != nil {
					t.Fatal(err)
				}
			}
			ref := "one"
			if variant == "unrelated" {
				ref = "two"
			}
			if err := s.Activate(ctx, ref, binding, func(Secret) error { return nil }, func() error { return nil }); err != nil {
				t.Fatal(err)
			}
			effects := 0
			valid := variant == "released" || variant == "unrelated"
			if err := s.CheckRetirement(ctx, ref); (err == nil) != valid {
				t.Fatal("read-only deletion preview differs from pin authority", err)
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if err := s.CheckRetirement(cancelled, ref); !errors.Is(err, context.Canceled) {
				t.Fatal("cancelled fence wait was ignored", err)
			}
			err := s.RetireBound(ctx, ref, func(AccessBinding) error { effects++; return nil })
			if (err == nil) != valid || (effects == 1) != valid {
				t.Fatalf("retirement=%v effects=%d", err, effects)
			}
			if variant == "pinned" && !errors.Is(err, ErrPublicationPinned) {
				t.Fatal("pin rejection not distinguished", err)
			}
			_, stage, readErr := s.load(ctx, ref)
			if readErr != nil || (!valid && stage != "verified") || (valid && stage != "retired") {
				t.Fatal("pin check changed lifecycle before authorization", readErr)
			}
			if variant == "pinned" {
				probe.pinned = false
				if err := s.RetireBound(ctx, ref, func(AccessBinding) error { effects++; return nil }); err != nil || effects != 1 {
					t.Fatal("released pin could not retire", err)
				}
			}
		})
	}
}

func TestPublicationPinDependencyIsRequired(t *testing.T) {
	s, _ := accessFixture(t)
	if _, err := NewServiceWithPublicationPins(s.db, s.secrets, s.domain, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing pin lookup accepted", err)
	}
	if _, err := NewServiceWithPublicationPins(nil, s.secrets, s.domain, &publicationPinProbe{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid service accepted", err)
	}
}
