package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestInspectRunnerImageReadsPinnedImageInventory(t *testing.T) {
	imageDigest := "sha256:" + strings.Repeat("a", 64)
	jarDigest := "sha256:" + strings.Repeat("b", 64)
	runner := &fakeRunner{responses: []runResponse{{output: "openjdk version \"21.0.11\"\nLiquibase Version: 5.0.4\n__SQLRS_PGJARS__\n" + strings.TrimPrefix(jarDigest, "sha256:") + "  /liquibase/internal/lib/postgresql.jar\n"}}}
	runtime := NewDocker(Options{Binary: "docker", Runner: runner})
	got, err := runtime.InspectRunnerImage(context.Background(), RunnerImageBinding{ManifestDigest: imageDigest, DigestRef: "liquibase/liquibase@" + imageDigest})
	if err != nil || got.LiquibaseVersion != "5.0.4" || got.JavaMajor != 21 || len(got.PostgreSQLJars) != 1 || got.PostgreSQLJars[0].Digest != jarDigest {
		t.Fatalf("inventory=%+v error=%v", got, err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("Docker calls=%d", len(runner.calls))
	}
	args := strings.Join(runner.calls[0].args, " ")
	for _, required := range []string{"--network none", "--entrypoint sh", "liquibase/liquibase@" + imageDigest} {
		if !strings.Contains(args, required) {
			t.Fatalf("preflight did not inspect pinned offline image: %s", args)
		}
	}
}

func TestInspectRunnerImageRejectsUntrustedOrIncompleteInventory(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		binding RunnerImageBinding
		output  string
	}{
		{RunnerImageBinding{DigestRef: "liquibase/liquibase:latest", ManifestDigest: digest}, ""},
		{RunnerImageBinding{DigestRef: "liquibase/liquibase@" + digest, ManifestDigest: digest}, "Liquibase Version: 5.0.4\n"},
		{RunnerImageBinding{DigestRef: "liquibase/liquibase@" + digest, ManifestDigest: digest}, "openjdk version \"21.0.11\"\n"},
		{RunnerImageBinding{DigestRef: "liquibase/liquibase@" + digest, ManifestDigest: digest}, "openjdk version \"21.0.11\"\nLiquibase Version: 5.0.4\n__SQLRS_PGJARS__\nnot-a-digest /liquibase/lib/postgresql.jar\n"},
	} {
		runner := &fakeRunner{responses: []runResponse{{output: tc.output}}}
		if _, err := NewDocker(Options{Binary: "docker", Runner: runner}).InspectRunnerImage(context.Background(), tc.binding); err == nil {
			t.Fatalf("incomplete or untrusted inspection accepted: %+v", tc)
		}
	}
}
