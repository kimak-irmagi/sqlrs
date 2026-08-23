package cli

import "io"

func PrintInitUsage(w io.Writer) {
	io.WriteString(w, "Usage:\n")
	io.WriteString(w, "  sqlrs init [local] [flags]\n")
	io.WriteString(w, "  sqlrs init remote --url <url> --token <token> [flags]\n\n")
	io.WriteString(w, "Global flags:\n")
	io.WriteString(w, "  --workspace <path>      Workspace root (default: cwd)\n")
	io.WriteString(w, "  --force                 Allow nested workspaces\n")
	io.WriteString(w, "  --update                Update existing workspace config\n")
	io.WriteString(w, "  --dry-run               Show intended actions only\n")
	io.WriteString(w, "  -h, --help              Show help\n\n")
	io.WriteString(w, "Local flags:\n")
	io.WriteString(w, "  --snapshot <backend>    Snapshot backend (auto|btrfs|overlay|copy)\n")
	io.WriteString(w, "  --store <type> [path]   Store type and optional path (dir|device|image)\n")
	io.WriteString(w, "  --store-size <NGB>      Image store size (default: 100GB)\n")
	io.WriteString(w, "  --reinit                Recreate the store (destructive)\n")
	io.WriteString(w, "  --engine <path>         Engine binary path\n")
	io.WriteString(w, "  --wsl-engine <path>     Linux engine binary to install into WSL\n")
	io.WriteString(w, "  --shared-cache          Enable shared cache in local config\n")
	io.WriteString(w, "  --no-start              Skip WSL auto-start during init\n")
	io.WriteString(w, "  --distro <name>         WSL distro name\n\n")
	io.WriteString(w, "Remote flags:\n")
	io.WriteString(w, "  --url <url>             Remote engine endpoint\n")
	io.WriteString(w, "  --token <token>         Remote engine token\n")
}
