package benchmark

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestReleasedNativePathsUseProjectProcessDirectory(t *testing.T) {
	root := t.TempDir()
	for _, item := range []struct{ name, dir, file string }{{"jhipster", "jhipster-sample-app", "config/liquibase/master.xml"}, {"mobilitydata-user", "mobilitydata", "changelog_user.xml"}, {"mobilitydata-main", "mobilitydata", "changelog.xml"}} {
		args := []string{"update", "--searchPath", item.dir, "--changelog-file", item.dir + "/" + item.file}
		original := append([]string(nil), args...)
		req, err := hostRequest(Request{WorkDir: root, LiquibaseArgs: args}, item.name)
		if err != nil || req.WorkDir != filepath.Join(root, item.dir) || req.LiquibaseArgs[2] != "." || req.LiquibaseArgs[4] != item.file {
			t.Fatal("native root/arguments differ", item.name)
		}
		if !reflect.DeepEqual(args, original) {
			t.Fatal("input request changed")
		}
	}
	if _, err := hostRequest(Request{WorkDir: root}, "private"); err == nil {
		t.Fatal("unsupported fixture accepted")
	}
	if _, err := hostRequest(Request{WorkDir: "relative"}, "jhipster"); err == nil {
		t.Fatal("relative input root accepted")
	}
}
