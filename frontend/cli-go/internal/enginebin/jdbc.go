package enginebin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const (
	JDBCDriverFileName = "postgresql-42.7.13.jar"
	JDBCDriverDigest   = "sha256:6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93"
)

// ReadBundledJDBC reads only the release-adjacent driver whose checksum is
// fixed by docs/adr/2026-10-06-postgresql-jdbc-artifact-delivery.md. A nil
// result indicates a missing bundle, which the WSL installer rejects.
func ReadBundledJDBC(enginePath string) ([]byte, error) {
	return readBundledJDBCWithDigest(enginePath, JDBCDriverDigest)
}

func readBundledJDBCWithDigest(enginePath, expected string) ([]byte, error) {
	if enginePath == "" {
		return nil, fmt.Errorf("engine path is required")
	}
	absolute, err := filepath.Abs(enginePath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(absolute)
	candidates := []string{filepath.Join(dir, "lib", JDBCDriverFileName)}
	if filepath.Base(filepath.Dir(dir)) == "libexec" {
		candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(dir)), "lib", JDBCDriverFileName))
	}
	for _, candidate := range candidates {
		info, err := os.Lstat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("bundled JDBC driver is not a regular file")
		}
		data, err := os.ReadFile(candidate)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		if "sha256:"+hex.EncodeToString(sum[:]) != expected {
			return nil, fmt.Errorf("bundled JDBC driver checksum mismatch")
		}
		return data, nil
	}
	return nil, nil
}
