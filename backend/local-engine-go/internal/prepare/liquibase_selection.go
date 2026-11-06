package prepare

import (
	"regexp"
	"strings"
)

var liquibaseVersionPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// normalizeLiquibaseVersion interprets the user-authored selector from
// docs/user-guides/sqlrs-liquibase-container-cli-design.md. The returned tag
// is still mutable intent; the container resolver must pin its digest before
// planning, as specified in
// docs/architecture/liquibase-container-component-structure.md.
func normalizeLiquibaseVersion(kind string, version string) (string, error) {
	if kind != "lb" {
		if version != "" {
			return "", ValidationError{Code: "invalid_argument", Message: "liquibase_version is only valid for lb"}
		}
		return "", nil
	}
	if version == "" {
		return "latest", nil
	}
	if strings.TrimSpace(version) != version || !liquibaseVersionPattern.MatchString(version) {
		return "", ValidationError{Code: "invalid_argument", Message: "invalid liquibase_version selector"}
	}
	return version, nil
}
