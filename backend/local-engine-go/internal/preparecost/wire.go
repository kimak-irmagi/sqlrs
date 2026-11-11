package preparecost

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	WirePrefix    = "sqlrs-prepare-cost-v1 "
	MaxWireLine   = 4096
	MaxEngineWire = 8 << 20
)

// EncodeReport builds closed numeric/enum frames after measurement finishes.
// Incomplete observations deliberately carry no spans: they cannot qualify a
// sample or disclose rejected keys. Complete reports pass the same independent
// containment/quota/overflow rules as accounting before any output is written.
// CT10: issue89-cost-attribution-and-trimming.md §3.
func EncodeReport(report Report) ([]byte, error) {
	if report.Version != 1 || report.Root == 0 || report.ElapsedNS < 0 {
		return nil, errEvidence
	}
	complete := 0
	spans := report.Spans
	helpers := report.Helpers
	if report.Complete {
		if report.Clock != nil && (report.Clock.Domain != "linux-monotonic-relative-v1" ||
			!timelineInterval(report.Clock.OriginNS, report.Clock.OriginNS) || report.ElapsedNS > maxTimelineNumber-report.Clock.OriginNS ||
			len(spans) == 0 || spans[0].ID != 1 || spans[0].Parent != 0 || spans[0].Operation != Request) {
			return nil, errEvidence
		}
		if _, err := Summarize(spans, max(1, report.ElapsedNS)); err != nil {
			return nil, errEvidence
		}
		complete = 1
		seen := make(map[int]bool, len(helpers))
		for _, helper := range helpers {
			if helper.Owner <= 0 || helper.Owner > len(spans) || seen[helper.Owner] || !validHelper(helper, spans[helper.Owner-1]) {
				return nil, errEvidence
			}
			seen[helper.Owner] = true
		}
	} else {
		spans = nil
		helpers = nil
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "%sbegin %d %d %d %d\n", WirePrefix, report.Root, len(spans), report.ElapsedNS, complete)
	if report.Complete && report.Clock != nil {
		fmt.Fprintf(&output, "%sclock %d linux-monotonic-relative-v1 %d\n", WirePrefix, report.Root, report.Clock.OriginNS)
	}
	for _, span := range spans {
		fmt.Fprintf(&output, "%sspan %d %d %d %s %s %d %d %d %s\n", WirePrefix,
			report.Root, span.ID, span.Parent, span.Operation, span.Phase, span.Step,
			span.StartNS, span.EndNS, span.Outcome)
	}
	for _, helper := range helpers {
		if !helper.Available {
			fmt.Fprintf(&output, "%shelper %d %d unavailable %d\n", WirePrefix, report.Root, helper.Owner, helper.HostElapsedNS)
			continue
		}
		for _, region := range helper.Detail.Regions {
			fmt.Fprintf(&output, "%shelper %d %d %s %d %d %d\n", WirePrefix, report.Root,
				helper.Owner, region.Phase, region.StartCS, region.EndCS, helper.HostElapsedNS)
		}
	}
	fmt.Fprintf(&output, "%send %d %d\n", WirePrefix, report.Root, len(spans))
	return output.Bytes(), nil
}

// Emitter serializes per-root frames on one private engine stream. It owns no
// execution result; callers must not replace prepare outcomes with Emit errors.
// Its receiver bounds cumulative input independently and drains unrelated logs.
type Emitter struct {
	mu     sync.Mutex
	output io.Writer
}

func NewEmitter(output io.Writer) *Emitter { return &Emitter{output: output} }

// Emit validates and buffers outside the writer lock, then writes one root's
// frames without interleaving another emission. Errors are closed diagnostics,
// never arbitrary writer text. A short write is not confirmed delivery.
func (e *Emitter) Emit(report Report) error {
	var observerStart int64
	if report.Clock != nil {
		var err error
		observerStart, err = report.clock.Read()
		if err != nil {
			return errEvidence
		}
	}
	encodingStart := time.Now()
	data, err := EncodeReport(report)
	if err != nil {
		return err
	}
	encodingNS := time.Since(encodingStart).Nanoseconds()
	lockStart := time.Now()
	e.mu.Lock()
	defer e.mu.Unlock()
	lockWaitNS := time.Since(lockStart).Nanoseconds()
	if e.output == nil {
		return errEvidence
	}
	writeStart := time.Now()
	count, err := e.output.Write(data)
	writeNS := time.Since(writeStart).Nanoseconds()
	if err != nil || count != len(data) {
		return errEvidence
	}
	// This marker cannot measure its own construction/write or Report copying.
	// It is separate, non-additive evidence, not total observer overhead (DT04).
	marker := fmt.Appendf(nil, "%sobserver %d %d %d %d\n", WirePrefix, report.Root, encodingNS, lockWaitNS, writeNS)
	if count, err := e.output.Write(marker); err != nil || count != len(marker) {
		return errEvidence
	}
	if report.Clock != nil {
		observerEnd, err := report.clock.Read()
		if err != nil || observerEnd < observerStart {
			return errEvidence
		}
		// This final frame cannot observe its own write. Its missing self-cost
		// remains explicit; the off/on control measures complete observer impact.
		window := fmt.Appendf(nil, "%sobserver-window %d %d %d\n", WirePrefix, report.Root, observerStart, observerEnd)
		if count, err := e.output.Write(window); err != nil || count != len(window) {
			return errEvidence
		}
	}
	return nil
}
