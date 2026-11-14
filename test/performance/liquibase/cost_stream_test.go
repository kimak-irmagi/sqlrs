package benchmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func costStreamExample(root uint64) string {
	return fmt.Sprintf("sqlrs-prepare-cost-v1 begin %[1]d 2 100 1\n"+
		"sqlrs-prepare-cost-v1 span %[1]d 1 0 prepare.worker worker 0 0 100 succeeded\n"+
		"sqlrs-prepare-cost-v1 span %[1]d 2 1 capture.read step 1 10 30 failed\n"+
		"sqlrs-prepare-cost-v1 end %[1]d 2\n", root)
}

// CT10: independent literal frames, fragmented real pipes and reversed root
// completion prove that stream identity is not process start/completion order.
func TestCostStreamCompleteInterleavedAndPrivateLogs(t *testing.T) {
	first, second := strings.Split(costStreamExample(1), "\n"), strings.Split(costStreamExample(2), "\n")
	wire := first[0] + "\n" + strings.Join(second[:3], "\n") + "\n" + strings.Join(first[1:3], "\n") + "\n" + second[3] + "\n" + first[3] + "\n"
	wire = strings.Repeat("private-log-marker", 20000) + "\n" + wire + "unrelated private-log-marker"
	stream := newCostStream()
	if err := stream.consume(strings.NewReader(wire)); err != nil {
		t.Fatal("valid interleaving or unrelated long logs rejected", err)
	}
	for root := uint64(1); root <= 2; root++ {
		got, err := stream.await(t.Context(), root)
		if err != nil || got.Root != root || !got.Complete || got.ElapsedNS != 100 || len(got.Spans) != 2 ||
			got.Spans[1].Parent != 1 || got.Spans[1].Operation != "capture.read" || got.Spans[1].Step != 1 || got.Spans[1].Outcome != "failed" {
			t.Fatal("closed root differs from independent expectation", got, err)
		}
		got.Spans[0].Operation = "caller-mutation"
		again, err := stream.await(t.Context(), root)
		if err != nil || again.Spans[0].Operation != "prepare.worker" || strings.Contains(fmt.Sprint(again), "private-log-marker") {
			t.Fatal("root snapshot aliases caller or retained unrelated logs")
		}
	}
	if _, err := stream.await(t.Context(), 3); err == nil {
		t.Fatal("unknown root qualified after EOF")
	}
}

func TestCostStreamRejectsMalformedAndPartialEvidence(t *testing.T) {
	base := costStreamExample(1)
	for _, variant := range []string{"version", "unknown-frame", "root-zero", "root-overflow", "root-leading-zero", "count-overflow", "count-negative", "count-limit", "complete-flag", "elapsed-negative", "elapsed-overflow", "duplicate-begin", "duplicate-end", "foreign-span", "foreign-end", "span-before-begin", "span-id", "span-parent", "span-count", "operation", "phase", "step", "outcome", "boundary", "containment", "end-count", "missing-end", "missing-span", "unterminated", "extra-field", "oversize-cost", "incomplete"} {
		t.Run(variant, func(t *testing.T) {
			wire := base
			switch variant {
			case "version":
				wire = strings.Replace(wire, "cost-v1", "cost-v2", 1)
			case "unknown-frame":
				wire = strings.Replace(wire, "begin", "private-log-marker", 1)
			case "root-zero":
				wire = strings.Replace(wire, "begin 1", "begin 0", 1)
			case "root-overflow":
				wire = strings.Replace(wire, "begin 1", "begin 18446744073709551616", 1)
			case "root-leading-zero":
				wire = strings.Replace(wire, "begin 1", "begin 01", 1)
			case "count-overflow":
				wire = strings.Replace(wire, "begin 1 2", "begin 1 9223372036854775808", 1)
			case "count-negative":
				wire = strings.Replace(wire, "begin 1 2", "begin 1 -1", 1)
			case "count-limit":
				wire = strings.Replace(wire, "begin 1 2", "begin 1 32769", 1)
			case "complete-flag":
				wire = strings.Replace(wire, "100 1\n", "100 2\n", 1)
			case "elapsed-negative":
				wire = strings.Replace(wire, "2 100 1", "2 -1 1", 1)
			case "elapsed-overflow":
				wire = strings.Replace(wire, "2 100 1", "2 9223372036854775808 1", 1)
			case "duplicate-begin":
				wire = strings.Split(wire, "\n")[0] + "\n" + wire
			case "duplicate-end":
				wire += "sqlrs-prepare-cost-v1 end 1 2\n"
			case "foreign-span":
				wire = strings.Replace(wire, "span 1 2", "span 2 2", 1)
			case "foreign-end":
				wire = strings.Replace(wire, "end 1", "end 2", 1)
			case "span-before-begin":
				lines := strings.Split(wire, "\n")
				lines[0], lines[1] = lines[1], lines[0]
				wire = strings.Join(lines, "\n")
			case "span-id":
				wire = strings.Replace(wire, "span 1 2 1", "span 1 1 1", 1)
			case "span-parent":
				wire = strings.Replace(wire, "span 1 2 1", "span 1 2 2", 1)
			case "span-count":
				wire = strings.Replace(wire, "begin 1 2", "begin 1 1", 1)
			case "operation":
				wire = strings.Replace(wire, "capture.read", "private-log-marker", 1)
			case "phase":
				wire = strings.Replace(wire, "step 1 10", "private-log-marker 1 10", 1)
			case "step":
				wire = strings.Replace(wire, "step 1 10", "step 0 10", 1)
			case "outcome":
				wire = strings.Replace(wire, "30 failed", "30 private-log-marker", 1)
			case "boundary":
				wire = strings.Replace(wire, "10 30 failed", "10 101 failed", 1)
			case "containment":
				wire = strings.Replace(wire, "0 0 100 succeeded", "0 20 100 succeeded", 1)
			case "end-count":
				wire = strings.Replace(wire, "end 1 2", "end 1 1", 1)
			case "missing-end":
				wire = strings.Replace(wire, "sqlrs-prepare-cost-v1 end 1 2\n", "", 1)
			case "missing-span":
				wire = strings.Replace(wire, "sqlrs-prepare-cost-v1 span 1 2 1 capture.read step 1 10 30 failed\n", "", 1)
			case "unterminated":
				wire = strings.TrimSuffix(wire, "\n")
			case "extra-field":
				wire = strings.Replace(wire, "end 1 2", "end 1 2 private-log-marker", 1)
			case "oversize-cost":
				wire = "sqlrs-prepare-cost-v1 " + strings.Repeat("private-log-marker", 300) + "\n"
			case "incomplete":
				wire = "sqlrs-prepare-cost-v1 begin 1 0 100 0\nsqlrs-prepare-cost-v1 end 1 0\n"
			}
			stream := newCostStream()
			readErr := stream.consume(strings.NewReader(wire))
			got, err := stream.await(t.Context(), 1)
			if variant != "incomplete" && readErr == nil || err == nil || !reflect.DeepEqual(got, costRoot{}) || strings.Contains(err.Error(), "private-log-marker") {
				t.Fatal("malformed or partial stream qualified or leaked input", variant)
			}
		})
	}
}

func TestCostStreamWaitDeadlineReadFailureAndLateEnd(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { reader.Close(); writer.Close() })
	stream := newCostStream()
	done := make(chan error, 1)
	go func() { done <- stream.consume(reader) }()
	lines := strings.Split(costStreamExample(1), "\n")
	if _, err := io.WriteString(writer, strings.Join(lines[:3], "\n")+"\n"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := stream.await(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled waiter qualified")
	}
	deadline, stop := context.WithTimeout(t.Context(), time.Millisecond)
	defer stop()
	if _, err := stream.await(deadline, 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("missing end bypassed deadline")
	}
	if _, err := io.WriteString(writer, lines[3]+"\n"); err != nil {
		t.Fatal(err)
	}
	got, err := stream.await(t.Context(), 1)
	if err != nil || len(got.Spans) != 2 {
		t.Fatal("late but caller-bounded end lost", err)
	}
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal("reader did not finish cleanly", err)
	}
	broken := newCostStream()
	if err := broken.consume(costBrokenReader{}); err == nil || strings.Contains(err.Error(), "private-log-marker") {
		t.Fatal("private read failure exposed/ignored")
	}
	if _, err := broken.await(t.Context(), 1); err == nil {
		t.Fatal("broken reader qualified")
	}
}

type costBrokenReader struct{}

func (costBrokenReader) Read([]byte) (int, error) { return 0, errors.New("private-log-marker") }

// Build exact valid 8 MiB evidence using many empty roots. Vary only the
// canonical elapsed integer's digit count, never padding/unknown fields.
func costQuotaWire(t *testing.T) string {
	t.Helper()
	const limit = 8 << 20
	var out strings.Builder
	root := uint64(1)
	makeRoot := func(id uint64, padding int) string {
		elapsed := "0"
		if padding > 0 {
			elapsed = "1" + strings.Repeat("0", padding)
		}
		return "sqlrs-prepare-cost-v1 begin " + strconv.FormatUint(id, 10) + " 0 " + elapsed + " 1\nsqlrs-prepare-cost-v1 end " + strconv.FormatUint(id, 10) + " 0\n"
	}
	for limit-out.Len() > 3000 {
		out.WriteString(makeRoot(root, 0))
		root++
	}
	remaining := limit - out.Len()
	for count := 1; count <= 50; count++ {
		base := 0
		for i := 0; i < count; i++ {
			base += len(makeRoot(root+uint64(i), 0))
		}
		extra := remaining - base
		if extra < 0 || extra > count*18 {
			continue
		}
		for i := 0; i < count; i++ {
			padding := min(extra, 18)
			out.WriteString(makeRoot(root+uint64(i), padding))
			extra -= padding
		}
		if out.Len() != limit {
			t.Fatal("quota fixture differs from exact byte boundary")
		}
		return out.String()
	}
	t.Fatal("cannot represent quota fixture with canonical integers")
	return ""
}

func TestCostStreamExactByteQuotaAndLongUnrelatedFragments(t *testing.T) {
	wire := costQuotaWire(t)
	stream := newCostStream()
	if err := stream.consume(strings.NewReader(wire)); err != nil {
		t.Fatal("exact cost byte quota rejected", err)
	}
	if _, err := stream.await(t.Context(), 1); err != nil {
		t.Fatal("complete root at exact quota lost", err)
	}
	over := newCostStream()
	if err := over.consume(strings.NewReader(wire + "sqlrs-prepare-cost-v1 begin 999999 0 0 1\n")); err == nil {
		t.Fatal("cost byte quota overflow qualified")
	}
	if _, err := over.await(t.Context(), 1); err == nil {
		t.Fatal("global quota failure retained a qualified partial sample")
	}
	for _, size := range []int{4095, 4096, 4097, 65536} {
		s := newCostStream()
		if err := s.consume(strings.NewReader(strings.Repeat("x", size) + "\n" + costStreamExample(1))); err != nil {
			t.Fatal("unrelated long line affected cost grammar", size, err)
		}
	}
}
