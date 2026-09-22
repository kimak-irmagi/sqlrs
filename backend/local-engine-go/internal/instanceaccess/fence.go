package instanceaccess

import "context"

// instanceFence counts holders and waiters so unused references do not remain
// in memory. gate contains one token exactly while an operation owns the fence.
type instanceFence struct {
	gate  chan struct{}
	users int
}

// lockInstance returns a release function owned by the caller. All access
// lifecycle operations take this fence before any short metadata exclusion.
// Waiting honors cancellation; holding it cannot block another instance.
// See managed-database-identity-internals.md, atomicity and recovery.
func (s *Service) lockInstance(ctx context.Context, ref string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.fencesMu.Lock()
	if s.fences == nil {
		s.fences = make(map[string]*instanceFence)
	}
	f := s.fences[ref]
	if f == nil {
		f = &instanceFence{gate: make(chan struct{}, 1)}
		s.fences[ref] = f
	}
	f.users++
	s.fencesMu.Unlock()

	releaseReference := func() {
		s.fencesMu.Lock()
		defer s.fencesMu.Unlock()
		f.users--
		if f.users == 0 {
			delete(s.fences, ref)
		}
	}
	select {
	case f.gate <- struct{}{}:
		unlock := func() { <-f.gate; releaseReference() }
		// Cancellation and an available gate may become ready together.
		if err := ctx.Err(); err != nil {
			unlock()
			return nil, err
		}
		return unlock, nil
	case <-ctx.Done():
		releaseReference()
		return nil, ctx.Err()
	}
}
