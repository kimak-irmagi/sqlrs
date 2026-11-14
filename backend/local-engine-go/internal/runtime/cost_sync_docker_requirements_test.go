package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

// CT05 prerequisite: qualify GNU sync -f and the exact containing filesystem
// in the actual pinned image before changing helper flush arguments. This is
// Actions-only functional evidence, not a timing sample or a power-loss test.
func TestCostSyncImplementationDocker(t *testing.T) {
	if os.Getenv("SQLRS_COST_NATIVE_PROBE") != "1" {
		t.Skip("explicit Actions-only cost prerequisite")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("native cost probe must run in Actions, not on the developer host")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	image := strings.TrimSpace(string(imageBytes))
	if err != nil || !validCaptureDigest(image) {
		t.Fatal("native cost probe needs a resolved Liquibase 5.0.4 image")
	}
	const script = `set -eu
version=$(sync --version)
IFS=' ' read -r utility gnu coreutils release rest <<EOF
$version
EOF
[ "$utility $gnu $coreutils" = 'sync (GNU coreutils)' ]
hash=$(sha256sum "$(command -v sync)")
mkdir -m 700 /sqlrs-cost-probe
cd /sqlrs-cost-probe
printf 'one' > stdout.partial
printf 'two' > stderr.partial
device=$(stat -c '%d' .)
[ "$(stat -c '%d' stdout.partial)" = "$device" ]
[ "$(stat -c '%d' stderr.partial)" = "$device" ]
sync -f stdout.partial stderr.partial
sync -f .
mv stdout.partial stdout.complete
mv stderr.partial stderr.complete
sync -f .
printf 'complete' > manifest.partial
sync -f manifest.partial
mv manifest.partial manifest.complete
sync -f .
[ "$(cat stdout.complete)" = one ]
[ "$(cat stderr.complete)" = two ]
[ "$(cat manifest.complete)" = complete ]
printf 'sqlrs-sync-probe-v1 %s %s\n' "$release" "${hash%% *}"
`
	rt := NewDocker(Options{Binary: "docker"})
	name := fmt.Sprintf("sqlrs-step-cost-sync-%d", time.Now().UnixNano())
	id, err := rt.CreateContainer(ctx, RunRequest{ImageID: image, Name: name, EntryPoint: "/bin/sh", User: "0", Network: "none", Args: []string{"-c", script}})
	if err != nil {
		t.Fatal("native sync probe create failed")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		if err := rt.Stop(cleanup, id); err != nil {
			t.Error("exact sync probe cleanup failed")
		}
		if err := rt.VerifyContainerAbsent(cleanup, id); err != nil {
			t.Error("exact sync probe remains after cleanup")
		}
	})
	if err := rt.StartContainer(ctx, id); err != nil {
		t.Fatal("native sync probe start failed")
	}
	output, exit, err := rt.WaitContainer(ctx, id)
	fields := strings.Fields(output)
	if err != nil || exit != 0 || len(fields) != 3 || fields[0] != "sqlrs-sync-probe-v1" ||
		!regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`).MatchString(fields[1]) || !validCaptureHex(fields[2]) {
		t.Fatal("pinned image did not confirm GNU sync -f on the shared filesystem")
	}
	t.Logf("CT05_SYNC image=%s coreutils=%s utility_sha256=%s", image, fields[1], fields[2])
}
