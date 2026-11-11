package benchmark

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
)

const costWirePrefix = "sqlrs-prepare-cost-v1 "
const costWireFamily = "sqlrs-prepare-cost-"
const costLineLimit = 4096
const costByteLimit = 8 << 20
const costSpanLimit = 32768

var errCostStream = errors.New("invalid or incomplete prepare cost evidence")

// costSpan mirrors the closed wire grammar, not the engine's implementation.
// No raw log text, command, ID, path or arbitrary error enters evidence.
type costSpan struct {
	ID, Parent, Step          int
	Operation, Phase, Outcome string
	StartNS, EndNS            int64
}

type costRoot struct {
	Root            uint64
	ElapsedNS       int64
	Declared        int
	Complete, Ended bool
	Spans           []costSpan
	Helpers         []costHelperObservation
	Observer        *costObserver
	Clock           *costTimelineClock
	ObserverWindow  *costTimelineWindow
	helperIndex     map[int]int
}

// Complete-mode clock data is independent of product types and reference-relative.
type costTimelineClock struct {
	Domain   string
	OriginNS int64
}

type costTimelineWindow struct{ StartNS, EndNS int64 }

const maxCostTimelineNumber = uint64(1<<53 - 1)

// costStream drains one engine's private stderr through bounded fragments.
// Its owner closes/waits the pipe during exact process cleanup. Await is used
// only after the original API timer ends, never to extend that timed interval.
// Requirements: CT10 and limited OT-CT03, cost-attribution-and-trimming design.
type costStream struct {
	mu      sync.Mutex
	roots   map[uint64]*costRoot
	changed chan struct{}
	closed  bool
	err     error
}

func newCostStream() *costStream {
	return &costStream{roots: make(map[uint64]*costRoot), changed: make(chan struct{})}
}

// consume retains only validated fields. Unrelated huge lines are discarded
// in constant-sized fragments. Broken/oversized cost data never qualify a
// partial sample, including roots whose end frame preceded a later stream fault.
func (s *costStream) consume(source io.Reader) (returned error) {
	defer func() { returned = s.finish(returned) }()
	reader := bufio.NewReaderSize(source, costLineLimit)
	dropping, accepted := false, 0
	for {
		line, err := reader.ReadSlice('\n')
		isCost := !dropping && bytes.HasPrefix(line, []byte(costWireFamily))
		if isCost {
			accepted += len(line)
			if errors.Is(err, bufio.ErrBufferFull) || accepted > costByteLimit || len(line) == 0 || line[len(line)-1] != '\n' {
				return errCostStream
			}
			if err := s.accept(string(line[:len(line)-1])); err != nil {
				return errCostStream
			}
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			dropping = true
			continue
		}
		dropping = false
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errCostStream
		}
	}
}

func (s *costStream) accept(line string) error {
	if !strings.HasPrefix(line, costWirePrefix) {
		return errCostStream
	}
	fields := strings.Split(strings.TrimPrefix(line, costWirePrefix), " ")
	if len(fields) < 2 {
		return errCostStream
	}
	id, ok := costUnsigned(fields[1], 64)
	if !ok || id == 0 {
		return errCostStream
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, exists := s.roots[id]
	switch fields[0] {
	case "begin":
		if exists || len(fields) != 5 {
			return errCostStream
		}
		count, validCount := costUnsigned(fields[2], 63)
		elapsed, validElapsed := costUnsigned(fields[3], 63)
		if !validCount || count > costSpanLimit || !validElapsed || (fields[4] != "0" && fields[4] != "1") || fields[4] == "0" && count != 0 {
			return errCostStream
		}
		s.roots[id] = &costRoot{Root: id, Declared: int(count), ElapsedNS: int64(elapsed), Complete: fields[4] == "1"}
	case "clock":
		if !exists || !root.Complete || root.Ended || root.Clock != nil || len(root.Spans) != 0 ||
			len(fields) != 4 || fields[2] != "linux-monotonic-relative-v1" {
			return errCostStream
		}
		origin, valid := costUnsigned(fields[3], 63)
		if !valid || origin > maxCostTimelineNumber || uint64(root.ElapsedNS) > maxCostTimelineNumber-origin {
			return errCostStream
		}
		root.Clock = &costTimelineClock{Domain: fields[2], OriginNS: int64(origin)}
	case "span":
		if !exists || root.Ended || len(root.Helpers) > 0 || len(fields) != 10 || len(root.Spans) >= root.Declared {
			return errCostStream
		}
		spanID, validID := costUnsigned(fields[2], 63)
		parent, validParent := costUnsigned(fields[3], 63)
		step, validStep := costUnsigned(fields[6], 63)
		start, validStart := costUnsigned(fields[7], 63)
		end, validEnd := costUnsigned(fields[8], 63)
		if !validID || spanID != uint64(len(root.Spans)+1) || !validParent || parent >= spanID || !validStep || uint64(int(step)) != step ||
			!validStart || !validEnd || end < start || end > uint64(root.ElapsedNS) || !costOperation(fields[4]) || !costPhase(fields[5], step) || !costOutcome(fields[9]) {
			return errCostStream
		}
		span := costSpan{ID: int(spanID), Parent: int(parent), Step: int(step), Operation: fields[4], Phase: fields[5], Outcome: fields[9], StartNS: int64(start), EndNS: int64(end)}
		if root.Clock != nil && len(root.Spans) == 0 && (span.Operation != "prepare.request" || span.Parent != 0 || span.Phase != "worker" || span.Step != 0) {
			return errCostStream
		}
		if parent != 0 {
			owner := root.Spans[parent-1]
			if span.StartNS < owner.StartNS || span.EndNS > owner.EndNS || owner.Step > 0 && (span.Step != owner.Step || span.Phase != owner.Phase) {
				return errCostStream
			}
		}
		root.Spans = append(root.Spans, span)
	case "helper":
		if !exists || root.Ended || len(root.Spans) != root.Declared || !root.Complete {
			return errCostStream
		}
		if err := root.acceptHelper(fields); err != nil {
			return err
		}
	case "observer":
		if !exists || !root.Ended || root.Observer != nil || len(fields) != 5 {
			return errCostStream
		}
		encode, okEncode := costUnsigned(fields[2], 63)
		wait, okWait := costUnsigned(fields[3], 63)
		write, okWrite := costUnsigned(fields[4], 63)
		if !okEncode || !okWait || !okWrite {
			return errCostStream
		}
		root.Observer = &costObserver{EncodeNS: int64(encode), LockWaitNS: int64(wait), WriteNS: int64(write)}
	case "observer-window":
		if !exists || !root.Ended || root.Clock == nil || root.Observer == nil || root.ObserverWindow != nil || len(fields) != 4 {
			return errCostStream
		}
		start, validStart := costUnsigned(fields[2], 63)
		end, validEnd := costUnsigned(fields[3], 63)
		if !validStart || !validEnd || end > maxCostTimelineNumber || end < start || start < uint64(root.Clock.OriginNS+root.ElapsedNS) {
			return errCostStream
		}
		root.ObserverWindow = &costTimelineWindow{int64(start), int64(end)}
	case "end":
		if !exists || root.Ended || len(fields) != 3 {
			return errCostStream
		}
		count, valid := costUnsigned(fields[2], 63)
		if !valid || count != uint64(root.Declared) || len(root.Spans) != root.Declared {
			return errCostStream
		}
		for _, helper := range root.Helpers {
			if helper.Available && len(helper.Detail.Regions) != len(costHelperPhases) {
				return errCostStream
			}
		}
		root.Ended = true
	default:
		return errCostStream
	}
	close(s.changed)
	s.changed = make(chan struct{})
	return nil
}

func costUnsigned(value string, bits int) (uint64, bool) {
	number, err := strconv.ParseUint(value, 10, bits)
	return number, err == nil && strconv.FormatUint(number, 10) == value
}

// Keep independent allowlists in this observer. Adding a key requires matching
// source-catalog/connection tests; neither child output nor a request extends it.
func costOperation(value string) bool {
	switch value {
	case "prepare.request", "prepare.admission":
		return true
	case "prepare.worker", "runner.inventory", "capture.read", "capture.prune", "docker.command", "docker.capture_copy":
		return true
	case "prepare.planning", "prepare.session_startup", "prepare.step":
		return true
	case "liquibase.invoke", "liquibase.plan_invoke", "db.prepare_snapshot", "db.resume.step", "db.resume.publication",
		"db.verify.after_apply", "db.verify.after_resume", "state.snapshot", "state.size", "state.seal",
		"journal.state_publish", "journal.invocation_plan", "journal.invocation_admit", "journal.invocation_complete",
		"prepare.publication", "prepare.final_cleanup":
		return true
	}
	return false
}

func costPhase(value string, step uint64) bool {
	if value == "step" {
		return step > 0
	}
	switch value {
	case "admission", "worker", "startup", "planning", "publication", "cleanup":
		return step == 0
	}
	return false
}

func costOutcome(value string) bool {
	switch value {
	case "succeeded", "failed", "cancelled", "panicked":
		return true
	}
	return false
}

func (s *costStream) finish(cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cause == nil {
		for _, root := range s.roots {
			if !root.Ended {
				cause = errCostStream
				break
			}
		}
	}
	if cause != nil {
		s.err = errCostStream
	}
	s.closed = true
	close(s.changed)
	return s.err
}

// await returns an independent closed snapshot. The caller owns its deadline
// and must also observe the consumer's final error during process teardown.
func (s *costStream) await(ctx context.Context, id uint64) (costRoot, error) {
	return s.awaitState(ctx, id, false)
}

// awaitMeasured requires the emission marker as well as end. The same caller
// deadline covers both waits after the unchanged API timer (DT05).
func (s *costStream) awaitMeasured(ctx context.Context, id uint64) (costRoot, error) {
	return s.awaitState(ctx, id, true)
}

func (s *costStream) awaitState(ctx context.Context, id uint64, measured bool) (costRoot, error) {
	return s.awaitSelectedState(ctx, id, measured, false)
}

// awaitTimeline also requires the shared origin and actual emission window.
// A legacy root/end/observer never silently qualifies the complete mode.
func (s *costStream) awaitTimeline(ctx context.Context, id uint64) (costRoot, error) {
	return s.awaitSelectedState(ctx, id, true, true)
}

func (s *costStream) awaitSelectedState(ctx context.Context, id uint64, measured, timeline bool) (costRoot, error) {
	for {
		if err := ctx.Err(); err != nil {
			return costRoot{}, err
		}
		s.mu.Lock()
		if s.err != nil {
			s.mu.Unlock()
			return costRoot{}, errCostStream
		}
		root, found := s.roots[id]
		if found && root.Ended && (!measured || root.Observer != nil) &&
			(!timeline || root.Clock != nil && root.ObserverWindow != nil) {
			if !root.Complete {
				s.mu.Unlock()
				return costRoot{}, errCostStream
			}
			copy := *root
			copy.Spans = append([]costSpan(nil), root.Spans...)
			copy.Helpers = append([]costHelperObservation(nil), root.Helpers...)
			for index := range copy.Helpers {
				copy.Helpers[index].Detail.Regions = append([]costHelperRegion(nil), root.Helpers[index].Detail.Regions...)
			}
			copy.helperIndex = nil
			if root.Observer != nil {
				observer := *root.Observer
				copy.Observer = &observer
			}
			if root.Clock != nil {
				clock := *root.Clock
				copy.Clock = &clock
			}
			if root.ObserverWindow != nil {
				window := *root.ObserverWindow
				copy.ObserverWindow = &window
			}
			s.mu.Unlock()
			return copy, nil
		}
		closed, changed := s.closed, s.changed
		s.mu.Unlock()
		if closed {
			return costRoot{}, errCostStream
		}
		select {
		case <-ctx.Done():
			return costRoot{}, ctx.Err()
		case <-changed:
		}
	}
}
