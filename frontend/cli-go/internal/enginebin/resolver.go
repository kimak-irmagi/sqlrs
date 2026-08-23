// Package enginebin resolves and validates local sqlrs engine executables.
// Its precedence and ownership rules are defined in
// docs/architecture/local-engine-binary-discovery-component-structure.md.
package enginebin

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Kind identifies the runtime role of an engine binary.
type Kind string

const (
	// KindHost is the native engine for the CLI host operating system.
	KindHost Kind = "host"
	// KindWSLPayload is a Linux engine distributed for installation into WSL.
	KindWSLPayload Kind = "wsl"
)

// Origin identifies the discovery source that supplied a resolved binary.
type Origin string

const (
	OriginExplicit    Origin = "explicit"
	OriginEnvironment Origin = "environment"
	OriginConfig      Origin = "config"
	OriginBundle      Origin = "bundle"
	OriginPath        Origin = "PATH"
)

// Format is the detected executable container format.
type Format string

const (
	FormatELF   Format = "elf"
	FormatPE    Format = "pe"
	FormatMachO Format = "macho"
)

// Request describes one binary role and its ordered override candidates.
type Request struct {
	Kind            Kind
	TargetOS        string
	TargetArch      string
	ExplicitPath    string
	EnvironmentPath string
	ConfigPath      string
}

// Resolved is a validated engine candidate.
type Resolved struct {
	Path   string
	Kind   Kind
	Origin Origin
	Format Format
	Arch   string
}

// Validation describes the executable metadata verified by Validate.
type Validation struct {
	Format Format
	Arch   string
}

// Resolver provides injectable host lookups for deterministic tests.
type Resolver struct {
	Executable func() (string, error)
	LookPath   func(string) (string, error)
}

// Resolve applies the documented override, bundle, and PATH precedence.
func (r Resolver) Resolve(req Request) (Resolved, error) {
	if req.Kind != KindHost && req.Kind != KindWSLPayload {
		return Resolved{}, fmt.Errorf("unknown engine runtime %q", req.Kind)
	}
	if strings.TrimSpace(req.TargetOS) == "" || strings.TrimSpace(req.TargetArch) == "" {
		return Resolved{}, fmt.Errorf("engine %s target OS and architecture are required", req.Kind)
	}

	ordered := []struct {
		origin Origin
		path   string
	}{
		{OriginExplicit, req.ExplicitPath},
		{OriginEnvironment, req.EnvironmentPath},
		{OriginConfig, req.ConfigPath},
	}
	for _, candidate := range ordered {
		if strings.TrimSpace(candidate.path) == "" {
			continue
		}
		return resolveCandidate(req, candidate.origin, candidate.path)
	}

	executableFn := r.Executable
	if executableFn == nil {
		executableFn = os.Executable
	}
	executablePath, executableErr := executableFn()
	if executableErr == nil && strings.TrimSpace(executablePath) != "" {
		bundlePath := bundleCandidate(filepath.Dir(executablePath), req)
		if _, err := os.Stat(bundlePath); err == nil {
			return resolveCandidate(req, OriginBundle, bundlePath)
		} else if !os.IsNotExist(err) {
			return Resolved{}, fmt.Errorf("validate %s bundle engine %q: %w", req.Kind, bundlePath, err)
		}
	}

	if req.Kind == KindHost {
		lookPathFn := r.LookPath
		if lookPathFn == nil {
			lookPathFn = exec.LookPath
		}
		name := "sqlrs-engine"
		if req.TargetOS == "windows" {
			name += ".exe"
		}
		if found, err := lookPathFn(name); err == nil {
			return resolveCandidate(req, OriginPath, found)
		}
	}

	envName := "SQLRS_DAEMON_PATH"
	attempts := "explicit, environment, config, bundle, PATH"
	if req.Kind == KindWSLPayload {
		envName = "SQLRS_WSL_ENGINE_PATH"
		attempts = "explicit, environment, config, bundle"
	}
	return Resolved{}, fmt.Errorf("%s engine was not found; attempted %s; set %s or rerun sqlrs init local with the matching engine flag", req.Kind, attempts, envName)
}

func resolveCandidate(req Request, origin Origin, candidate string) (Resolved, error) {
	pathValue, err := filepath.Abs(filepath.Clean(strings.TrimSpace(candidate)))
	if err != nil {
		return Resolved{}, fmt.Errorf("resolve %s %s engine %q: %w", origin, req.Kind, candidate, err)
	}
	validation, err := Validate(pathValue, req.TargetOS, req.TargetArch)
	if err != nil {
		return Resolved{}, fmt.Errorf("validate %s %s engine %q: %w", origin, req.Kind, pathValue, err)
	}
	return Resolved{Path: pathValue, Kind: req.Kind, Origin: origin, Format: validation.Format, Arch: validation.Arch}, nil
}

func bundleCandidate(root string, req Request) string {
	if req.Kind == KindWSLPayload {
		return filepath.Join(root, "libexec", "linux-"+req.TargetArch, "sqlrs-engine")
	}
	name := "sqlrs-engine"
	if req.TargetOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(root, name)
}

// Validate confirms that path is a regular executable for targetOS/targetArch.
func Validate(path, targetOS, targetArch string) (Validation, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Validation{}, err
	}
	if !info.Mode().IsRegular() {
		return Validation{}, fmt.Errorf("not a regular file")
	}
	if targetOS != "windows" && runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return Validation{}, fmt.Errorf("file is not executable")
	}
	file, err := os.Open(path)
	if err != nil {
		return Validation{}, err
	}
	defer file.Close()

	format, arch, err := inspect(file)
	if err != nil {
		return Validation{}, err
	}
	wantFormat, err := formatForOS(targetOS)
	if err != nil {
		return Validation{}, err
	}
	if format != wantFormat {
		return Validation{}, fmt.Errorf("format %s is incompatible with %s (expected %s)", format, targetOS, wantFormat)
	}
	if arch != targetArch {
		return Validation{}, fmt.Errorf("architecture %s is incompatible with %s", arch, targetArch)
	}
	return Validation{Format: format, Arch: arch}, nil
}

func inspect(file *os.File) (Format, string, error) {
	header := make([]byte, 64)
	n, err := io.ReadFull(file, header)
	if err != nil && err != io.ErrUnexpectedEOF {
		return "", "", fmt.Errorf("read executable header: %w", err)
	}
	if n < 4 {
		return "", "", fmt.Errorf("read executable header: file is too short")
	}
	if string(header[:4]) == "\x7fELF" {
		if n < 20 {
			return "", "", fmt.Errorf("read ELF header: file is too short")
		}
		if header[5] != 1 {
			return "", "", fmt.Errorf("unsupported ELF byte order")
		}
		arch, err := elfArch(binary.LittleEndian.Uint16(header[18:20]))
		return FormatELF, arch, err
	}
	if string(header[:2]) == "MZ" {
		if n < 64 {
			return "", "", fmt.Errorf("read DOS header: file is too short")
		}
		offset := int64(binary.LittleEndian.Uint32(header[0x3c:0x40]))
		peHeader := make([]byte, 6)
		if _, err := file.ReadAt(peHeader, offset); err != nil {
			return "", "", fmt.Errorf("read PE header: %w", err)
		}
		if string(peHeader[:4]) != "PE\x00\x00" {
			return "", "", fmt.Errorf("invalid PE signature")
		}
		arch, err := peArch(binary.LittleEndian.Uint16(peHeader[4:6]))
		return FormatPE, arch, err
	}
	if n < 8 {
		return "", "", fmt.Errorf("read Mach-O header: file is too short")
	}
	magic := binary.LittleEndian.Uint32(header[:4])
	if magic == 0xfeedface || magic == 0xfeedfacf {
		arch, err := machoArch(binary.LittleEndian.Uint32(header[4:8]))
		return FormatMachO, arch, err
	}
	return "", "", fmt.Errorf("unrecognized executable format")
}

func formatForOS(targetOS string) (Format, error) {
	switch targetOS {
	case "linux":
		return FormatELF, nil
	case "windows":
		return FormatPE, nil
	case "darwin":
		return FormatMachO, nil
	default:
		return "", fmt.Errorf("unsupported target OS %q", targetOS)
	}
}

func elfArch(machine uint16) (string, error) {
	switch machine {
	case 62:
		return "amd64", nil
	case 183:
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported ELF machine %d", machine)
	}
}

func peArch(machine uint16) (string, error) {
	switch machine {
	case 0x8664:
		return "amd64", nil
	case 0xaa64:
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported PE machine %#x", machine)
	}
}

func machoArch(cpu uint32) (string, error) {
	switch cpu {
	case 0x01000007:
		return "amd64", nil
	case 0x0100000c:
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported Mach-O CPU %#x", cpu)
	}
}
