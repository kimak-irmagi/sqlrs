package driverbundle

import (
	"strings"
	"testing"
)

// D01a/D01b: the catalog selects exactly one PostgreSQL driver for a
// supported Liquibase/Java image family, before any container step runs.
func TestCatalogSelectsOneVerifiedPostgreSQLDriver(t *testing.T) {
	embeddedDigest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name      string
		inventory ImageInventory
		wantMode  string
		wantHash  string
	}{
		{"community 5 without bundled driver", ImageInventory{LiquibaseVersion: "5.0.4", JavaMajor: 21}, "mounted", DriverDigest},
		{"community 4 embedded driver", ImageInventory{LiquibaseVersion: "4.33.0", JavaMajor: 17, PostgreSQLJars: []EmbeddedDriver{{Path: "/liquibase/internal/lib/postgresql.jar", Digest: embeddedDigest}}}, "embedded", embeddedDigest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Select("postgresql", tc.inventory)
			if err != nil || got.Mode != tc.wantMode || got.Digest != tc.wantHash {
				t.Fatalf("Select() = %+v, %v", got, err)
			}
		})
	}
}

func TestCatalogFailsClosedForUnsupportedOrDuplicateDriver(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name      string
		database  string
		inventory ImageInventory
	}{
		{"H2 target", "h2", ImageInventory{LiquibaseVersion: "5.0.4", JavaMajor: 21}},
		{"future family", "postgresql", ImageInventory{LiquibaseVersion: "6.0.0", JavaMajor: 21}},
		{"old Java", "postgresql", ImageInventory{LiquibaseVersion: "4.33.0", JavaMajor: 7}},
		{"unknown Java", "postgresql", ImageInventory{LiquibaseVersion: "5.0.4"}},
		{"duplicate driver", "postgresql", ImageInventory{LiquibaseVersion: "5.0.4", JavaMajor: 21, PostgreSQLJars: []EmbeddedDriver{{Path: "/liquibase/lib/one.jar", Digest: digest}, {Path: "/liquibase/lib/two.jar", Digest: digest}}}},
		{"bad embedded path", "postgresql", ImageInventory{LiquibaseVersion: "5.0.4", JavaMajor: 21, PostgreSQLJars: []EmbeddedDriver{{Path: "/tmp/postgresql.jar", Digest: digest}}}},
		{"bad embedded hash", "postgresql", ImageInventory{LiquibaseVersion: "5.0.4", JavaMajor: 21, PostgreSQLJars: []EmbeddedDriver{{Path: "/liquibase/lib/postgresql.jar", Digest: "unverified"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Select(tc.database, tc.inventory); err == nil {
				t.Fatal("unsupported driver combination was accepted")
			}
		})
	}
}
