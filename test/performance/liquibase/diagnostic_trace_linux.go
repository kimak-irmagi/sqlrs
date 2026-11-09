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
func decodeDiagnosticTrace(input io.Reader, enginePID int) ([]diagnosticInterval, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 65536)
	active := map[int]diagnosticInterval{}
	type partial struct {
		text string
		at   int64
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
			if _, found := pending[pid]; found {
				return nil, diagnosticDecodeError{5}
			}
			if item, found := active[pid]; found {
				item.EndNS = at
				if pid != enginePID {
					result = append(result, item)
				}
				delete(active, pid)
			}
			continue
		}
		start := at
		if strings.HasSuffix(body, "<unfinished ...>") {
			if _, found := pending[pid]; found {
				return nil, diagnosticDecodeError{6}
			}
			pending[pid] = partial{strings.Split(body, "<unfinished ...>")[0], at}
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
			return nil, diagnosticDecodeError{8}
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
		loc := diagnosticCString.FindStringIndex(body)
		if loc == nil {
			return nil, diagnosticDecodeError{11}
		}
		program, err := strconv.Unquote(body[loc[0]:loc[1]])
		if err != nil {
			return nil, diagnosticDecodeError{12}
		}
		tail := body[loc[1]:]
		a, b := strings.Index(tail, "["), strings.LastIndex(tail, "]")
		if a < 0 || b < a {
			return nil, diagnosticDecodeError{13}
		}
		list := tail[a+1 : b]
		argv := []string{}
		for strings.TrimSpace(list) != "" {
			list = strings.TrimSpace(list)
			q := diagnosticCString.FindStringIndex(list)
			if q == nil || q[0] != 0 {
				return nil, diagnosticDecodeError{14}
			}
			value, err := strconv.Unquote(list[:q[1]])
			if err != nil {
				return nil, diagnosticDecodeError{15}
			}
			argv = append(argv, value)
			list = strings.TrimSpace(list[q[1]:])
			if list != "" {
				if list[0] != ',' {
					return nil, diagnosticDecodeError{16}
				}
				list = list[1:]
			}
		}
		if prior, found := active[pid]; found {
			prior.EndNS = start
			if pid != enginePID {
				result = append(result, prior)
			}
		}
		active[pid] = diagnosticInterval{Category: diagnosticCategory(program, argv), StartNS: start}
	}
	if scanner.Err() != nil || len(active) != 0 || len(pending) != 0 {
		return nil, diagnosticDecodeError{17}
	}
	return result, nil
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
