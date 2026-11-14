package config

import (
	"fmt"
)

// RejectLiquibaseRunnerSettings enforces the container-only CLI contract from
// docs/architecture/liquibase-container-component-structure.md. Presence is
// invalid even when the obsolete value is empty or null.
func RejectLiquibaseRunnerSettings(path string) error {
	data, err := readConfigMap(path)
	if err != nil {
		return err
	}
	liquibase, ok := data["liquibase"].(map[string]any)
	if !ok {
		return nil
	}
	for _, key := range []string{"exec", "exec_mode", "env"} {
		if _, present := liquibase[key]; present {
			return fmt.Errorf("liquibase.%s is no longer supported; Liquibase runs in a container", key)
		}
	}
	return nil
}
