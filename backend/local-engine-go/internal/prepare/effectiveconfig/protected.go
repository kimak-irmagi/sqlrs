// Package effectiveconfig builds the engine-owned Liquibase settings file
// described in docs/architecture/liquibase-container-component-structure.md.
package effectiveconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const protectedContainerPath = "/sqlrs/protected/liquibase.properties"

// ProtectedOptions supplies the admitted ordinary settings and the managed
// database connection. Password is never included in errors or mount paths.
type ProtectedOptions struct {
	Root       string
	URL        string
	Username   string
	Password   string
	Properties map[string]string
}

// ProtectedConfig is an ephemeral file and its container-only location. The
// caller must defer Close immediately after a successful CreateProtected.
type ProtectedConfig struct {
	HostPath      string
	ContainerPath string
	directory     string
}

// CreateProtected writes one read-only-to-container defaults file with
// ordinary project settings and engine-owned connection values.
func CreateProtected(options ProtectedOptions) (*ProtectedConfig, error) {
	root := filepath.Clean(options.Root)
	if !filepath.IsAbs(root) || strings.TrimSpace(options.URL) == "" || strings.TrimSpace(options.Username) == "" {
		return nil, fmt.Errorf("absolute protected root, database URL, and username are required")
	}
	keys := make([]string, 0, len(options.Properties))
	for key := range options.Properties {
		if invalidPropertiesKey(key) || protectedProperty(key) {
			return nil, fmt.Errorf("Liquibase property %q cannot override protected settings", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("cannot prepare protected Liquibase root: %w", err)
	}
	dir, err := os.MkdirTemp(root, "lb-protected-")
	if err != nil {
		return nil, fmt.Errorf("cannot create protected Liquibase directory: %w", err)
	}
	file := filepath.Join(dir, "liquibase.properties")
	var body strings.Builder
	for _, key := range keys {
		body.WriteString(key)
		body.WriteByte('=')
		body.WriteString(escapePropertiesValue(options.Properties[key]))
		body.WriteByte('\n')
	}
	for _, pair := range [][2]string{
		{"liquibase.command.url", options.URL},
		{"liquibase.command.username", options.Username},
		{"liquibase.command.password", options.Password},
	} {
		body.WriteString(pair[0])
		body.WriteByte('=')
		body.WriteString(escapePropertiesValue(pair[1]))
		body.WriteByte('\n')
	}
	// The exact file is bind-mounted read-only for the image's unprivileged
	// user. The containing host directory remains private (0700), so host
	// users cannot traverse to the file despite its container-readable mode.
	if err := os.WriteFile(file, []byte(body.String()), 0o644); err != nil {
		_ = os.Remove(dir)
		return nil, fmt.Errorf("cannot write protected Liquibase settings: %w", err)
	}
	if err := os.Chmod(file, 0o444); err != nil {
		_ = os.Remove(file)
		_ = os.Remove(dir)
		return nil, fmt.Errorf("cannot protect Liquibase settings: %w", err)
	}
	return &ProtectedConfig{HostPath: file, ContainerPath: protectedContainerPath, directory: dir}, nil
}

func invalidPropertiesKey(key string) bool {
	if key == "" {
		return true
	}
	for _, c := range key {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' {
			continue
		}
		return true
	}
	return false
}

func protectedProperty(key string) bool {
	key = strings.ToLower(key)
	key = strings.TrimPrefix(key, "liquibase.command.")
	key = strings.TrimPrefix(key, "liquibase.")
	switch key {
	case "url", "username", "password", "driver", "classpath", "referenceurl", "referenceusername", "referencepassword", "referencedriver":
		return true
	default:
		return false
	}
}

func escapePropertiesValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r", "\\r")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "\t", "\\t")
	if strings.HasPrefix(value, " ") {
		value = "\\" + value
	}
	return value
}

// Close removes the exact file and directory created by CreateProtected.
func (config *ProtectedConfig) Close() error {
	if config == nil {
		return nil
	}
	fileErr := os.Remove(config.HostPath)
	if errors.Is(fileErr, os.ErrNotExist) {
		fileErr = nil
	}
	dirErr := os.Remove(config.directory)
	if errors.Is(dirErr, os.ErrNotExist) {
		dirErr = nil
	}
	return errors.Join(fileErr, dirErr)
}
