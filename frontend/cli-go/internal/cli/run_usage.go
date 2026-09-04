package cli

import "io"

func PrintRunUsage(w io.Writer) {
	io.WriteString(w, "Usage:\n")
	io.WriteString(w, "  sqlrs run [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] <run-ref> --instance <id|id-prefix|name>\n")
	io.WriteString(w, "  sqlrs run[:kind] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--instance <id|id-prefix|name>] [-- <command> ] [args...]\n\n")
	io.WriteString(w, "Options:\n")
	io.WriteString(w, "  --instance <id|id-prefix|name>  Target instance by full id, 8+ hex prefix, or name\n")
	io.WriteString(w, "  --ref <git-ref>       Read run inputs from a selected Git revision\n")
	io.WriteString(w, "  --ref-mode <mode>     Ref mode: worktree (default) or blob\n")
	io.WriteString(w, "  --ref-keep-worktree   Keep detached worktree after exit (worktree mode only)\n")
	io.WriteString(w, "  -h, --help            Show help\n\n")
	io.WriteString(w, "Notes:\n")
	io.WriteString(w, "  Alias mode resolves <run-ref> from the current working directory.\n")
	io.WriteString(w, "  Paths inside the alias file resolve relative to that alias file.\n")
	io.WriteString(w, "  If <command> is omitted, the run kind default command is used.\n")
	io.WriteString(w, "  In composite form, prepare ... run may mix raw and alias stages.\n")
	io.WriteString(w, "  Bounded --ref support is single-stage only; prepare ... run --ref is not supported yet.\n")
}
