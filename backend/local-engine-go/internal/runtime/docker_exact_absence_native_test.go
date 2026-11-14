package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"testing"
	"time"
)

// exactAbsenceNativeParent owns only a random label and its recorded full ID.
// Cleanup inventories by ownership, not the original name: rename is the case
// being tested, so a name-filtered cleanup could leak the very target it owns.
func exactAbsenceNativeParent(t *testing.T, ctx context.Context, image string) (string, string) {
	t.Helper()
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	owner := hex.EncodeToString(nonce[:])
	name := "sqlrs-lb-exact-" + owner
	const label = "sqlrs.exact-absence-test"
	var id string
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		data, err := exec.CommandContext(cleanup, "docker", "container", "ls", "-a", "--no-trunc", "--filter", "label="+label+"="+owner, "--format", "{{.ID}}").Output()
		if err != nil {
			t.Error("owned exact-ID cleanup inventory unavailable")
			return
		}
		target := strings.TrimSpace(string(data))
		if target == "" {
			return
		}
		if !createdContainerID.MatchString(target) || id != "" && target != id {
			t.Error("owned cleanup identity changed; refusing removal")
			return
		}
		proof, err := exec.CommandContext(cleanup, "docker", "inspect", "--format", `{{index .Config.Labels "sqlrs.exact-absence-test"}}`, target).Output()
		if err != nil || strings.TrimSpace(string(proof)) != owner {
			t.Error("owned cleanup label changed; refusing removal")
			return
		}
		// Rescue cleanup is test-only, not evidence of non-forced product removal.
		if err := exec.CommandContext(cleanup, "docker", "rm", "-f", "-v", target).Run(); err != nil {
			t.Error("owned exact-ID cleanup failed")
			return
		}
		left, err := exec.CommandContext(cleanup, "docker", "container", "ls", "-a", "--no-trunc", "--filter", "id="+target, "--format", "{{.ID}}").Output()
		if err != nil || strings.TrimSpace(string(left)) != "" {
			t.Error("owned cleanup absence unproved")
		}
	})
	data, err := exec.CommandContext(ctx, "docker", "create", "--name", name, "--label", label+"="+owner, "--network", "none", "--user", "0", "--entrypoint", "/bin/sh", image, "-c", "exec sleep 3600").Output()
	if err != nil {
		t.Fatal("cannot create owned exact-ID fixture")
	}
	id = strings.TrimSpace(string(data))
	if !createdContainerID.MatchString(id) {
		t.Fatal("fixture did not return full ID")
	}
	if err := exec.CommandContext(ctx, "docker", "start", id).Run(); err != nil {
		t.Fatal("cannot start owned exact-ID fixture")
	}
	return name, id
}

// EA11 transport-level D evidence only: real Docker must find renamed running
// AND stopped owners by full ID, then prove actual absence after exact rm.
// This does not qualify SQL/DB/host-cleanup or the whole EA11 coordinator matrix.
func TestDockerExactAbsenceRenamedContainerDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("select Docker suite with SQLRS_LIQUIBASE_DOCKER_E2E=1")
	}
	if goruntime.GOOS == "linux" && os.Geteuid() == 0 {
		t.Fatal("native Linux absence check requires an ordinary engine user")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	if err != nil || !validCaptureDigest(strings.TrimSpace(string(imageBytes))) {
		t.Fatal("installed pinned Liquibase fixture image unavailable")
	}
	image := strings.TrimSpace(string(imageBytes))
	name, parent := exactAbsenceNativeParent(t, ctx, image)
	otherName, other := exactAbsenceNativeParent(t, ctx, image)
	if parent == other {
		t.Fatal("unrelated control shares parent identity")
	}
	if err := exec.CommandContext(ctx, "docker", "rename", parent, name+"-renamed").Run(); err != nil {
		t.Fatal("cannot rename owned fixture")
	}
	rt := NewDocker(Options{})
	for _, status := range []string{"running", "exited"} {
		if status == "exited" {
			if err := exec.CommandContext(ctx, "docker", "stop", "--time", "1", parent).Run(); err != nil {
				t.Fatal("cannot stop exact owned fixture")
			}
		}
		if _, found, err := rt.InspectNamedContainer(ctx, name); err != nil || found {
			t.Fatal("planned-name absence not independently reached", err)
		}
		view, found, err := rt.InspectNamedContainer(ctx, name+"-renamed")
		if err != nil || !found || view.ID != parent || view.Status != status {
			t.Fatal("renamed full-ID/status control not reached", err)
		}
		if err := rt.VerifyContainerAbsent(ctx, parent); err == nil {
			t.Fatal("surviving renamed owner treated as absent", status)
		}
	}
	if err := rt.RemoveStoppedContainer(ctx, parent); err != nil {
		t.Fatal("exact non-forced removal failed", err)
	}
	// Independent raw Docker inventory, not the implementation under test.
	left, err := exec.CommandContext(ctx, "docker", "container", "ls", "-a", "--no-trunc", "--filter", "id="+parent, "--format", "{{.ID}}").Output()
	if err != nil || strings.TrimSpace(string(left)) != "" {
		t.Fatal("native removal not independently confirmed")
	}
	if err := rt.VerifyContainerAbsent(ctx, parent); err != nil {
		t.Fatal("removed exact ID did not qualify absence", err)
	}
	control, found, err := rt.InspectNamedContainer(ctx, otherName)
	if err != nil || !found || control.ID != other || control.Status != "running" {
		t.Fatal("unrelated owned control changed", err)
	}
	if err := rt.VerifyContainerAbsent(ctx, other); err == nil {
		t.Fatal("unrelated surviving owner treated as absent")
	}
}
