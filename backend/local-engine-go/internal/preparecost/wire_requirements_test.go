package preparecost

import (
	"bytes"
	"errors"
	"io"
	"math"
	"strings"
	"sync"
	"testing"
)

func wireCostReport() Report {
	return Report{Version: 1, Root: 7, ElapsedNS: 100, Complete: true, Spans: []Span{
		{ID: 1, Operation: Worker, Phase: PhaseWorker, StartNS: 0, EndNS: 100, Outcome: Succeeded},
		{ID: 2, Parent: 1, Operation: CaptureRead, Phase: PhaseStep, Step: 1, StartNS: 10, EndNS: 30, Outcome: Failed},
	}}
}

// CT10: the encoder's grammar is checked against independent literal frames,
// not its own decoder. Only fixed keys/numbers can reach private stderr.
func TestCostWireClosedFrames(t *testing.T) {
	report := wireCostReport()
	wire, err := EncodeReport(report)
	want := "sqlrs-prepare-cost-v1 begin 7 2 100 1\n" +
		"sqlrs-prepare-cost-v1 span 7 1 0 prepare.worker worker 0 0 100 succeeded\n" +
		"sqlrs-prepare-cost-v1 span 7 2 1 capture.read step 1 10 30 failed\n" +
		"sqlrs-prepare-cost-v1 end 7 2\n"
	if err != nil || string(wire) != want {
		t.Fatal("closed report differs from independent wire oracle", err)
	}
	if len(report.Spans) != 2 || report.Spans[1].Parent != 1 {
		t.Fatal("serialization mutated owned evidence")
	}
	report.Complete = false
	report.Spans[1].Operation = "private-input-marker"
	wire, err = EncodeReport(report)
	if err != nil || string(wire) != "sqlrs-prepare-cost-v1 begin 7 0 100 0\nsqlrs-prepare-cost-v1 end 7 0\n" {
		t.Fatal("incomplete evidence leaked payload or pretended completeness", err)
	}
}

func TestCostWireRejectsInvalidCompleteEvidence(t *testing.T) {
	for _, variant := range []string{"version", "root", "elapsed", "operation", "outcome", "phase", "step", "id", "parent", "boundary", "quota"} {
		t.Run(variant, func(t *testing.T) {
			report := wireCostReport()
			switch variant {
			case "version":
				report.Version = 2
			case "root":
				report.Root = 0
			case "elapsed":
				report.ElapsedNS = -1
			case "operation":
				report.Spans[1].Operation = "private-input-marker"
			case "outcome":
				report.Spans[1].Outcome = "private-input-marker"
			case "phase":
				report.Spans[1].Phase = "private-input-marker"
			case "step":
				report.Spans[1].Step = 0
			case "id":
				report.Spans[1].ID = 1
			case "parent":
				report.Spans[1].Parent = 3
			case "boundary":
				report.Spans[1].EndNS = 101
			case "quota":
				report.Spans = make([]Span, MaxSpans+1)
			}
			wire, err := EncodeReport(report)
			if err == nil || wire != nil || strings.Contains(err.Error(), "private-input-marker") {
				t.Fatal("invalid complete report emitted private/partial frames", variant)
			}
		})
	}
	zero := Report{Version: 1, Root: math.MaxUint64, Complete: true}
	if wire, err := EncodeReport(zero); err != nil || !bytes.Contains(wire, []byte("18446744073709551615")) {
		t.Fatal("empty present report or full uint64 root rejected", err)
	}
}

type costWireWriter struct{ short, refuse bool }

func (w costWireWriter) Write(data []byte) (int, error) {
	if w.refuse {
		return 0, errors.New("private-writer-marker")
	}
	if w.short {
		return len(data) - 1, nil
	}
	return len(data), nil
}

func TestCostWireWriterAndConcurrentRoots(t *testing.T) {
	for _, writer := range []io.Writer{nil, costWireWriter{short: true}, costWireWriter{refuse: true}} {
		emitter := NewEmitter(writer)
		if err := emitter.Emit(wireCostReport()); err == nil || strings.Contains(err.Error(), "private-writer-marker") {
			t.Fatal("write failure claimed complete delivery or exposed diagnostics")
		}
	}
	var buffer bytes.Buffer
	emitter := NewEmitter(&buffer)
	var workers sync.WaitGroup
	for root := uint64(1); root <= 16; root++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			report := wireCostReport()
			report.Root = root
			if err := emitter.Emit(report); err != nil {
				t.Error("complete root emission failed", err)
			}
		}()
	}
	workers.Wait()
	lines := strings.Split(strings.TrimSuffix(buffer.String(), "\n"), "\n")
	if len(lines) != 16*4 {
		t.Fatal("concurrent roots lost or interleaved frame bytes")
	}
	for index := 0; index < len(lines); index += 4 {
		begin, end := strings.Split(lines[index], " "), strings.Split(lines[index+3], " ")
		if begin[1] != "begin" || end[1] != "end" || begin[2] != end[2] {
			t.Fatal("one emission crossed another root's frames")
		}
	}
	before := buffer.String()
	invalid := wireCostReport()
	invalid.Root = 0
	if err := emitter.Emit(invalid); err == nil || buffer.String() != before {
		t.Fatal("invalid report touched the output")
	}
}
