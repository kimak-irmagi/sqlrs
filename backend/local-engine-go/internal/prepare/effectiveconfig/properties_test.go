package effectiveconfig

import (
	"strings"
	"testing"
)

func TestParsePropertiesOrdinaryValues(t *testing.T) {
	values, err := ParseProperties(strings.NewReader("# comment\nliquibase.command.contextFilter : de\\u0076\nliquibase.command.labelFilter=release\\\n  -candidate\n"))
	if err != nil {
		t.Fatal(err)
	}
	if values["liquibase.command.contextFilter"] != "dev" || values["liquibase.command.labelFilter"] != "release-candidate" {
		t.Fatalf("parsed defaults=%v", values)
	}
}

func TestParsePropertiesRejectsProtectedAndAmbiguousValues(t *testing.T) {
	for _, content := range []string{
		"liquibase.command.password=secret\n",
		"url=jdbc:postgresql://other\n",
		"liquibase.command.contextFilter=bad\\q\n",
		"liquibase.command.contextFilter=bad\\u12\n",
	} {
		if _, err := ParseProperties(strings.NewReader(content)); err == nil {
			t.Fatalf("accepted %q", content)
		}
	}
}
