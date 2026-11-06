package cli

import "io"

func PrintCacheUsage(w io.Writer) {
	io.WriteString(w, "Usage:\n")
	io.WriteString(w, "  sqlrs cache explain prepare [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] <ref>\n")
	io.WriteString(w, "  sqlrs cache explain prepare:psql [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--image <image-id>] [--] [psql-args...]\n")
	io.WriteString(w, "  sqlrs cache explain prepare:lb [--liquibase-version <release|latest>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--image <image-id>] [--] [liquibase-args...]\n\n")
	io.WriteString(w, "Notes:\n")
	io.WriteString(w, "  cache explain is read-only and only supports wrapped prepare stages.\n")
	io.WriteString(w, "  --watch and --no-watch are not accepted because cache explain does not execute the stage.\n")
	io.WriteString(w, "  --liquibase-version selects a Liquibase release (default: latest) before --.\n")
}
