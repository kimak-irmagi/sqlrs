package managedidentity

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	find    func(context.Context, BaseSelector) (Record, error)
	reserve func(context.Context, Record) (Record, error)
	get     func(context.Context, string, string) (Record, error)
}

func (s fakeStore) Find(ctx context.Context, q BaseSelector) (Record, error) { return s.find(ctx, q) }
func (s fakeStore) Reserve(ctx context.Context, r Record) (Record, error)    { return s.reserve(ctx, r) }
func (s fakeStore) Get(ctx context.Context, d, l string) (Record, error)     { return s.get(ctx, d, l) }
func testRecord(t *testing.T) Record {
	t.Helper()
	b, err := Generate(bytes.NewReader(bytes.Repeat([]byte{7}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	return Record{Selector: validSelector(), Binding: b, CreatedAt: time.Now().UTC()}
}
func TestServiceRequiresStore(t *testing.T) {
	if _, err := NewService(nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestServiceReusesExistingWithoutEntropy(t *testing.T) {
	record := testRecord(t)
	service, _ := NewService(fakeStore{find: func(context.Context, BaseSelector) (Record, error) { return record, nil }})
	service.entropy = failingEntropy{}
	got, err := service.ResolveOrReserveBase(context.Background(), record.Selector)
	if err != nil || got != record.Binding {
		t.Fatalf("reuse: %+v %v", got, err)
	}
}
func TestServiceReturnsReservationWinner(t *testing.T) {
	winner := testRecord(t)
	service, _ := NewService(fakeStore{
		find: func(context.Context, BaseSelector) (Record, error) { return Record{}, ErrNotFound },
		reserve: func(_ context.Context, r Record) (Record, error) {
			if err := r.Validate(); err != nil {
				t.Fatal(err)
			}
			if r.Binding == winner.Binding {
				t.Fatal("fixture must simulate a different concurrent winner")
			}
			return winner, nil
		}})
	service.entropy = bytes.NewReader(bytes.Repeat([]byte{9}, 16))
	got, err := service.ResolveOrReserveBase(context.Background(), winner.Selector)
	if err != nil || got != winner.Binding {
		t.Fatalf("winner: %+v %v", got, err)
	}
}
func TestServiceRejectsFailuresWithoutPublication(t *testing.T) {
	original := testRecord(t)
	for _, phase := range []string{"find", "reserve", "get"} {
		for _, scenario := range []string{"corrupt", "foreign", "storage", "canceled", "deadline", "missing"} {
			t.Run(phase+"/"+scenario, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				response := func() (Record, error) {
					r := original
					switch scenario {
					case "corrupt":
						r.Binding.Username = "postgres"
					case "foreign":
						r.Selector.DomainRef = "other"
					case "storage":
						return Record{}, errors.New("secret database diagnostic")
					case "canceled":
						cancel()
						return r, nil
					case "deadline":
						return Record{}, context.DeadlineExceeded
					case "missing":
						return Record{}, ErrNotFound
					}
					return r, nil
				}
				writes := 0
				service, _ := NewService(fakeStore{
					find: func(context.Context, BaseSelector) (Record, error) {
						if phase == "find" {
							return response()
						}
						return Record{}, ErrNotFound
					},
					reserve: func(context.Context, Record) (Record, error) {
						writes++
						if phase == "reserve" {
							return response()
						}
						return Record{}, errors.New("unexpected write")
					},
					get: func(context.Context, string, string) (Record, error) { return response() },
				})
				service.entropy = bytes.NewReader(bytes.Repeat([]byte{8}, 16))
				var got IdentityBinding
				var err error
				if phase == "get" {
					got, err = service.GetIdentity(ctx, original.Selector.DomainRef, original.Binding.LineageRef)
				} else {
					got, err = service.ResolveOrReserveBase(ctx, original.Selector)
				}
				if err == nil || got != (IdentityBinding{}) {
					t.Fatalf("published %+v: %v", got, err)
				}
				if strings.Contains(err.Error(), "secret") {
					t.Fatal("diagnostic leaked")
				}
				if phase == "find" && scenario != "missing" && writes != 0 {
					t.Fatal("regenerated after failed read")
				}
				if scenario == "canceled" && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if scenario == "deadline" && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestServiceGetValidatesExactReference(t *testing.T) {
	r := testRecord(t)
	service, _ := NewService(fakeStore{get: func(context.Context, string, string) (Record, error) { return r, nil }})
	got, err := service.GetIdentity(context.Background(), r.Selector.DomainRef, r.Binding.LineageRef)
	if err != nil || got != r.Binding {
		t.Fatal(err)
	}
	if _, err := service.GetIdentity(context.Background(), r.Selector.DomainRef, "other"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
func TestServiceRejectsBadInputAndCancellationBeforeIO(t *testing.T) {
	service, _ := NewService(fakeStore{})
	if _, err := service.ResolveOrReserveBase(context.Background(), BaseSelector{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	for _, refs := range [][2]string{{"", "valid"}, {"valid", "../escape"}} {
		if _, err := service.GetIdentity(context.Background(), refs[0], refs[1]); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ResolveOrReserveBase(ctx, validSelector()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := service.GetIdentity(ctx, "valid", "valid"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	service, _ = NewService(fakeStore{find: func(context.Context, BaseSelector) (Record, error) { return Record{}, ErrNotFound }})
	service.entropy = failingEntropy{}
	if _, err := service.ResolveOrReserveBase(context.Background(), validSelector()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
func TestRecordRejectsInvalidPersistentMetadata(t *testing.T) {
	good := testRecord(t)
	for _, mutate := range []func(*Record){
		func(r *Record) { r.Selector = BaseSelector{} },
		func(r *Record) { r.CreatedAt = time.Time{} },
		func(r *Record) { r.Binding.Username = "sqlrs" },
	} {
		r := good
		mutate(&r)
		if !errors.Is(r.Validate(), ErrInvalid) {
			t.Fatal("invalid record accepted")
		}
	}
}
func TestConcurrentServicesReturnOneStoreWinner(t *testing.T) {
	// This proves coordinator behavior, not database durability; real store tests
	// separately prove the unique-selector constraint and restart behavior.
	var mu sync.Mutex
	var winner Record
	adapter := fakeStore{
		find: func(context.Context, BaseSelector) (Record, error) {
			mu.Lock()
			defer mu.Unlock()
			if winner == (Record{}) {
				return Record{}, ErrNotFound
			}
			return winner, nil
		},
		reserve: func(_ context.Context, r Record) (Record, error) {
			mu.Lock()
			defer mu.Unlock()
			if winner == (Record{}) {
				winner = r
			}
			return winner, nil
		},
	}
	const count = 16
	output := make(chan IdentityBinding, count)
	failures := make(chan error, count)
	var group sync.WaitGroup
	for i := 0; i < count; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			s, _ := NewService(adapter)
			b, err := s.ResolveOrReserveBase(context.Background(), validSelector())
			output <- b
			failures <- err
		}()
	}
	group.Wait()
	close(output)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for got := range output {
		if got != winner.Binding {
			t.Fatal("multiple published identities")
		}
	}
}
