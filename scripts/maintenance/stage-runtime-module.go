// Command stage-runtime-module creates a deterministic file-backed Go proxy
// entry for source-tree clean-consumer verification. It uses only stdlib.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const modulePath = "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"

func main() {
	if len(os.Args) != 4 {
		panic("usage: stage-runtime-module MODULE_ROOT PROXY_ROOT VERSION")
	}
	root, proxy, version := os.Args[1], os.Args[2], os.Args[3]
	target := filepath.Join(proxy, filepath.FromSlash(modulePath), "@v")
	if err := os.MkdirAll(target, 0o755); err != nil {
		panic(err)
	}
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		panic(err)
	}
	goMod = canonicalText(goMod)
	if err := os.WriteFile(filepath.Join(target, version+".mod"), goMod, 0o644); err != nil {
		panic(err)
	}
	info, _ := json.Marshal(struct {
		Version string
		Time    time.Time
	}{version, time.Unix(0, 0).UTC()})
	if err := os.WriteFile(filepath.Join(target, version+".info"), append(info, '\n'), 0o644); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(target, "list"), []byte(version+"\n"), 0o644); err != nil {
		panic(err)
	}
	archive, err := os.Create(filepath.Join(target, version+".zip"))
	if err != nil {
		panic(err)
	}
	writer := zip.NewWriter(archive)
	prefix := modulePath + "@" + version + "/"
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if strings.HasPrefix(filepath.Base(relative), "coverage-") {
			return nil
		}
		if entry.Type()&fs.ModeType != 0 {
			return fmt.Errorf("non-regular module file: %s", relative)
		}
		header := &zip.FileHeader{Name: prefix + relative, Method: zip.Deflate}
		header.SetMode(0o644)
		header.SetModTime(time.Unix(0, 0).UTC())
		output, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if isTextModuleFile(relative) {
			content = canonicalText(content)
		}
		_, err = output.Write(content)
		return err
	})
	if err != nil {
		_ = writer.Close()
		_ = archive.Close()
		panic(err)
	}
	if err := writer.Close(); err != nil {
		_ = archive.Close()
		panic(err)
	}
	if err := archive.Close(); err != nil {
		panic(err)
	}
}

// canonicalText matches the LF-normalized bytes stored by Git, so a proxy
// staged from Windows has the same module checksums as one staged on Unix.
func canonicalText(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}

// isTextModuleFile identifies source formats whose Git-canonical representation
// uses LF line endings and can therefore be normalized without altering data.
func isTextModuleFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".json", ".md", ".mod", ".sha256", ".sum", ".txt", ".yaml", ".yml":
		return true
	default:
		return false
	}
}
