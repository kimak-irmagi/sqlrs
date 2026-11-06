package prepare

import (
	"fmt"
	"regexp"
	"strings"
)

// lb-container-v2 is the approved fingerprint domain, separate from host states.
// A hash is identity, not permission to replay: the main #89 delivery does not
// enable automatic retry. Recovery authority remains a separate journal policy.
// See docs/architecture/liquibase-container-component-structure.md.
const liquibaseContainerFingerprintDomain = "lb-container-v2"

var canonicalSHA256 = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// liquibaseResolvedInputs are the non-secret, pinned execution inputs from
// docs/architecture/liquibase-container-component-structure.md. The index
// digest and artifact storage reference are provenance, but the selected
// platform manifest and verified driver bytes determine execution behavior.
type liquibaseResolvedInputs struct {
	DeclarationDigest     string
	RequestedTag          string
	DBImageDigest         string
	TargetIdentityDigest  string
	PlatformOS            string
	PlatformArch          string
	PlatformVariant       string
	ImageManifestDigest   string
	DriverMode            string
	DriverDigest          string
	DriverArtifactRef     string
	BundleManifestDigest  string
	EffectiveConfigDigest string
	PolicyRevision        string
}

// liquibaseResolvedFingerprint separates container execution from the legacy
// host-runner hash domain. It rejects unpinned or malformed inputs before any
// planning or cache lookup; identical fields always produce one canonical
// sha256 digest. See the resolved-input invariants in
// docs/architecture/liquibase-container-component-structure.md.
func liquibaseResolvedFingerprint(input liquibaseResolvedInputs) (string, error) {
	for _, field := range [][2]string{
		{"declaration", input.DeclarationDigest},
		{"database image", input.DBImageDigest},
		{"target identity", input.TargetIdentityDigest},
		{"runner manifest", input.ImageManifestDigest},
		{"driver", input.DriverDigest},
		{"bundle manifest", input.BundleManifestDigest},
		{"effective config", input.EffectiveConfigDigest},
	} {
		if !canonicalSHA256.MatchString(field[1]) {
			return "", fmt.Errorf("invalid %s digest for Liquibase execution", field[0])
		}
	}
	if input.RequestedTag == "" {
		return "", fmt.Errorf("Liquibase runner tag is required")
	}
	if _, err := normalizeLiquibaseVersion("lb", input.RequestedTag); err != nil {
		return "", err
	}
	for _, field := range [][2]string{
		{"platform OS", input.PlatformOS},
		{"platform arch", input.PlatformArch},
		{"policy revision", input.PolicyRevision},
	} {
		if field[1] == "" || strings.TrimSpace(field[1]) != field[1] {
			return "", fmt.Errorf("invalid %s for Liquibase execution", field[0])
		}
	}
	if input.PlatformVariant != strings.TrimSpace(input.PlatformVariant) {
		return "", fmt.Errorf("invalid platform variant for Liquibase execution")
	}
	switch input.DriverMode {
	case "embedded":
		if input.DriverArtifactRef != "" {
			return "", fmt.Errorf("embedded Liquibase driver cannot have an artifact reference")
		}
	case "mounted":
		if strings.TrimSpace(input.DriverArtifactRef) == "" {
			return "", fmt.Errorf("mounted Liquibase driver needs an artifact reference")
		}
	default:
		return "", fmt.Errorf("invalid Liquibase driver mode")
	}
	hasher := newStateHasher()
	hasher.write("domain", liquibaseContainerFingerprintDomain)
	for _, field := range [][2]string{
		{"declaration_digest", input.DeclarationDigest},
		{"requested_tag", input.RequestedTag},
		{"db_image_digest", input.DBImageDigest},
		{"target_identity_digest", input.TargetIdentityDigest},
		{"platform_os", input.PlatformOS},
		{"platform_arch", input.PlatformArch},
		{"platform_variant", input.PlatformVariant},
		{"image_manifest_digest", input.ImageManifestDigest},
		{"driver_mode", input.DriverMode},
		{"driver_digest", input.DriverDigest},
		{"bundle_manifest_digest", input.BundleManifestDigest},
		{"effective_config_digest", input.EffectiveConfigDigest},
		{"policy_revision", input.PolicyRevision},
	} {
		hasher.write(field[0], field[1])
	}
	return "sha256:" + hasher.sum(), nil
}
