package benchmark

import (
	"context"
	"debug/buildinfo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactIdentity(t *testing.T) {
	info := &buildinfo.BuildInfo{}
	if validateArtifact(info, strings.Repeat("a", 40)) == nil {
		t.Fatal("missing metadata accepted")
	}
}

func TestOwnedCleanupRejectsOutsideAndInvalidIDs(t *testing.T) {
	root := t.TempDir()
	if isWithin(root, filepath.Dir(root)) || isWithin(root, root+"-other") {
		t.Fatal("outside root accepted")
	}
	if !isWithin(root, filepath.Join(root, "store")) {
		t.Fatal("owned directory refused")
	}
	calls := 0
	run := func(_ context.Context, args ...string) ([]byte, error) { calls++; return []byte("bad-id"), nil }
	if cleanupDockerE2EContainers(context.Background(), root, run) == nil || calls != 1 {
		t.Fatal("invalid ID reached removal")
	}
	if cleanupDockerE2EContainers(context.Background(), "relative", run) == nil {
		t.Fatal("relative root accepted")
	}
}

func TestDockerCounterRejectsBrokenAndCountsExactInterval(t *testing.T) {
	r := &benchmarkDockerCreates{events: []benchmarkContainerCreate{{at: 10, image: "lb", name: "sqlrs-lb-1"}, {at: 11, image: "lb", name: "inspection"}, {at: 30, image: "db", name: "outside"}}}
	count, probes, err := r.countNano(10, 20, "lb")
	if err != nil || count != 2 || probes != 1 {
		t.Fatal("interval count differs")
	}
}

func TestFullOracleHasExpectedChangesAndSchema(t *testing.T) {
	for _, row := range []struct {
		name, prefix string
		count        int
	}{{"jhipster", "change|", 8}, {"mobilitydata-user", "changelog|", 12}, {"mobilitydata-main", "changelog|", 102}} {
		data, err := os.ReadFile(filepath.Join("fixtures", row.name+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		schema := false
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, row.prefix) {
				count++
			}
			if strings.HasPrefix(line, "schema|") || strings.HasPrefix(line, "column|") {
				schema = true
			}
		}
		if count != row.count || !schema {
			t.Fatal("incomplete independent oracle", row.name)
		}
	}
}

func TestReleasedEngine(t *testing.T) { collect(t) }
