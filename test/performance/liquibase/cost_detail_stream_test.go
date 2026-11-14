package benchmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func costDetailStreamExample(root uint64) string {
	base := strings.Replace(costStreamExample(root), " 2 100 1\n", " 3 100 1\n", 1)
	base = strings.TrimSuffix(base, fmt.Sprintf("sqlrs-prepare-cost-v1 end %d 2\n", root))
	base += fmt.Sprintf("sqlrs-prepare-cost-v1 span %d 3 1 liquibase.plan_invoke planning 0 0 50 succeeded\n", root)
	for _, phase := range []string{"preflight", "cli", "drain", "stream_flush", "hash_rename", "renamed_flush", "manifest_flush", "final_flush"} {
		base += fmt.Sprintf("sqlrs-prepare-cost-v1 helper %d 3 %s 0 0 0\n", root, phase)
	}
	return base + fmt.Sprintf("sqlrs-prepare-cost-v1 end %[1]d 3\nsqlrs-prepare-cost-v1 observer %[1]d 1 2 3\n", root)
}

func TestCostDetailWaitsForObserverAndOwnsSnapshots(t *testing.T) {
	stream := newCostStream()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	consumed := make(chan error, 1)
	go func() { consumed <- stream.consume(reader) }()
	base, marker, _ := strings.Cut(costDetailStreamExample(1), "sqlrs-prepare-cost-v1 observer")
	if _, err := io.WriteString(writer, base); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := stream.awaitMeasured(ctx, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("end without observer qualified measured root")
	}
	for _, fragment := range []string{"sqlrs-prepare-", "cost-v1 observer", marker} {
		if _, err := io.WriteString(writer, fragment); err != nil {
			t.Fatal(err)
		}
	}
	root, err := stream.awaitMeasured(t.Context(), 1)
	if err != nil || root.Observer == nil || root.Observer.EncodeNS != 1 || root.Observer.LockWaitNS != 2 || root.Observer.WriteNS != 3 ||
		len(root.Helpers) != 1 || root.Helpers[0].Owner != 3 || !root.Helpers[0].Available || len(root.Helpers[0].Detail.Regions) != 8 {
		t.Fatal("measured root differs from independent literal evidence", err)
	}
	root.Helpers[0].Detail.Regions[0].Phase = "private-mutation"
	root.Observer.EncodeNS = 99
	again, err := stream.awaitMeasured(t.Context(), 1)
	if err != nil || again.Observer.EncodeNS != 1 || again.Helpers[0].Detail.Regions[0].Phase != "preflight" {
		t.Fatal("snapshot aliases reader")
	}
	writer.Close()
	if err := <-consumed; err != nil {
		t.Fatal(err)
	}
}

func TestCostDetailRefusesMalformedAndPartialMetadata(t *testing.T) {
	base := costDetailStreamExample(1)
	for _, variant := range []string{"no-observer", "no-helper", "unavailable", "unknown-owner", "wrong-kind", "duplicate-phase", "phase-order", "phase-gap", "host-mismatch", "host-containment", "negative", "leading-zero", "overflow", "extra-field", "observer-early", "observer-duplicate", "observer-foreign", "helper-late", "partial", "private"} {
		t.Run(variant, func(t *testing.T) {
			wire := base
			switch variant {
			case "no-observer":
				wire, _, _ = strings.Cut(wire, "sqlrs-prepare-cost-v1 observer")
			case "no-helper":
				var lines []string
				for _, line := range strings.Split(wire, "\n") {
					if !strings.Contains(line, " helper ") {
						lines = append(lines, line)
					}
				}
				wire = strings.Join(lines, "\n")
			case "unavailable":
				begin := strings.Index(wire, "sqlrs-prepare-cost-v1 helper")
				end := strings.Index(wire, "sqlrs-prepare-cost-v1 end")
				wire = wire[:begin] + "sqlrs-prepare-cost-v1 helper 1 3 unavailable 0\n" + wire[end:]
			case "unknown-owner":
				wire = strings.Replace(wire, "helper 1 3", "helper 1 9", 1)
			case "wrong-kind":
				wire = strings.Replace(wire, "helper 1 3", "helper 1 1", 1)
			case "duplicate-phase":
				wire = strings.Replace(wire, "helper 1 3 cli", "helper 1 3 preflight", 1)
			case "phase-order":
				wire = strings.Replace(wire, "helper 1 3 preflight", "helper 1 3 cli", 1)
			case "phase-gap":
				wire = strings.Replace(wire, "cli 0 0 0", "cli 1 1 0", 1)
			case "host-mismatch":
				wire = strings.Replace(wire, "cli 0 0 0", "cli 0 0 1", 1)
			case "host-containment":
				wire = strings.ReplaceAll(wire, "0 0 0\n", "0 0 51\n")
			case "negative":
				wire = strings.Replace(wire, "observer 1 1 2 3", "observer 1 -1 2 3", 1)
			case "leading-zero":
				wire = strings.Replace(wire, "preflight 0 0", "preflight 00 0", 1)
			case "overflow":
				wire = strings.Replace(wire, "preflight 0 0", "preflight 0 922337203686", 1)
			case "extra-field":
				wire = strings.Replace(wire, "observer 1 1 2 3", "observer 1 1 2 3 4", 1)
			case "observer-early":
				wire = "sqlrs-prepare-cost-v1 observer 1 1 2 3\n" + wire
			case "observer-duplicate":
				wire += "sqlrs-prepare-cost-v1 observer 1 1 2 3\n"
			case "observer-foreign":
				wire = strings.Replace(wire, "observer 1", "observer 2", 1)
			case "helper-late":
				wire += "sqlrs-prepare-cost-v1 helper 1 3 unavailable 0\n"
			case "partial":
				wire = strings.TrimSuffix(wire, "\n")
			case "private":
				wire = strings.Replace(wire, "observer 1 1", "observer 1 private-marker", 1)
			}
			stream := newCostStream()
			err := stream.consume(strings.NewReader(wire))
			if err == nil {
				root, awaitErr := stream.awaitMeasured(t.Context(), 1)
				err = awaitErr
				if err == nil {
					err = qualifyCostPass(root, 1, 100, 1)
				}
			}
			if err == nil || strings.Contains(err.Error(), "private-marker") {
				t.Fatal("invalid metadata qualified or leaked", variant)
			}
		})
	}
}

func TestCostDetailInterleavedCompletionAndObserverOutsideAPI(t *testing.T) {
	one := strings.SplitAfter(costDetailStreamExample(1), "\n")
	two := costDetailStreamExample(2)
	wire := one[0] + two + strings.Join(one[1:], "")
	stream := newCostStream()
	if err := stream.consume(strings.NewReader(wire)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint64{2, 1} {
		root, err := stream.awaitMeasured(t.Context(), id)
		if err != nil || qualifyCostPass(root, id, 100, 1) != nil {
			t.Fatal("valid interleaving rejected", err)
		}
		root.Observer.WriteNS = 1000 // separately measured, not inside API window
		if err := qualifyCostPass(root, id, 100, 1); err != nil {
			t.Fatal("observer incorrectly constrained to API window")
		}
	}
}

// DT-C1: independently target each uncovered refusal, preserving a valid
// parsed control so an earlier unrelated fault cannot make these cases green.
func TestCostDetailAdditionalRefusalBoundaries(t *testing.T) {
	base := costDetailStreamExample(1)
	for _, variant := range []string{"arity", "duplicate-unavailable", "unknown-unavailable", "host-bound", "incomplete-phases"} {
		t.Run(variant, func(t *testing.T) {
			wire := base
			switch variant {
			case "arity":
				wire = strings.Replace(wire, "helper 1 3 preflight 0 0 0", "helper 1 3 preflight 0 0", 1)
			case "duplicate-unavailable", "unknown-unavailable":
				before, tail, _ := strings.Cut(wire, "sqlrs-prepare-cost-v1 helper")
				_, after, _ := strings.Cut(tail, "sqlrs-prepare-cost-v1 end")
				markers := "sqlrs-prepare-cost-v1 helper 1 3 unavailable 0\nsqlrs-prepare-cost-v1 helper 1 3 unavailable 0\n"
				if variant == "unknown-unavailable" {
					markers = "sqlrs-prepare-cost-v1 helper 1 3 unknown 0\n"
				}
				wire = before + markers + "sqlrs-prepare-cost-v1 end" + after
			case "host-bound":
				wire = strings.Replace(wire, "helper 1 3 preflight 0 0 0", "helper 1 3 preflight 0 2 0", 1)
			case "incomplete-phases":
				wire = strings.Replace(wire, "sqlrs-prepare-cost-v1 helper 1 3 final_flush 0 0 0\n", "", 1)
			}
			stream := newCostStream()
			if err := stream.consume(strings.NewReader(wire)); !errors.Is(err, errCostStream) {
				t.Fatal("targeted malformed detail escaped its parser boundary")
			}
			control := newCostStream()
			if err := control.consume(strings.NewReader(base)); err != nil {
				t.Fatal("independent valid control rejected", err)
			}
			if _, err := control.awaitMeasured(t.Context(), 1); err != nil {
				t.Fatal("valid measured control rejected", err)
			}
		})
	}
	stream := newCostStream()
	if err := stream.consume(strings.NewReader(base)); err != nil {
		t.Fatal(err)
	}
	root, err := stream.awaitMeasured(t.Context(), 1)
	if err != nil || qualifyDetails(root) != nil {
		t.Fatal("direct qualification control missing", err)
	}
	for _, variant := range []string{"missing", "encode", "wait", "write"} {
		copy := root
		observer := *root.Observer
		copy.Observer = &observer
		switch variant {
		case "missing":
			copy.Observer = nil
		case "encode":
			copy.Observer.EncodeNS = -1
		case "wait":
			copy.Observer.LockWaitNS = -1
		case "write":
			copy.Observer.WriteNS = -1
		}
		if err := qualifyDetails(copy); !errors.Is(err, errCostStream) {
			t.Fatal("invalid observer qualified directly", variant)
		}
	}
}
