package runtime

import (
	"strings"
	"testing"
)

// CT05: inspect the actual embedded scripts, not a simplified copy. Four
// phase barriers remain; deduplicating filesystem arguments cannot move them.
func TestCostHelperFourFlushBarriers(t *testing.T) {
	for _, kind := range []string{"planning", "session"} {
		t.Run(kind, func(t *testing.T) {
			bytes, digest := LiquibaseCaptureHelper()
			if kind == "session" {
				bytes, digest = LiquibaseStepCaptureHelper()
			}
			if captureDigest(bytes) != digest {
				t.Fatal("embedded helper digest differs from executed bytes")
			}
			source := string(bytes)
			marker := `rm "$stdout_fifo" "$stderr_fifo" || die`
			_, terminal, found := strings.Cut(source, marker)
			if !found {
				t.Fatal("real terminal capture boundary missing")
			}
			var calls []string
			for _, line := range strings.Split(terminal, "\n") {
				if strings.HasPrefix(line, "sync ") {
					calls = append(calls, line)
				}
			}
			want := []string{`sync -f "$capture" || die`, `sync -f "$capture" || die`,
				`sync -f "$manifest_partial" || die`, `sync -f "$capture" || die`}
			if strings.Join(calls, "\n") != strings.Join(want, "\n") {
				t.Fatal("terminal flush boundaries/arguments differ", calls)
			}
			boundaries := []string{want[0], `stdout_hash=$(sha256sum "$stdout_partial")`,
				`mv "$stderr_partial" "$capture/stderr.complete" || die`, want[1],
				`> "$manifest_partial" || die`, want[2], `mv "$manifest_partial" "$capture/manifest.complete" || die`, want[3]}
			for _, boundary := range boundaries {
				_, remaining, found := strings.Cut(terminal, boundary)
				if !found {
					t.Fatal("flush moved across its established phase", boundary)
				}
				terminal = remaining
			}
			if kind == "session" && (strings.Count(source, `sync -f /sqlrs/capture || die`) != 2 ||
				strings.Count(source, `sync -f "$partial" || die`) != 1) {
				t.Fatal("readiness/prune flush changed with terminal deduplication")
			}
		})
	}
}
