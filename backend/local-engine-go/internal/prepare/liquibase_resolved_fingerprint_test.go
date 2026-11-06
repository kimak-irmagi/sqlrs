package prepare

import (
	"strings"
	"testing"
)

func resolvedFingerprintFixture() liquibaseResolvedInputs {
	digest := func(char string) string { return "sha256:" + strings.Repeat(char, 64) }
	return liquibaseResolvedInputs{
		DeclarationDigest:     digest("a"),
		RequestedTag:          "latest",
		DBImageDigest:         digest("b"),
		TargetIdentityDigest:  digest("c"),
		PlatformOS:            "linux",
		PlatformArch:          "amd64",
		ImageManifestDigest:   digest("d"),
		DriverMode:            "embedded",
		DriverDigest:          digest("e"),
		BundleManifestDigest:  digest("f"),
		EffectiveConfigDigest: digest("0"),
		PolicyRevision:        "v1",
	}
}

// F01a: every result-relevant resolved input changes the versioned domain.
// The same pinned values are deterministic, including a mutable user tag.
func TestLiquibaseResolvedFingerprintMatrix(t *testing.T) {
	base := resolvedFingerprintFixture()
	want, err := liquibaseResolvedFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := liquibaseResolvedFingerprint(base); err != nil || got != want {
		t.Fatalf("identical binding changed: %q %v", got, err)
	}
	if !strings.HasPrefix(want, "sha256:") || len(want) != 71 {
		t.Fatalf("noncanonical fingerprint: %q", want)
	}
	changes := map[string]func(*liquibaseResolvedInputs){
		"declaration": func(v *liquibaseResolvedInputs) { v.DeclarationDigest = "sha256:" + strings.Repeat("1", 64) },
		"tag":         func(v *liquibaseResolvedInputs) { v.RequestedTag = "5.0.1" },
		"db image":    func(v *liquibaseResolvedInputs) { v.DBImageDigest = "sha256:" + strings.Repeat("2", 64) },
		"target":      func(v *liquibaseResolvedInputs) { v.TargetIdentityDigest = "sha256:" + strings.Repeat("3", 64) },
		"os":          func(v *liquibaseResolvedInputs) { v.PlatformOS = "windows" },
		"arch":        func(v *liquibaseResolvedInputs) { v.PlatformArch = "arm64" },
		"variant":     func(v *liquibaseResolvedInputs) { v.PlatformVariant = "v8" },
		"manifest":    func(v *liquibaseResolvedInputs) { v.ImageManifestDigest = "sha256:" + strings.Repeat("4", 64) },
		"driver mode": func(v *liquibaseResolvedInputs) { v.DriverMode = "mounted"; v.DriverArtifactRef = "artifact" },
		"driver":      func(v *liquibaseResolvedInputs) { v.DriverDigest = "sha256:" + strings.Repeat("5", 64) },
		"bundle":      func(v *liquibaseResolvedInputs) { v.BundleManifestDigest = "sha256:" + strings.Repeat("6", 64) },
		"settings":    func(v *liquibaseResolvedInputs) { v.EffectiveConfigDigest = "sha256:" + strings.Repeat("7", 64) },
		"policy":      func(v *liquibaseResolvedInputs) { v.PolicyRevision = "v2" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			input := base
			change(&input)
			got, err := liquibaseResolvedFingerprint(input)
			if err != nil || got == want {
				t.Fatalf("fingerprint did not change: %q %v", got, err)
			}
		})
	}
}

func TestLiquibaseResolvedFingerprintRejectsUnpinnedInputs(t *testing.T) {
	for name, change := range map[string]func(*liquibaseResolvedInputs){
		"missing manifest":             func(v *liquibaseResolvedInputs) { v.ImageManifestDigest = "" },
		"invalid digest":               func(v *liquibaseResolvedInputs) { v.BundleManifestDigest = "sha256:NOTHEX" },
		"missing target":               func(v *liquibaseResolvedInputs) { v.TargetIdentityDigest = "" },
		"invalid tag":                  func(v *liquibaseResolvedInputs) { v.RequestedTag = "liquibase/liquibase:latest" },
		"missing platform":             func(v *liquibaseResolvedInputs) { v.PlatformArch = "" },
		"unknown driver mode":          func(v *liquibaseResolvedInputs) { v.DriverMode = "host" },
		"missing mounted artifact":     func(v *liquibaseResolvedInputs) { v.DriverMode = "mounted" },
		"unexpected embedded artifact": func(v *liquibaseResolvedInputs) { v.DriverArtifactRef = "artifact" },
		"missing policy":               func(v *liquibaseResolvedInputs) { v.PolicyRevision = "" },
	} {
		t.Run(name, func(t *testing.T) {
			input := resolvedFingerprintFixture()
			change(&input)
			if _, err := liquibaseResolvedFingerprint(input); err == nil {
				t.Fatal("unresolved execution was fingerprinted")
			}
		})
	}
}
