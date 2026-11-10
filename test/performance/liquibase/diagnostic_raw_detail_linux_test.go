package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

// RD01: observation labels describe an anchored region, never authorise syntax.
// Literal expected tuples cover the closed classes independently of the parser.
func TestDiagnosticRawDetailShape(t *testing.T) {
	for _, row := range []struct{ body, family, count, kind string }{
		{"???( <unfinished ...>", "fallback", "empty", "none"},
		{"syscall_0xabc(0 <unfinished ...>", "numeric", "short", "numeric"},
		{"syscall_0xabc(0,0,0,0,0) = 0", "numeric", "short", "numeric"},
		{"syscall_0xffffffffffffffff(0,0x1,0xffffffffffffffff,0,0,0) = 0", "numeric", "six", "numeric"},
		{"syscall_0xabc(0,0,0,0,0,0,0 <unfinished ...>", "numeric", "extra", "numeric"},
		{"???(" + strings.Repeat("0,", 20) + "0) = 0", "fallback", "extra", "numeric"},
		{"syscall_0xabc(123456789 <unfinished ...>", "numeric", "short", "other"},
		{"???(0,PRIVATE_SENTINEL,0,0,0,0) = 0", "fallback", "six", "other"},
		{"???(<unavailable> <unfinished ...>", "fallback", "none", "unavailable"},
		{"???(<unavailable_PRIVATE_SENTINEL>) = 0", "fallback", "short", "other"},
		{"syscall_0xABC(0) = 0", "other", "short", "numeric"},
		{"syscall_0x10000000000000000(0) = 0", "other", "short", "numeric"},
		{"PRIVATE_SENTINEL(0) = 0", "other", "short", "numeric"},
		{"???(\t0 , 0 ,0,0,0,0\t) = 0", "fallback", "six", "numeric"},
		{`???("PRIVATE_SENTINEL,0",0,0,0,0) = 0`, "fallback", "none", "none"},
		{"???(nested(0),0,0,0,0,0) = 0", "fallback", "none", "none"},
		{"???([0,0],0,0,0,0) = 0", "fallback", "none", "none"},
		{"???(0,0", "fallback", "none", "none"},
		{`"PRIVATE_SENTINEL ???(0,0,0,0,0,0) = 0"`, "none", "none", "none"},
	} {
		got := describeDiagnosticRawFailure(row.body, "other")
		stage := "other"
		if row.family == "none" {
			stage = "none"
		}
		want := diagnosticRawFailure{row.family, row.count, row.kind, stage}
		if got != want {
			t.Fatal("raw shape differs from independent expectation")
		}
		assertRawDetailPrivate(t, got)
	}
}

// RD01/RD03: these are actual parser failures, not stage inferred from a label.
func TestDiagnosticRawDetailParserStages(t *testing.T) {
	for _, row := range []struct {
		body, stage string
		code        int
	}{
		{"syscall_0xABC(0,0,0,0,0,0) = 0", "name", 8},
		{"???(0,0", "framing", 8},
		{"???( <unfinished ...>", "arity", 8},
		{"???(0,0,0,0,0,PRIVATE_SENTINEL) = 0", "argument", 8},
		{"???(0,0,0,0,0,0) = 1", "result", 9},
		{"???(0,0,0,0,0,0) = 0 <0..1>", "result", 9},
		{"???(0,0,0,0,0,0) = 0 <9223372036.854775808>", "duration", 9},
	} {
		_, err := parseDiagnosticRaw(row.body)
		if diagnosticDecodeCode(err) != row.code || err.Error() != "incomplete diagnostic trace" {
			t.Fatal("raw checkpoint changed")
		}
		for _, candidate := range []error{err, fmt.Errorf("outer: %w", err)} {
			var original diagnosticDecodeError
			if !errors.As(candidate, &original) || diagnosticErrorRaw(candidate).RejectStage != row.stage {
				t.Fatal("real parser stage lost")
			}
			assertRawDetailPrivate(t, diagnosticErrorRaw(candidate))
			if strings.Contains(fmt.Sprintf("%+v", candidate), "PRIVATE_SENTINEL") {
				t.Fatal("parser error retained private input")
			}
		}
	}
	_, err := parseDiagnosticRaw("???(0,0,0,0,0,0) = 0")
	if err != nil || diagnosticErrorRaw(err) != (diagnosticRawFailure{"none", "none", "none", "none"}) {
		t.Fatal("valid grammar acquired failure metadata")
	}
	// The wrapper owns only cause and enum value, not another raw-string field.
	typ := reflect.TypeOf(diagnosticRawFailureError{})
	if typ.NumField() != 2 {
		t.Fatal("unexpected retained error state")
	}
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type.Kind() == reflect.String {
			t.Fatal("raw text retained directly")
		}
	}
}

func assertRawDetailPrivate(t *testing.T, value diagnosticRawFailure) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"PRIVATE_SENTINEL", "123456789", "ffffffffffffffff", "syscall_0x"} {
		if strings.Contains(string(data), private) {
			t.Fatal("raw observation leaked input")
		}
	}
}

// RD02: the first parser failure survives trimming and interleaved PID state.
func TestDiagnosticRawDetailPending(t *testing.T) {
	for _, row := range []struct{ body, family, count, kind string }{
		{"???(", "fallback", "empty", "none"},
		{"syscall_0xabc(0", "numeric", "short", "numeric"},
		{"syscall_0xabc(123456789", "numeric", "short", "other"},
		{"???(<unavailable>", "fallback", "none", "unavailable"},
		{"???(0,0,0,0,0,0,0", "fallback", "extra", "numeric"},
	} {
		for _, terminal := range []string{"+++ exited with 0 +++", "+++ exited with 1 +++", "+++ killed by SIGTERM +++"} {
			raw := "2 100.1 " + row.body + " <unfinished ...>\n2 100.3 " + terminal + "\n"
			rows, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
			want := diagnosticRawFailure{row.family, row.count, row.kind, "arity"}
			if rows != nil || diagnosticDecodeCode(err) != 8 || diagnosticErrorContext(err) != (diagnosticFailureContext{"pending_entry", "other"}) || diagnosticErrorPrefix(err) != (diagnosticPrefixContext{"unknown_syscall", "other"}) || diagnosticErrorRaw(err) != want {
				t.Fatal("original pending failure was replaced")
			}
			if _, err := decodeDiagnosticTrace(strings.NewReader(raw), 1); diagnosticDecodeCode(err) != 5 || diagnosticErrorRaw(err) != (diagnosticRawFailure{"none", "none", "none", "none"}) {
				t.Fatal("strict pending behaviour changed")
			}
		}
	}
	raw := "2 100.1 ???( <unfinished ...>\n3 100.2 syscall_0xabc(0 <unfinished ...>\n3 100.3 +++ exited with 0 +++\n2 100.4 +++ exited with 0 +++\n"
	_, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
	if diagnosticErrorRaw(err) != (diagnosticRawFailure{"numeric", "short", "numeric", "arity"}) {
		t.Fatal("another PID's metadata returned")
	}
	_, err = decodeMeasuredDiagnosticTrace(strings.NewReader("2 100.1 "+rawTestEntry+" <unfinished ...>\n2 100.2 <... syscall_0xabc resumed>) = 1\n"+rawTestExit), 1)
	if diagnosticErrorRaw(err).RejectStage != "result" {
		t.Fatal("matched resume parser stage lost")
	}
	for _, raw := range []string{
		"2 100.1 ???( <unfinished ...>\n",
		"2 100.1 ???(0,0,0,0,0,0) = 0\r\n" + rawTestExit,
		"2 100.1 ???(0,0,0,0,0,0) = 0\n2 100.0 +++ exited with 0 +++\n",
		"2 100.2 <... ??? resumed>) = 0\n" + rawTestExit,
		"2 100.1 ???(0,0,0,0,0,0 <unfinished ...>\n3 100.2 <... ??? resumed>) = 0\n" + rawTestExit,
		"2 100.1 ???(0,0,0,0,0,0 <unfinished ...>\n2 100.2 <... syscall_0xabc resumed>) = 0\n" + rawTestExit,
		strings.Repeat("x", 65537),
	} {
		rows, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
		if err == nil || rows != nil || diagnosticErrorRaw(err) != (diagnosticRawFailure{"none", "none", "none", "none"}) {
			t.Fatal("lifecycle/framing failure invented raw evidence")
		}
	}
	if _, err := decodeMeasuredDiagnosticTrace(helperFailingReader{errors.New("PRIVATE_SENTINEL")}, 1); diagnosticErrorRaw(err) != (diagnosticRawFailure{"none", "none", "none", "none"}) {
		t.Fatal("reader failure acquired raw evidence")
	}
}

// RD03: coherent enum combinations are validated at the safe Go accessor.
func TestDiagnosticRawDetailUnavailable(t *testing.T) {
	unavailable := diagnosticRawFailure{"none", "none", "none", "none"}
	pairs := [][2]string{{"empty", "none"}, {"none", "none"}, {"none", "unavailable"}, {"short", "numeric"}, {"short", "other"}, {"six", "numeric"}, {"six", "other"}, {"extra", "numeric"}, {"extra", "other"}}
	for _, family := range []string{"numeric", "fallback", "other"} {
		for _, stage := range []string{"name", "framing", "arity", "argument", "result", "duration", "other"} {
			for _, pair := range pairs {
				want := diagnosticRawFailure{family, pair[0], pair[1], stage}
				if got := diagnosticErrorRaw(diagnosticRawFailureError{cause: diagnosticDecodeError{8}, detail: want}); got != want {
					t.Fatal("valid metadata tuple unavailable")
				}
			}
		}
	}
	for _, bad := range []diagnosticRawFailure{
		{}, unavailable, {"PRIVATE_SENTINEL", "six", "numeric", "argument"}, {"numeric", "PRIVATE_SENTINEL", "numeric", "argument"},
		{"numeric", "six", "PRIVATE_SENTINEL", "argument"}, {"numeric", "six", "numeric", "PRIVATE_SENTINEL"},
		{"numeric", "empty", "numeric", "arity"}, {"numeric", "six", "none", "argument"}, {"numeric", "short", "unavailable", "argument"},
		{"none", "six", "numeric", "arity"}, {"numeric", "six", "numeric", "none"}, {"numeric", "none", "numeric", "framing"},
	} {
		if got := diagnosticErrorRaw(diagnosticRawFailureError{cause: diagnosticDecodeError{8}, detail: bad}); got != unavailable {
			t.Fatal("invalid metadata tuple trusted")
		}
	}
	for _, err := range []error{nil, errors.New("PRIVATE_SENTINEL"), diagnosticDecodeError{8}, diagnosticContextError{cause: diagnosticDecodeError{8}, shape: "other"}} {
		if got := diagnosticErrorRaw(err); got != unavailable {
			t.Fatal("legacy/foreign error acquired detail")
		}
	}
}

// RD05: real private acquisition and an independently specified marker line.
func TestDiagnosticRawDetailPrivatePipe(t *testing.T) {
	for _, mode := range []string{"measured", "strict", "missing_exit"} {
		trace, err := newDiagnosticTrace(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		fd, err := syscall.Dup(int(trace.writer.Fd()))
		if err != nil {
			trace.abort()
			t.Fatal(err)
		}
		writer := os.NewFile(uintptr(fd), "private-test-writer")
		trace.begin(0)
		raw := "2 100.1 ???( <unfinished ...>\n"
		if mode != "missing_exit" {
			raw += rawTestExit
		}
		_, err = io.WriteString(writer, raw)
		writer.Close()
		if err != nil {
			trace.abort()
			t.Fatal("private fixture write failed")
		}
		var rows []diagnosticInterval
		if mode == "strict" {
			rows, err = trace.finish(1)
		} else {
			rows, err = trace.finishMeasured(1)
		}
		if err == nil || rows != nil {
			t.Fatal("invalid raw evidence accepted")
		}
		detail := diagnosticErrorRaw(err)
		line := fmt.Sprintf("P01B_DIAGNOSTIC_RAW_CONTEXT %s %s %s %s", detail.NameFamily, detail.ArgumentCount, detail.ArgumentKind, detail.RejectStage)
		want := "P01B_DIAGNOSTIC_RAW_CONTEXT none none none none"
		if mode == "measured" {
			want = "P01B_DIAGNOSTIC_RAW_CONTEXT fallback empty none arity"
		}
		if line != want {
			t.Fatal("private transport/marker boundary differs")
		}
	}
}

// RD06: preserve the old literal guards; the actual errors/pipe above and native
// RD07 prove behaviour instead of treating this source check as a workload test.
func TestDiagnosticRawDetailWiring(t *testing.T) {
	data, err := os.ReadFile("collector_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, `if prefix.Prefix == "unknown_syscall" {`)
	if start < 0 {
		t.Fatal("raw metadata lacks unknown-call guard")
	}
	end := strings.Index(source[start:], "\n\t\t\t\t\t}")
	if end < 0 {
		t.Fatal("raw metadata guard boundary missing")
	}
	branch := source[start : start+end]
	if !strings.Contains(branch, "diagnosticErrorRaw(traceErr)") || !strings.Contains(branch, "P01B_DIAGNOSTIC_RAW_CONTEXT %s %s %s %s") || strings.Contains(branch, "p.text") || strings.Contains(branch, "scanner.Text") {
		t.Fatal("missing/private raw metadata publication")
	}
}
