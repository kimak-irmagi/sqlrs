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

// CT05/CT13: qualify the actual pinned 5.0.3 campaign image too, outside API
// timing. The earlier 5.0.4 proof cannot establish another image's semantics.
func TestCostJHipsterImagePrerequisitesDocker(t *testing.T) {
	if os.Getenv("SQLRS_COST_NATIVE_PROBE") != "1" {
		t.Skip("explicit Actions-only image prerequisite")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("image prerequisite requires Actions")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	const pinned = "liquibase/liquibase@sha256:17f98b85e6d4c33b84ee409d055f1526227c77b55dd30e3879ec5088716a32eb"
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", pinned, "--format", "{{.Id}}").Output()
	image := strings.TrimSpace(string(imageBytes))
	if err != nil || !validCaptureDigest(image) {
		t.Fatal("pinned JHipster image is unavailable")
	}
	const script = `set -eu
IFS=' ' read -r uptime idle < /proc/uptime
case "${uptime#*.}" in [0-9][0-9]) ;; *) exit 71 ;; esac
version=$(sync --version)
IFS=' ' read -r utility gnu coreutils release rest <<EOF
$version
EOF
[ "$utility $gnu $coreutils" = 'sync (GNU coreutils)' ]
hash=$(sha256sum "$(command -v sync)")
mkdir -m 700 /sqlrs-cost-image-probe
cd /sqlrs-cost-image-probe
printf one > stdout.partial
printf two > stderr.partial
device=$(stat -c '%d' .)
[ "$(stat -c '%d' stdout.partial)" = "$device" ]
[ "$(stat -c '%d' stderr.partial)" = "$device" ]
sync -f stdout.partial stderr.partial
sync -f .
mv stdout.partial stdout.complete
mv stderr.partial stderr.complete
sync -f .
printf complete > manifest.partial
sync -f manifest.partial
mv manifest.partial manifest.complete
sync -f .
[ "$(cat stdout.complete)" = one ]
[ "$(cat stderr.complete)" = two ]
[ "$(cat manifest.complete)" = complete ]
printf 'sqlrs-jhipster-cost-image-v1 %s %s\n' "$release" "${hash%% *}"
`
	rt := NewDocker(Options{Binary: "docker"})
	id, err := rt.CreateContainer(ctx, RunRequest{ImageID: image, Name: fmt.Sprintf("sqlrs-step-cost-image-%d", time.Now().UnixNano()), EntryPoint: "/bin/sh", User: "0", Network: "none", Args: []string{"-c", script}})
	if err != nil {
		t.Fatal("image prerequisite create failed")
	}
	t.Cleanup(func() {
		cleanup, finish := context.WithTimeout(context.Background(), 20*time.Second)
		defer finish()
		if err := rt.Stop(cleanup, id); err != nil {
			t.Error("image prerequisite cleanup failed")
		}
		if err := rt.VerifyContainerAbsent(cleanup, id); err != nil {
			t.Error("image prerequisite container retained")
		}
	})
	if err := rt.StartContainer(ctx, id); err != nil {
		t.Fatal("image prerequisite start failed")
	}
	output, exit, err := rt.WaitContainer(ctx, id)
	fields := strings.Fields(output)
	if err != nil || exit != 0 || len(fields) != 3 || fields[0] != "sqlrs-jhipster-cost-image-v1" || !regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`).MatchString(fields[1]) || !validCaptureHex(fields[2]) {
		t.Fatal("JHipster image did not prove coarse clock and same-filesystem GNU sync")
	}
	if err := rt.Stop(ctx, id); err != nil {
		t.Fatal("image prerequisite removal failed before fallback")
	}
	if err := rt.VerifyContainerAbsent(ctx, id); err != nil {
		t.Fatal("image prerequisite leaked before fallback")
	}
	t.Logf("CT13_IMAGE coreutils=%s utility_sha256=%s", fields[1], fields[2])
}
