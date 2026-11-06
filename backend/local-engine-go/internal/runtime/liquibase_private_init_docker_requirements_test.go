package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// PC04/05/16: run the pinned v2 initializer in an ordinary engine user's
// actual Docker environment. There is no DB or SQL child; initialization must
// produce its own private marker, rather than a test writing readiness for it.
func TestLiquibasePrivateInitializerDocker(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_DOCKER_E2E") != "1" {
		t.Skip("native initialization suite not selected")
	}
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		t.Fatal("ordinary Linux engine UID required")
	}
	helper, err := os.ReadFile("assets/liquibase-step-capture-v2.sh")
	if err != nil {
		t.Fatalf("approved v2 initializer unavailable: %v", err)
	}
	helperPath := filepath.Join(t.TempDir(), "step-capture.sh")
	if err := os.WriteFile(helperPath, helper, 0444); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	imageBytes, err := exec.CommandContext(ctx, "docker", "image", "inspect", "liquibase/liquibase:5.0.4", "--format", "{{.Id}}").Output()
	if err != nil {
		t.Fatal("required native image unavailable")
	}
	image := strings.TrimSpace(string(imageBytes))
	if !validCaptureDigest(image) {
		t.Fatal("unresolved native image identity")
	}
	for _, variant := range []string{"control", "helper_digest", "generation", "session_token", "config_digest", "untrusted_uid", "root_symlink", "parent_symlink", "root_mode", "parent_writable", "root_owner", "ready_corrupt"} {
		t.Run(variant, func(t *testing.T) {
			want := SessionReadinessIdentity{SessionID: "private-init-session", WorkerGeneration: 2,
				RunnerConfigDigest: "sha256:" + strings.Repeat("b", 64), HelperDigest: captureDigest(helper)}
			digest, generation, session, config, user := want.HelperDigest, "2", want.SessionID, want.RunnerConfigDigest, "0"
			setup := ":"
			switch variant {
			case "helper_digest":
				digest = "sha256:" + strings.Repeat("d", 64)
			case "generation":
				generation = "0"
			case "session_token":
				session = "../foreign"
			case "config_digest":
				config = "private-invalid-value"
			case "untrusted_uid":
				user = "1001"
			case "root_symlink":
				setup = "mkdir -m 700 /private-target; ln -s /private-target /sqlrs/capture"
			case "parent_symlink":
				setup = "mkdir -m 755 /private-parent; rmdir /sqlrs; ln -s /private-parent /sqlrs"
			case "root_mode":
				setup = "mkdir -m 777 /sqlrs/capture"
			case "parent_writable":
				setup = "chmod 777 /sqlrs"
			case "root_owner":
				setup = "mkdir -m 700 /sqlrs/capture; chown 1001:1001 /sqlrs/capture"
			case "ready_corrupt":
				setup = "mkdir -m 700 /sqlrs/capture; umask 077; printf '{}' > /sqlrs/capture/ready.complete"
			}
			parentSetup := "mkdir -p /sqlrs"
			if variant == "untrusted_uid" {
				parentSetup = ":"
			}
			args := []string{"run", "-d", "--network", "none", "--user", user, "--entrypoint", "/bin/sh",
				"-v", dockerBindSpec(helperPath, "/private-helper/step-capture.sh", true),
				"-e", "SQLRS_SESSION_ID=" + session, "-e", "SQLRS_WORKER_GENERATION=" + generation,
				"-e", "SQLRS_RUNNER_CONFIG_DIGEST=" + config, "-e", "SQLRS_HELPER_DIGEST=" + digest,
				image, "-c", "set -eu; " + parentSetup + "; " + setup + "; exec /bin/sh /private-helper/step-capture.sh serve"}
			_, runErr, name, id := runRetainedCaptureTestContainer(t, ctx, args)
			if runErr != nil || !createdContainerID.MatchString(id) {
				t.Fatal("owned initializer failed to launch")
			}
			want.RunnerID = id
			rt := NewDocker(Options{Binary: "docker"})
			observeCtx, observeCancel := context.WithTimeout(ctx, 15*time.Second)
			defer observeCancel()
			var proof SessionReadiness
			for {
				view, found, inspectErr := rt.InspectNamedContainer(observeCtx, name)
				if inspectErr != nil || !found || view.ID != id {
					t.Fatal("initializer identity unavailable")
				}
				if variant != "control" && view.Status == "exited" {
					if view.ExitCode != 90 {
						t.Fatalf("initializer did not reject safely: exit=%d", view.ExitCode)
					}
					if got, err := rt.ReadLiquibaseSessionReadiness(observeCtx, name, want); err == nil || got != (SessionReadiness{}) {
						t.Fatal("rejected initializer returned readiness")
					}
					return
				}
				if variant == "control" {
					var readErr error
					proof, readErr = rt.ReadLiquibaseSessionReadiness(observeCtx, name, want)
					if readErr == nil {
						break
					}
					if view.Status == "exited" {
						t.Fatalf("valid initializer exited: code=%d", view.ExitCode)
					}
				}
				if observeCtx.Err() != nil {
					t.Fatal("initializer did not reach expected state within bound")
				}
				time.Sleep(25 * time.Millisecond)
			}
			if proof.ReadyBytes < 1 || proof.ReadyBytes > 8192 || !validCaptureDigest(proof.ReadyDigest) {
				t.Fatal("unbounded/invalid readiness evidence")
			}
			// Real ownership and path privacy, rather than just an unreadable
			// missing file. Neither chmod nor chown repairs the host.
			check := `test "$(stat -c '%u:%a' /sqlrs/capture)" = '0:700' && test "$(stat -c '%u:%a' /sqlrs/capture/ready.complete)" = '0:600' && test "$(find /sqlrs/capture -mindepth 1 -maxdepth 1 | wc -l)" = 1`
			if err := exec.CommandContext(ctx, "docker", "exec", "--user", "0", id, "/bin/sh", "-c", check).Run(); err != nil {
				t.Fatal("initializer ownership/modes or no-child invariant failed")
			}
			if err := exec.CommandContext(ctx, "docker", "exec", "--user", "1001", id, "/bin/sh", "-c", "test ! -r /sqlrs/capture/ready.complete && test ! -w /sqlrs/capture").Run(); err != nil {
				t.Fatal("untrusted child can access readiness root")
			}
			again, err := rt.ReadLiquibaseSessionReadiness(ctx, name, want)
			if err != nil || again != proof {
				t.Fatal("reconciliation did not read unchanged readiness")
			}
		})
	}
}
