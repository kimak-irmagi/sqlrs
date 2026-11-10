package benchmark

import (
	"bufio"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var errDiagnosticTrace = errors.New("incomplete diagnostic trace")
var diagnosticTraceLine = regexp.MustCompile(`^\s*([0-9]+)\s+([0-9]+\.[0-9]+)\s+(.*)$`)
var diagnosticCString = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
var diagnosticExecReturn = regexp.MustCompile(`\) = (-?[0-9]+)(?: [A-Z][A-Z0-9_]*(?: \([^\n]*\))?)?(?: <[0-9.]+>)?$`)
var diagnosticExit = regexp.MustCompile(`^\+\+\+ (?:exited with [0-9]+|killed by SIG[A-Z0-9]+(?: \(core dumped\))?) \+\+\+$`)

// diagnosticDecodeError stores a fixed code only, never a copy of trace data.
// DG03/DG09 checkpoints: 1 quota, 2 framing, 3 PID, 4 time, 5 pending exit,
// 6 duplicate pending, 7 orphan resume, 8 record kind, 9 result syntax,
// 10 result value, 11 program, 12 program escape, 13 argv shape, 14 argv quote,
// 15 argv escape, 16 separator/truncation, 17 stream/incomplete lifecycle.
type diagnosticDecodeError struct{ code int }

func (diagnosticDecodeError) Error() string { return "incomplete diagnostic trace" }
func diagnosticDecodeCode(err error) int {
	var value diagnosticDecodeError
	if errors.As(err, &value) && value.code >= 1 && value.code <= 17 {
		return value.code
	}
	return 0
}

// diagnosticContextError keeps only closed context, never the trace body. Its
// cause preserves the old checkpoint for errors.As (CX01–CX04 in diagnostic tests).
type diagnosticContextError struct {
	cause           error
	location, shape string
	prefix          diagnosticPrefixContext
}

func (diagnosticContextError) Error() string   { return "incomplete diagnostic trace" }
func (e diagnosticContextError) Unwrap() error { return e.cause }

// diagnosticFailureContext is an allowlisted observation, not permission to
// accept a new record. See the approved diagnostic component amendment.
type diagnosticFailureContext struct{ Location, Shape string }

func diagnosticErrorContext(err error) diagnosticFailureContext {
	result := diagnosticFailureContext{"none", "none"}
	var value diagnosticContextError
	if errors.As(err, &value) {
		switch value.location {
		case "record", "pending_entry":
			result.Location = value.location
		}
		switch value.shape {
		case "superseded_exec", "exec_entry", "exec_resume", "other":
			result.Shape = value.shape
		}
	}
	return result
}

var diagnosticSupersededShape = regexp.MustCompile(`^\+\+\+ superseded by execve in pid [1-9][0-9]* \+\+\+$`)

// diagnosticRecordShape classifies only a fixed form in an already-read body.
// Even a recognized superseded marker remains rejected by the current decoder.
func diagnosticRecordShape(body string) string {
	if diagnosticSupersededShape.MatchString(body) {
		return "superseded_exec"
	}
	if strings.HasPrefix(body, "execve(") || strings.HasPrefix(body, "execveat(") {
		return "exec_entry"
	}
	if strings.HasPrefix(body, "<... execve resumed>") || strings.HasPrefix(body, "<... execveat resumed>") {
		return "exec_resume"
	}
	return "other"
}

// diagnosticInterval has no process IDs/argv. The contract is recorded in
// docs/architecture/issue89-performance-diagnostic-design.md. DG03/DG05 retain executable
// segments privately until they can be clipped into relative public intervals.
type diagnosticInterval struct {
	Category       string
	StartNS, EndNS int64
}
type diagnosticPublicInterval struct {
	Category       string
	StartMS, EndMS float64
}
type diagnosticTraceSummary struct {
	CoveredMS, ResidualMS float64
	Commands              int
	Intervals             []diagnosticPublicInterval
}

// diagnosticCategory returns fixed labels only. Unknown commands remain visible
// as other, rather than inferring a role from arbitrary SQL/password substrings.
func diagnosticCategory(program string, argv []string) string {
	switch filepath.Base(program) {
	case "java":
		return "host.java"
	case "liquibase":
		return "host.liquibase"
	case "pg_ctl":
		return "host.pg_control"
	case "psql":
		return "host.pg_identity"
	case "docker":
		if len(argv) < 2 {
			return "other"
		}
		args := argv[1:]
		if args[0] == "container" && len(args) > 1 {
			args = args[1:]
		}
		if len(args) > 1 && args[0] == "image" && args[1] == "inspect" {
			return "docker.image_inspect"
		}
		switch args[0] {
		case "inspect":
			return "docker.inspect"
		case "create":
			return "docker.create"
		case "start":
			return "docker.start"
		case "stop", "kill":
			return "docker.stop"
		case "rm":
			return "docker.remove"
		case "cp":
			return "docker.copy"
		case "run":
			return "docker.run"
		case "exec":
			for _, arg := range args {
				if arg == "/sqlrs/helper/step-capture.sh" {
					return "docker.step"
				}
				if arg == "/sqlrs/helper/capture.sh" {
					return "docker.planner"
				}
			}
			return "docker.exec"
		}
	}
	return "other"
}

// diagnosticTimestamp decodes epoch seconds exactly; no float precision loss or
// raw input is included in errors (DG03/DG09).
func diagnosticTimestamp(s string) (int64, error) {
	seconds, fraction, ok := strings.Cut(s, ".")
	if !ok || len(fraction) > 9 || len(fraction) == 0 {
		return 0, errDiagnosticTrace
	}
	a, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil || a < 0 || a > 9223372035 {
		return 0, errDiagnosticTrace
	}
	b, err := strconv.ParseInt(fraction+strings.Repeat("0", 9-len(fraction)), 10, 64)
	if err != nil || b < 0 {
		return 0, errDiagnosticTrace
	}
	return a*int64(time.Second) + b, nil
}

// decodeDiagnosticTrace accepts the configured strace exec/exit format only.
// A failed exec is not a launch; completed re-exec closes the preceding image.
// Missing commands are never interpreted as zero overhead. DG03/DG04.
func decodeDiagnosticTrace(input io.Reader, enginePID int, observedUntil ...int64) ([]diagnosticInterval, error) {
	// Only an explicit final API boundary permits ignoring a proven terminal
	// cleanup attempt which started strictly after it. Default callers retain
	// full-stream strictness; an absent terminal record is always incomplete.
	var cutoff int64
	if len(observedUntil) > 0 {
		cutoff = observedUntil[0]
	}
	return decodeDiagnosticTracePolicy(input, enginePID, cutoff, false)
}

// decodeMeasuredDiagnosticTrace opts into EU01–EU05: an exact terminal record
// may close a valid exec attempt without establishing which image actually ran.
// Default/late-cleanup decoding remains separate and strict. No raw data escapes.
func decodeMeasuredDiagnosticTrace(input io.Reader, enginePID int) ([]diagnosticInterval, error) {
	return decodeDiagnosticTracePolicy(input, enginePID, 0, true)
}

// decodeDiagnosticTracePolicy shares framing/quotas/lifecycle checks; only the
// measured policy can retain explicitly uncertain intervals (diagnostic design).
func decodeDiagnosticTracePolicy(input io.Reader, enginePID int, cutoff int64, measured bool) ([]diagnosticInterval, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 65536)
	if measured {
		// UR01 rejects control-character injection. ScanLines normally drops
		// a terminal CR, which would otherwise turn corrupt raw syntax valid.
		// Keep default strict framing untouched, including its old tests.
		scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
			advance, token, err := bufio.ScanLines(data, atEOF)
			if token != nil && advance > len(token) && data[len(token)] == '\r' {
				return 0, nil, errDiagnosticTrace
			}
			return advance, token, err
		})
	}
	active := map[int]diagnosticInterval{}
	type partial struct {
		text       string
		at         int64
		raw        *diagnosticRawCall
		rawFailure diagnosticRawFailure
	}
	pending := map[int]partial{}
	last := map[int]int64{}
	result := []diagnosticInterval{}
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > 250000 {
			return nil, diagnosticDecodeError{1}
		}
		m := diagnosticTraceLine.FindStringSubmatch(scanner.Text())
		if m == nil {
			return nil, diagnosticDecodeError{2}
		}
		pid, err := strconv.Atoi(m[1])
		if err != nil || pid < 1 {
			return nil, diagnosticDecodeError{3}
		}
		at, err := diagnosticTimestamp(m[2])
		if err != nil || at < last[pid] {
			return nil, diagnosticDecodeError{4}
		}
		last[pid] = at
		body := m[3]
		if diagnosticExit.MatchString(body) {
			imageEnd := at
			if p, found := pending[pid]; found {
				if measured && p.raw == nil {
					// Validate the entry, not an invented successful return. A
					// malformed unfinished line must not bypass argument checks.
					if _, err := diagnosticExecCategory(p.text); err != nil {
						if detail := normalizeDiagnosticRawFailure(p.rawFailure); detail.RejectStage != "none" {
							err = diagnosticRawFailureError{cause: err, detail: detail}
						}
						return nil, newDiagnosticContext(err, "pending_entry", p.text)
					}
					imageEnd = p.at
				} else if !measured && (cutoff <= 0 || p.at <= cutoff) {
					return nil, diagnosticDecodeError{5}
				}
				delete(pending, pid)
				if measured && p.raw == nil && pid != enginePID {
					// Append the preceding image first, preserving chronology
					// within one PID without merging overlapping other PIDs.
					if item, found := active[pid]; found {
						item.EndNS = imageEnd
						result = append(result, item)
						delete(active, pid)
					}
					result = append(result, diagnosticInterval{"exec.unconfirmed", p.at, at})
				}
			}
			if item, found := active[pid]; found {
				item.EndNS = imageEnd
				if pid != enginePID {
					result = append(result, item)
				}
				delete(active, pid)
			}
			continue
		}
		start := at
		var rawFailure diagnosticRawFailure
		if measured {
			// UR02: only an exact same-PID/name raw resume can finish its
			// pending call. No thread handoff or executable identity is inferred.
			rawBody := body
			resume := diagnosticRawResume.FindStringSubmatch(body)
			if resume != nil {
				p, found := pending[pid]
				if !found {
					return nil, diagnosticDecodeError{7}
				}
				if p.raw == nil || !p.raw.pending || p.raw.name != resume[1] {
					return nil, diagnosticDecodeError{8}
				}
				rawBody, start = p.text+resume[2], p.at
			}
			if resume != nil || strings.HasPrefix(body, "syscall_") || strings.HasPrefix(body, "???(") {
				call, rawErr := parseDiagnosticRaw(rawBody)
				if rawErr == nil {
					if _, found := pending[pid]; found && resume == nil {
						return nil, diagnosticDecodeError{6}
					}
					delete(pending, pid)
					if prior, found := active[pid]; !found || prior.Category != "process.unattributed" {
						if found && pid != enginePID {
							prior.EndNS = start
							result = append(result, prior)
						}
						active[pid] = diagnosticInterval{Category: "process.unattributed", StartNS: start}
					}
					if call.pending || call.unresolved {
						pending[pid] = partial{text: strings.TrimSuffix(rawBody, "<unfinished ...>"), at: start, raw: &call}
					}
					continue
				}
				// Preserve old PX02 malformed unfinished entries and their
				// exact pending-entry rejection/context at the terminal event.
				rawFailure = diagnosticErrorRaw(rawErr)
				if resume != nil || !strings.HasSuffix(body, "<unfinished ...>") {
					return nil, newDiagnosticContext(rawErr, "record", body)
				}
			}
			if p, found := pending[pid]; found && p.raw != nil {
				return nil, diagnosticDecodeError{8}
			}
		}
		if strings.HasSuffix(body, "<unfinished ...>") {
			if _, found := pending[pid]; found {
				return nil, diagnosticDecodeError{6}
			}
			pending[pid] = partial{text: strings.TrimSuffix(body, "<unfinished ...>"), at: at, rawFailure: rawFailure}
			continue
		}
		if strings.HasPrefix(body, "<... execve resumed>") || strings.HasPrefix(body, "<... execveat resumed>") {
			p, found := pending[pid]
			if !found {
				return nil, diagnosticDecodeError{7}
			}
			delete(pending, pid)
			close := strings.Index(body, ">")
			body = p.text + body[close+1:]
			start = p.at
		}
		if !strings.HasPrefix(body, "execve(") && !strings.HasPrefix(body, "execveat(") {
			return nil, newDiagnosticContext(diagnosticDecodeError{8}, "record", body)
		}
		returned := diagnosticExecReturn.FindStringSubmatch(body)
		if returned == nil {
			return nil, diagnosticDecodeError{9}
		}
		if returned[1] == "-1" {
			continue
		}
		if returned[1] != "0" {
			return nil, diagnosticDecodeError{10}
		}
		category, err := diagnosticExecCategory(body)
		if err != nil {
			return nil, newDiagnosticContext(err, "record", body)
		}
		if prior, found := active[pid]; found {
			prior.EndNS = start
			if pid != enginePID {
				result = append(result, prior)
			}
		}
		active[pid] = diagnosticInterval{Category: category, StartNS: start}
	}
	if scanner.Err() != nil || len(active) != 0 || len(pending) != 0 {
		return nil, diagnosticDecodeError{17}
	}
	return result, nil
}

// diagnosticExecCategory validates the same program/argv for a completed exec
// or an unfinished entry. It says nothing about success; the caller must have
// a confirmed return before attributing this category to a running image.
func diagnosticExecCategory(body string) (string, error) {
	if !strings.HasPrefix(body, "execve(") && !strings.HasPrefix(body, "execveat(") {
		return "", diagnosticDecodeError{8}
	}
	loc := diagnosticCString.FindStringIndex(body)
	if loc == nil {
		return "", diagnosticDecodeError{11}
	}
	program, err := strconv.Unquote(body[loc[0]:loc[1]])
	if err != nil {
		return "", diagnosticDecodeError{12}
	}
	tail := body[loc[1]:]
	a, b := strings.Index(tail, "["), strings.LastIndex(tail, "]")
	if a < 0 || b < a {
		return "", diagnosticDecodeError{13}
	}
	list := tail[a+1 : b]
	argv := []string{}
	for strings.TrimSpace(list) != "" {
		list = strings.TrimSpace(list)
		q := diagnosticCString.FindStringIndex(list)
		if q == nil || q[0] != 0 {
			return "", diagnosticDecodeError{14}
		}
		value, err := strconv.Unquote(list[:q[1]])
		if err != nil {
			return "", diagnosticDecodeError{15}
		}
		argv = append(argv, value)
		list = strings.TrimSpace(list[q[1]:])
		if list != "" {
			if list[0] != ',' {
				return "", diagnosticDecodeError{16}
			}
			list = list[1:]
		}
	}
	return diagnosticCategory(program, argv), nil
}

// summarizeDiagnosticIntervals reports clipped gross intervals and union
// coverage. Gross category costs overlap and are deliberately not additive.
func summarizeDiagnosticIntervals(rows []diagnosticInterval, start, end time.Time) (diagnosticTraceSummary, error) {
	result := diagnosticTraceSummary{Intervals: []diagnosticPublicInterval{}}
	a, b := start.UnixNano(), end.UnixNano()
	if b <= a || end.Sub(start) <= 0 || end.Sub(start) != time.Duration(b-a) {
		return result, errDiagnosticTrace
	}
	spans := [][2]int64{}
	for _, r := range rows {
		if r.EndNS < r.StartNS {
			return result, errDiagnosticTrace
		}
		lo, hi := max(a, r.StartNS), min(b, r.EndNS)
		if hi <= lo {
			continue
		}
		result.Intervals = append(result.Intervals, diagnosticPublicInterval{r.Category, float64(lo-a) / 1e6, float64(hi-a) / 1e6})
		result.Commands++
		spans = append(spans, [2]int64{lo, hi})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	var covered int64
	for i := 0; i < len(spans); {
		lo, hi := spans[i][0], spans[i][1]
		i++
		for i < len(spans) && spans[i][0] <= hi {
			hi = max(hi, spans[i][1])
			i++
		}
		covered += hi - lo
	}
	result.CoveredMS = float64(covered) / 1e6
	result.ResidualMS = float64(b-a-covered) / 1e6
	return result, nil
}

// copyDiagnosticTrace enforces the private byte cap during acquisition, not by
// periodically inspecting an unbounded file. Caller stops its group on error.
func copyDiagnosticTrace(dst io.Writer, src io.Reader, limit int64) error {
	if limit < 1 {
		return errDiagnosticTrace
	}
	n, err := io.Copy(dst, io.LimitReader(src, limit))
	if err != nil {
		return errDiagnosticTrace
	}
	if n == limit {
		var b [1]byte
		n, err := src.Read(b[:])
		if n != 0 || err != io.EOF {
			return errDiagnosticTrace
		}
	}
	return nil
}
