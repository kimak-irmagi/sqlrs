package runtime

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var runnerJavaVersion = regexp.MustCompile(`(?m)^(?:openjdk|java) version "([0-9]+)(?:\.([0-9]+))?[^"\r\n]*"`)
var runnerLiquibaseVersion = regexp.MustCompile(`(?m)^Liquibase Version: ([^\s\r\n]+)\s*$`)
var runnerJarLine = regexp.MustCompile(`^([0-9a-f]{64})  (/liquibase/(?:lib|internal/lib)/[^\s]+\.jar)$`)

// RunnerJar is a byte-verified JAR discovered in a pinned Liquibase image.
type RunnerJar struct {
	Path   string
	Digest string
}

// RunnerImageInspection is the runtime observation consumed by the engine's
// PostgreSQL driver catalog before planning. It is not inferred from a tag.
type RunnerImageInspection struct {
	LiquibaseVersion string
	JavaMajor        int
	PostgreSQLJars   []RunnerJar
}

// InspectRunnerImage runs only an offline, read-only probe against a trusted
// digest. The image's own CLI reports its version, while sha256sum hashes any
// conventional PostgreSQL JAR in its two Liquibase classpath directories.
func (r *DockerRuntime) InspectRunnerImage(ctx context.Context, binding RunnerImageBinding) (RunnerImageInspection, error) {
	if r == nil || !runnerDigestPattern.MatchString(binding.ManifestDigest) || binding.DigestRef != liquibaseRepository+"@"+binding.ManifestDigest {
		return RunnerImageInspection{}, fmt.Errorf("invalid trusted Liquibase image binding")
	}
	const probe = `set -eu
java -version 2>&1
/liquibase/liquibase --version
printf '__SQLRS_PGJARS__\n'
find /liquibase/lib /liquibase/internal/lib -maxdepth 1 -type f -iname '*postgres*.jar' -exec sha256sum {} \;`
	output, err := r.run(ctx, []string{"run", "--rm", "--network", "none", "--entrypoint", "sh", binding.DigestRef, "-c", probe}, nil)
	if err != nil {
		return RunnerImageInspection{}, fmt.Errorf("cannot inspect pinned Liquibase image: %w", err)
	}
	java := runnerJavaVersion.FindStringSubmatch(output)
	version := runnerLiquibaseVersion.FindStringSubmatch(output)
	if len(java) == 0 || len(version) == 0 {
		return RunnerImageInspection{}, fmt.Errorf("Liquibase image omitted Java or Liquibase version")
	}
	major, err := strconv.Atoi(java[1])
	if err != nil {
		return RunnerImageInspection{}, fmt.Errorf("invalid Liquibase image Java version")
	}
	if major == 1 {
		major, err = strconv.Atoi(java[2])
		if err != nil {
			return RunnerImageInspection{}, fmt.Errorf("invalid Liquibase image Java version")
		}
	}
	result := RunnerImageInspection{LiquibaseVersion: version[1], JavaMajor: major}
	_, inventory, found := strings.Cut(output, "__SQLRS_PGJARS__\n")
	if !found {
		return RunnerImageInspection{}, fmt.Errorf("Liquibase image omitted driver inventory")
	}
	for _, raw := range strings.Split(inventory, "\n") {
		line := strings.TrimSpace(raw)
		if !strings.Contains(strings.ToLower(line), "postgres") || !strings.Contains(strings.ToLower(line), ".jar") {
			continue
		}
		match := runnerJarLine.FindStringSubmatch(line)
		if len(match) == 0 {
			return RunnerImageInspection{}, fmt.Errorf("invalid PostgreSQL JAR inventory from pinned Liquibase image")
		}
		result.PostgreSQLJars = append(result.PostgreSQLJars, RunnerJar{Path: match[2], Digest: "sha256:" + match[1]})
	}
	return result, nil
}
