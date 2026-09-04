package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintRunUsageShowsAliasAndRawModes(t *testing.T) {
	var buf bytes.Buffer
	PrintRunUsage(&buf)

	out := buf.String()
	if !strings.Contains(out, "sqlrs run [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] <run-ref> --instance <id|id-prefix|name>") {
		t.Fatalf("expected alias-mode usage, got %q", out)
	}
	if !strings.Contains(out, "sqlrs run[:kind] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--instance <id|id-prefix|name>] [-- <command> ] [args...]") {
		t.Fatalf("expected raw-mode usage, got %q", out)
	}
	if !strings.Contains(out, "--ref-mode <mode>") {
		t.Fatalf("expected ref-mode option, got %q", out)
	}
	if !strings.Contains(out, "prepare ... run") {
		t.Fatalf("expected composite note, got %q", out)
	}
}
