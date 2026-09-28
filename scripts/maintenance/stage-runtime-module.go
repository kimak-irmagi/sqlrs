// Command stage-runtime-module creates a deterministic file-backed Go proxy
// entry for source-tree clean-consumer verification. It uses only stdlib.
package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
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
		status, err := entry.Info()
		if err != nil {
			return err
		}
		if !status.Mode().IsRegular() {
			return fmt.Errorf("non-regular module file: %s", relative)
		}
		header, err := zip.FileInfoHeader(status)
		if err != nil {
			return err
		}
		header.Name = prefix + relative
		header.Method = zip.Deflate
		header.SetModTime(time.Unix(0, 0).UTC())
		output, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(name)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
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
