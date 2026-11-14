package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// C02b: obsolete runner settings are forbidden by presence, including null,
// empty strings, and empty maps; unrelated project configuration still works.
func TestRejectLiquibaseRunnerSettingsByPresence(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		path string
	}{
		{name: "exec empty", yaml: "liquibase:\n  exec: ''\n", path: "liquibase.exec"},
		{name: "exec null", yaml: "liquibase:\n  exec: null\n", path: "liquibase.exec"},
		{name: "exec mode empty", yaml: "liquibase:\n  exec_mode: ''\n", path: "liquibase.exec_mode"},
		{name: "env empty", yaml: "liquibase:\n  env: {}\n", path: "liquibase.env"},
		{name: "ordinary", yaml: "dbms:\n  image: postgres:17\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			err := RejectLiquibaseRunnerSettings(path)
			if tc.path == "" {
				if err != nil {
					t.Fatalf("ordinary config rejected: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("removed setting %s accepted: %v", tc.path, err)
			}
		})
	}
}
