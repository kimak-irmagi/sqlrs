package preparecost

import (
	"bytes"
	"fmt"
	"io"
	"sync"
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
	if report.Complete {
		if _, err := Summarize(spans, max(1, report.ElapsedNS)); err != nil {
			return nil, errEvidence
		}
		complete = 1
	} else {
		spans = nil
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "%sbegin %d %d %d %d\n", WirePrefix, report.Root, len(spans), report.ElapsedNS, complete)
	for _, span := range spans {
		fmt.Fprintf(&output, "%sspan %d %d %d %s %s %d %d %d %s\n", WirePrefix,
			report.Root, span.ID, span.Parent, span.Operation, span.Phase, span.Step,
			span.StartNS, span.EndNS, span.Outcome)
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
	data, err := EncodeReport(report)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.output == nil {
		return errEvidence
	}
	count, err := e.output.Write(data)
	if err != nil || count != len(data) {
		return errEvidence
	}
	return nil
}
