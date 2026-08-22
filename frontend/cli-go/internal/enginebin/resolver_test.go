package enginebin

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveHostPrecedence(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "sqlrs")
	explicit := writeELF(t, root, "explicit", 62)
	environment := writeELF(t, root, "environment", 62)
	configured := writeELF(t, root, "configured", 62)
	_ = writeELF(t, root, "sqlrs-engine", 62)
	pathCandidate := writeELF(t, root, "path-engine", 62)
	resolver := Resolver{
		Executable: func() (string, error) { return executable, nil },
		LookPath:   func(string) (string, error) { return pathCandidate, nil },
	}

	tests := []struct {
		name, explicit, environment, configured, wantPath string
		wantOrigin                                        Origin
	}{
		{"explicit", explicit, environment, configured, explicit, OriginExplicit},
		{"environment", "", environment, configured, environment, OriginEnvironment},
		{"config", "", "", configured, configured, OriginConfig},
		{"bundle", "", "", "", filepath.Join(root, "sqlrs-engine"), OriginBundle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(Request{Kind: KindHost, TargetOS: "linux", TargetArch: "amd64", ExplicitPath: tt.explicit, EnvironmentPath: tt.environment, ConfigPath: tt.configured})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got.Path != tt.wantPath || got.Origin != tt.wantOrigin {
				t.Fatalf("got path=%q origin=%q, want path=%q origin=%q", got.Path, got.Origin, tt.wantPath, tt.wantOrigin)
			}
		})
	}

	if err := os.Remove(filepath.Join(root, "sqlrs-engine")); err != nil {
		t.Fatal(err)
	}
	got, err := resolver.Resolve(Request{Kind: KindHost, TargetOS: "linux", TargetArch: "amd64"})
	if err != nil {
		t.Fatalf("PATH fallback: %v", err)
	}
	if got.Path != pathCandidate || got.Origin != OriginPath {
		t.Fatalf("PATH fallback got %+v", got)
	}
}

func TestResolveWSLPayloadPrecedenceAndNeverUsesPath(t *testing.T) {
	root := t.TempDir()
	bundle := writeELF(t, filepath.Join(root, "libexec", "linux-amd64"), "sqlrs-engine", 62)
	environment := writeELF(t, root, "env-linux-engine", 62)
	configured := writeELF(t, root, "configured-linux-engine", 62)
	lookCalls := 0
	resolver := Resolver{
		Executable: func() (string, error) { return filepath.Join(root, "sqlrs.exe"), nil },
		LookPath: func(string) (string, error) {
			lookCalls++
			return "", errors.New("must not be called")
		},
	}

	for _, tt := range []struct {
		name, environment, configured, want string
		origin                              Origin
	}{
		{"environment", environment, configured, environment, OriginEnvironment},
		{"config", "", configured, configured, OriginConfig},
		{"bundle", "", "", bundle, OriginBundle},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(Request{Kind: KindWSLPayload, TargetOS: "linux", TargetArch: "amd64", EnvironmentPath: tt.environment, ConfigPath: tt.configured})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got.Path != tt.want || got.Origin != tt.origin {
				t.Fatalf("got %+v, want path=%q origin=%q", got, tt.want, tt.origin)
			}
		})
	}
	if lookCalls != 0 {
		t.Fatalf("WSL payload unexpectedly searched PATH %d times", lookCalls)
	}
}

func TestExplicitInvalidCandidateDoesNotFallThrough(t *testing.T) {
	root := t.TempDir()
	writeELF(t, root, "sqlrs-engine", 62)
	missing := filepath.Join(root, "missing")
	resolver := Resolver{Executable: func() (string, error) { return filepath.Join(root, "sqlrs"), nil }}
	_, err := resolver.Resolve(Request{Kind: KindHost, TargetOS: "linux", TargetArch: "amd64", ExplicitPath: missing})
	if err == nil || !strings.Contains(err.Error(), "explicit") || !strings.Contains(err.Error(), missing) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBundleResolutionUsesExecutableDirectoryNotWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	want := writeELF(t, root, "sqlrs-engine", 62)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	resolver := Resolver{Executable: func() (string, error) { return filepath.Join(root, "sqlrs"), nil }}
	got, err := resolver.Resolve(Request{Kind: KindHost, TargetOS: "linux", TargetArch: "amd64"})
	if err != nil || got.Path != want {
		t.Fatalf("got %+v err=%v, want %q", got, err, want)
	}
}

func TestValidateExecutableFormatsAndArchitectures(t *testing.T) {
	root := t.TempDir()
	linuxAMD64 := writeELF(t, root, "linux-amd64", 62)
	linuxARM64 := writeELF(t, root, "linux-arm64", 183)
	windowsAMD64 := writePE(t, root, "windows-amd64.exe", 0x8664)
	macAMD64 := writeMachO(t, root, "darwin-amd64", 0x01000007)
	for _, tt := range []struct {
		name, path, os, arch string
		format               Format
	}{
		{"elf", linuxAMD64, "linux", "amd64", FormatELF},
		{"pe", windowsAMD64, "windows", "amd64", FormatPE},
		{"macho", macAMD64, "darwin", "amd64", FormatMachO},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Validate(tt.path, tt.os, tt.arch)
			if err != nil || got.Format != tt.format || got.Arch != tt.arch {
				t.Fatalf("got %+v err=%v", got, err)
			}
		})
	}
	for _, tt := range []struct{ name, path, os, arch string }{
		{"wrong architecture", linuxARM64, "linux", "amd64"},
		{"wrong format", windowsAMD64, "linux", "amd64"},
		{"missing", filepath.Join(root, "missing"), "linux", "amd64"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Validate(tt.path, tt.os, tt.arch); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	short := filepath.Join(root, "short")
	if err := os.WriteFile(short, []byte{0x7f, 'E'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(short, "linux", "amd64"); err == nil {
		t.Fatal("expected truncated binary error")
	}
	if _, err := Validate(root, "linux", "amd64"); err == nil {
		t.Fatal("expected directory validation error")
	}
}

func TestResolutionErrorNamesRuntimeAndAttemptedSources(t *testing.T) {
	root := t.TempDir()
	resolver := Resolver{
		Executable: func() (string, error) { return filepath.Join(root, "sqlrs.exe"), nil },
		LookPath:   func(string) (string, error) { return "", errors.New("not found") },
	}
	_, err := resolver.Resolve(Request{Kind: KindHost, TargetOS: "windows", TargetArch: "amd64"})
	if err == nil {
		t.Fatal("expected resolution error")
	}
	for _, fragment := range []string{"host", "bundle", "PATH", "SQLRS_DAEMON_PATH"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not contain %q", err, fragment)
		}
	}
}

func TestResolveRejectsInvalidRequestsAndBrokenCandidates(t *testing.T) {
	resolver := Resolver{Executable: func() (string, error) { return "", errors.New("executable unavailable") }, LookPath: func(string) (string, error) { return "", errors.New("not found") }}
	for _, request := range []Request{
		{Kind: "other", TargetOS: "linux", TargetArch: "amd64"},
		{Kind: KindHost, TargetArch: "amd64"},
		{Kind: KindHost, TargetOS: "linux"},
	} {
		if _, err := resolver.Resolve(request); err == nil {
			t.Fatalf("expected invalid request error for %+v", request)
		}
	}

	root := t.TempDir()
	bundleDir := filepath.Join(root, "sqlrs-engine")
	if err := os.Mkdir(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver = Resolver{Executable: func() (string, error) { return filepath.Join(root, "sqlrs"), nil }}
	if _, err := resolver.Resolve(Request{Kind: KindHost, TargetOS: "linux", TargetArch: "amd64"}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected corrupt bundle error, got %v", err)
	}

	badPath := writeBinary(t, t.TempDir(), "sqlrs-engine", []byte("not an executable"))
	resolver = Resolver{Executable: func() (string, error) { return "", errors.New("unavailable") }, LookPath: func(string) (string, error) { return badPath, nil }}
	if _, err := resolver.Resolve(Request{Kind: KindHost, TargetOS: "linux", TargetArch: "amd64"}); err == nil || !strings.Contains(err.Error(), "PATH") {
		t.Fatalf("expected invalid PATH candidate error, got %v", err)
	}

	resolver = Resolver{Executable: func() (string, error) { return filepath.Join(t.TempDir(), "sqlrs.exe"), nil }, LookPath: func(string) (string, error) { t.Fatal("WSL resolver searched PATH"); return "", nil }}
	if _, err := resolver.Resolve(Request{Kind: KindWSLPayload, TargetOS: "linux", TargetArch: "amd64"}); err == nil || !strings.Contains(err.Error(), "SQLRS_WSL_ENGINE_PATH") {
		t.Fatalf("expected WSL resolution diagnostic, got %v", err)
	}
}

func TestResolveUsesDefaultHostLookups(t *testing.T) {
	if os.PathSeparator != '\\' {
		t.Skip("this test supplies a PE executable through Windows PATH")
	}
	root := t.TempDir()
	want := writePE(t, root, "sqlrs-engine.exe", 0x8664)
	t.Setenv("PATH", root)
	got, err := (Resolver{}).Resolve(Request{Kind: KindHost, TargetOS: "windows", TargetArch: "amd64"})
	if err != nil {
		t.Fatalf("Resolve with default lookups: %v", err)
	}
	if got.Path != want || got.Origin != OriginPath {
		t.Fatalf("got %+v, want PATH candidate %q", got, want)
	}
}

func TestValidateRejectsMalformedExecutableHeaders(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name, targetOS string
		data           []byte
	}{
		{"tiny", "linux", []byte{1, 2, 3}},
		{"short elf", "linux", []byte{0x7f, 'E', 'L', 'F'}},
		{"big endian elf", "linux", func() []byte { b := make([]byte, 20); copy(b, []byte{0x7f, 'E', 'L', 'F'}); b[5] = 2; return b }()},
		{"unknown elf machine", "linux", func() []byte {
			b := make([]byte, 20)
			copy(b, []byte{0x7f, 'E', 'L', 'F'})
			b[5] = 1
			binary.LittleEndian.PutUint16(b[18:], 1)
			return b
		}()},
		{"short dos", "windows", []byte{'M', 'Z', 0, 0}},
		{"missing pe header", "windows", func() []byte {
			b := make([]byte, 64)
			copy(b, []byte{'M', 'Z'})
			binary.LittleEndian.PutUint32(b[0x3c:], 1000)
			return b
		}()},
		{"invalid pe signature", "windows", func() []byte {
			b := make([]byte, 80)
			copy(b, []byte{'M', 'Z'})
			binary.LittleEndian.PutUint32(b[0x3c:], 64)
			copy(b[64:], []byte("NOPE!!"))
			return b
		}()},
		{"unknown pe machine", "windows", func() []byte {
			b := make([]byte, 80)
			copy(b, []byte{'M', 'Z'})
			binary.LittleEndian.PutUint32(b[0x3c:], 64)
			copy(b[64:], []byte{'P', 'E', 0, 0})
			return b
		}()},
		{"short macho", "darwin", []byte{0xce, 0xfa, 0xed, 0xfe}},
		{"unknown macho cpu", "darwin", func() []byte { b := make([]byte, 8); binary.LittleEndian.PutUint32(b, 0xfeedfacf); return b }()},
		{"unknown format", "linux", []byte("plain text")},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			path := writeBinary(t, root, strings.ReplaceAll(tt.name, " ", "-"), tt.data)
			if _, err := Validate(path, tt.targetOS, "amd64"); err == nil {
				t.Fatal("expected malformed executable error")
			}
		})
	}
}

func TestValidateSupportsArm64AndRejectsUnknownTargetOS(t *testing.T) {
	root := t.TempDir()
	for _, tt := range []struct {
		path, targetOS string
	}{
		{writeELF(t, root, "linux-arm64", 183), "linux"},
		{writePE(t, root, "windows-arm64", 0xaa64), "windows"},
		{writeMachO(t, root, "darwin-arm64", 0x0100000c), "darwin"},
	} {
		got, err := Validate(tt.path, tt.targetOS, "arm64")
		if err != nil || got.Arch != "arm64" {
			t.Fatalf("Validate(%s): got %+v err=%v", tt.targetOS, got, err)
		}
	}
	if _, err := Validate(writeELF(t, root, "unknown-os", 62), "plan9", "amd64"); err == nil || !strings.Contains(err.Error(), "unsupported target OS") {
		t.Fatalf("expected unsupported OS error, got %v", err)
	}
}

func writeELF(t *testing.T, dir, name string, machine uint16) string {
	t.Helper()
	data := make([]byte, 64)
	copy(data, []byte{0x7f, 'E', 'L', 'F'})
	data[4], data[5] = 2, 1
	binary.LittleEndian.PutUint16(data[18:20], machine)
	return writeBinary(t, dir, name, data)
}

func writePE(t *testing.T, dir, name string, machine uint16) string {
	t.Helper()
	data := make([]byte, 128)
	copy(data, []byte{'M', 'Z'})
	binary.LittleEndian.PutUint32(data[0x3c:0x40], 0x40)
	copy(data[0x40:0x44], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(data[0x44:0x46], machine)
	return writeBinary(t, dir, name, data)
}

func writeMachO(t *testing.T, dir, name string, cpu uint32) string {
	t.Helper()
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data[0:4], 0xfeedfacf)
	binary.LittleEndian.PutUint32(data[4:8], cpu)
	return writeBinary(t, dir, name, data)
}

func writeBinary(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
