package effectiveconfig

import (
	"strings"
	"testing"
)

// TestFileBearingProperties implements SP02/SP03's independent byte-decoding
// and precedence vectors. Generic ordinary-property semantics must not change.
func TestFileBearingProperties(t *testing.T) {
	for _, tc := range []struct {
		name, input, search string
		invalid             bool
	}{
		{"prefixed_empty", "liquibase.searchPath=\nsearchPath=db\n", "db", false},
		{"prefixed_empty_reverse", "searchPath=db\nliquibase.searchPath=\n", "db", false},
		{"prefixed_wins", "searchPath=shared\nliquibase.searchPath=db\n", "db", false},
		{"source_trim", "searchPath=db, \n", "db,", false},
		{"escaped_blank", "liquibase.searchPath=\\ \nsearchPath=db\n", "db", false},
		{"last_assignment", "searchPath=shared\nsearchPath=db\n", "db", false},
		{"byte_mapping", "searchPath=café\n", "cafÃ©", false},
		{"unicode_escape", `searchPath=caf\u00e9`, "café", false},
		{"continuation", "searchPath=d\\\n b\n", "db", false},
		{"explicit_none", "searchPath=,,\n", ",,", false},
		{"no_selection", "# comment\nliquibase.command.contextFilter=dev\n", "", false},
		{"malformed_utf8", "searchPath=\xff\n", "", true},
		{"unsupported_escape", `searchPath=\q`, "", true},
		{"protected", "liquibase.command.password=secret\n", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			properties, err := ParseFileBearingProperties(strings.NewReader(tc.input))
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid source accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := SelectedSearchPath(properties); got != tc.search {
				t.Fatalf("selected search=%q want=%q", got, tc.search)
			}
		})
	}
	const raw = "searchPath=café\nliquibase.command.contextFilter=café  \n"
	ordinary, err := ParseProperties(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	adapted, err := ParseFileBearingProperties(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if ordinary["searchPath"] != "café" || adapted["searchPath"] != "cafÃ©" || ordinary["liquibase.command.contextFilter"] != adapted["liquibase.command.contextFilter"] {
		t.Fatal("adapter changed generic decoding or ordinary settings")
	}
}
