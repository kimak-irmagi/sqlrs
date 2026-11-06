package driverbundle

import (
	"fmt"
	"regexp"
	"strings"
)

var catalogDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var supportedLiquibaseVersion = regexp.MustCompile(`^[45]\.[0-9]+(?:\.[0-9]+)?(?:[-+][A-Za-z0-9_.-]+)?$`)

// EmbeddedDriver identifies a PostgreSQL JDBC JAR found in the pinned image.
// Digest is over the actual image bytes, not a filename or mutable tag.
type EmbeddedDriver struct {
	Path   string
	Digest string
}

// ImageInventory is the verified Liquibase/Java/driver content of the pinned
// image, collected before planning as specified by the D01 catalog contract
// in docs/architecture/liquibase-container-component-structure.md.
type ImageInventory struct {
	LiquibaseVersion string
	JavaMajor        int
	PostgreSQLJars   []EmbeddedDriver
}

// Select fails closed unless exactly one compatible driver can be bound. A
// mounted result still needs Store.Publish to fill its durable ArtifactRef.
func Select(database string, inventory ImageInventory) (Binding, error) {
	if database != "postgresql" {
		return Binding{}, fmt.Errorf("unsupported Liquibase database target %q", database)
	}
	if !supportedLiquibaseVersion.MatchString(inventory.LiquibaseVersion) || inventory.JavaMajor < 8 {
		return Binding{}, fmt.Errorf("unsupported Liquibase/Java image family %q/%d", inventory.LiquibaseVersion, inventory.JavaMajor)
	}
	if len(inventory.PostgreSQLJars) > 1 {
		return Binding{}, fmt.Errorf("duplicate PostgreSQL JDBC drivers in Liquibase image")
	}
	if len(inventory.PostgreSQLJars) == 1 {
		jar := inventory.PostgreSQLJars[0]
		if !strings.HasPrefix(jar.Path, "/liquibase/") || strings.Contains(jar.Path, "..") || !catalogDigest.MatchString(jar.Digest) {
			return Binding{}, fmt.Errorf("invalid embedded PostgreSQL JDBC driver provenance")
		}
		return Binding{Mode: "embedded", Digest: jar.Digest}, nil
	}
	return Binding{Mode: "mounted", Digest: DriverDigest}, nil
}
