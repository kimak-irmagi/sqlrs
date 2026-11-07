package prepare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sqlrs/engine-local/internal/prepare/driverbundle"
	"github.com/sqlrs/engine-local/internal/prepare/effectiveconfig"
	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// Engine-owned capabilities; no request can select their implementations.
// See liquibase-container-component-structure.md and the #89 installation boundary.
type LiquibaseImageResolver interface {
	Resolve(context.Context, string, engineRuntime.RunnerPlatform) (engineRuntime.RunnerImageBinding, error)
}
type LiquibaseImageRuntime interface {
	RunnerPlatform(context.Context) (engineRuntime.RunnerPlatform, error)
	EnsureRunnerImage(context.Context, engineRuntime.RunnerImageBinding) (string, error)
	InspectRunnerImage(context.Context, engineRuntime.RunnerImageBinding) (engineRuntime.RunnerImageInspection, error)
}
type LiquibaseDriverStore interface {
	Publish(context.Context) (driverbundle.Binding, error)
	Materialize(context.Context, driverbundle.Binding, string) (*driverbundle.Materialized, error)
	OpenMaterialized(context.Context, driverbundle.Binding, string) (*driverbundle.Materialized, error)
}
type LiquibaseContainerDeps struct {
	Images           LiquibaseImageResolver
	Runtime          LiquibaseImageRuntime
	Sources          inputbundle.Store
	Drivers          LiquibaseDriverStore
	MaterializedRoot string
}
type liquibaseBindingQueue interface {
	GetLiquibaseJobBinding(context.Context, string) (queue.LiquibaseJobBinding, bool, error)
	GetLiquibaseExecutionForJob(context.Context, string) (queue.LiquibaseExecutionRecord, bool, error)
	BindLiquibaseExecution(context.Context, string, queue.LiquibaseExecutionRecord) error
}

// containerAdmissionSnapshot retains the original portable coordinates. Expanded
// search modes/revision negotiation remain #173; host locators grant no authority.
type containerAdmissionSnapshot struct {
	NormalizedArgs   []string `json:"normalized_args"`
	Mounts           []string `json:"mounts"`
	ContainerWorkDir string   `json:"container_work_dir"`
}
type persistedContainerRequest struct {
	Request
	Container *containerAdmissionSnapshot `json:"_sqlrs_container_admission,omitempty"`
}

// admitContainerLiquibase captures immutable directory views before atomically
// saving the admitted job and its bundle lease. Failed replies require readback.
func (m *PrepareService) admitContainerLiquibase(ctx context.Context, job queue.JobRecord, p preparedRequest) error {
	q, ok := m.queue.(interface {
		AdmitLiquibaseJob(context.Context, queue.JobRecord, string, string) error
		ConfirmLiquibaseAdmission(context.Context, queue.JobRecord, string, string) (bool, error)
	})
	if !ok || m.containerLiquibase == nil {
		return fmt.Errorf("container Liquibase admission unavailable")
	}
	snapshot := &containerAdmissionSnapshot{NormalizedArgs: append([]string(nil), p.normalizedArgs...), ContainerWorkDir: p.liquibaseContainerWorkDir}
	var roots []inputbundle.MountedRoot
	for _, mount := range p.liquibaseMounts {
		logical := strings.TrimPrefix(mount.ContainerPath, "/sqlrs/mnt/")
		if logical == mount.ContainerPath || logical == "" || strings.ContainsAny(logical, "/\\") || !mount.ReadOnly {
			return fmt.Errorf("invalid admitted mount")
		}
		roots = append(roots, inputbundle.MountedRoot{Path: logical, Source: mount.HostPath})
		snapshot.Mounts = append(snapshot.Mounts, mount.ContainerPath)
	}
	ref, err := m.containerLiquibase.Sources.PublishRoots(ctx, roots)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(persistedContainerRequest{Request: p.request, Container: snapshot})
	if err != nil {
		return err
	}
	job.RequestJSON = strPtr(string(encoded))
	if err := q.AdmitLiquibaseJob(ctx, job, p.request.LiquibaseVersion, ref.ManifestDigest); err != nil {
		confirmed, readErr := q.ConfirmLiquibaseAdmission(ctx, job, p.request.LiquibaseVersion, ref.ManifestDigest)
		if readErr != nil {
			return errors.Join(err, readErr)
		}
		if !confirmed {
			return err
		}
	}
	return nil
}

// bindContainerLiquibase materializes only CAS bytes and pins the exact trusted
// image/driver/config tuple before planning. It never launches a command.
func (m *PrepareService) bindContainerLiquibase(ctx context.Context, jobID string, p preparedRequest) (bound preparedRequest, cleanup func() error, returned error) {
	returned = m.withLiquibaseProjectionGate(ctx, jobID, func() error {
		var err error
		bound, cleanup, err = m.bindFreshContainerLiquibase(ctx, jobID, p)
		return err
	})
	return
}

func (m *PrepareService) bindFreshContainerLiquibase(ctx context.Context, jobID string, p preparedRequest) (bound preparedRequest, cleanup func() error, returned error) {
	deps := m.containerLiquibase
	q, ok := m.queue.(liquibaseBindingQueue)
	if !ok || deps == nil || deps.Images == nil || deps.Runtime == nil {
		return p, nil, fmt.Errorf("trusted Liquibase binding unavailable")
	}
	admitted, found, err := q.GetLiquibaseJobBinding(ctx, jobID)
	if err != nil || !found || admitted.RequestedTag != p.request.LiquibaseVersion {
		return p, nil, queue.ErrLiquibaseBinding
	}
	owner, err := m.localLiquibaseProjectionOwner(true)
	if err != nil {
		return p, nil, err
	}
	job, err := owner.OpenJob(ctx, jobID)
	if err != nil {
		return p, nil, err
	}
	copy, err := job.AllocateCopy(ctx)
	if copy != nil {
		p.liquibaseProjection = copy
		cleanup = copy.Close
	}
	defer func() {
		if returned != nil {
			bound = p
		}
	}()
	if err != nil {
		return p, cleanup, err
	}
	destination := filepath.Join(copy.Sources, "bundle")
	if err := deps.Sources.Materialize(ctx, inputbundle.BundleRef{ManifestDigest: admitted.BundleManifestDigest}, destination); err != nil {
		return p, cleanup, err
	}
	properties, effective, err := readAdmittedLiquibaseSettings(destination, p)
	if err != nil {
		return p, cleanup, err
	}
	p.liquibaseProperties = properties
	platform, err := deps.Runtime.RunnerPlatform(ctx)
	if err != nil {
		return p, cleanup, err
	}
	declaration := digestLiquibaseValues(p.request.ImageID, admitted.RequestedTag, p.normalizedArgs)
	record, alreadyBound, err := q.GetLiquibaseExecutionForJob(ctx, jobID)
	if err != nil {
		return p, cleanup, err
	}
	if !alreadyBound {
		image, err := deps.Images.Resolve(ctx, admitted.RequestedTag, platform)
		if err != nil {
			return p, cleanup, err
		}
		if image.Platform != platform || image.RequestedTag != admitted.RequestedTag || !canonicalSHA256.MatchString(image.ManifestDigest) || image.DigestRef != "liquibase/liquibase@"+image.ManifestDigest {
			return p, cleanup, queue.ErrLiquibaseBinding
		}
		if _, err := deps.Runtime.EnsureRunnerImage(ctx, image); err != nil {
			return p, cleanup, err
		}
		inventory, err := deps.Runtime.InspectRunnerImage(ctx, image)
		if err != nil {
			return p, cleanup, err
		}
		catalog := driverbundle.ImageInventory{LiquibaseVersion: inventory.LiquibaseVersion, JavaMajor: inventory.JavaMajor}
		for _, jar := range inventory.PostgreSQLJars {
			catalog.PostgreSQLJars = append(catalog.PostgreSQLJars, driverbundle.EmbeddedDriver{Path: jar.Path, Digest: jar.Digest})
		}
		driver, err := driverbundle.Select("postgresql", catalog)
		if err != nil {
			return p, cleanup, err
		}
		if driver.Mode == "mounted" {
			if deps.Drivers == nil {
				return p, cleanup, fmt.Errorf("verified JDBC artifact unavailable")
			}
			driver, err = deps.Drivers.Publish(ctx)
			if err != nil {
				return p, cleanup, err
			}
			if driver.Mode != "mounted" || driver.Digest != driverbundle.DriverDigest || driver.ArtifactRef == "" {
				return p, cleanup, queue.ErrLiquibaseBinding
			}
		}
		target := digestLiquibaseValues(p.managed.Key, p.managed.IdentityDigest, p.effectiveImageID())
		fingerprint, err := liquibaseResolvedFingerprint(liquibaseResolvedInputs{DeclarationDigest: declaration, RequestedTag: admitted.RequestedTag, DBImageDigest: digestLiquibaseValues(p.effectiveImageID()), TargetIdentityDigest: target, PlatformOS: platform.OS, PlatformArch: platform.Architecture, PlatformVariant: platform.Variant, ImageManifestDigest: image.ManifestDigest, DriverMode: driver.Mode, DriverDigest: driver.Digest, DriverArtifactRef: driver.ArtifactRef, BundleManifestDigest: admitted.BundleManifestDigest, EffectiveConfigDigest: effective, PolicyRevision: liquibaseContainerFingerprintDomain})
		if err != nil {
			return p, cleanup, err
		}
		record = queue.LiquibaseExecutionRecord{ExecutionFingerprint: fingerprint, DeclarationDigest: declaration, RequestedTag: admitted.RequestedTag, TargetIdentityDigest: target, PlatformOS: platform.OS, PlatformArch: platform.Architecture, PlatformVariant: platform.Variant, ImageIndexDigest: image.IndexDigest, ImageManifestDigest: image.ManifestDigest, DriverMode: driver.Mode, DriverDigest: driver.Digest, DriverArtifactRef: driver.ArtifactRef, BundleManifestDigest: admitted.BundleManifestDigest, EffectiveConfigDigest: effective, PolicyRevision: liquibaseContainerFingerprintDomain, CreatedAt: m.now().UTC().Format(time.RFC3339Nano)}
		if err := q.BindLiquibaseExecution(ctx, jobID, record); err != nil {
			return p, cleanup, err
		}
	}
	if record.DeclarationDigest != declaration || record.EffectiveConfigDigest != effective || record.BundleManifestDigest != admitted.BundleManifestDigest || record.PlatformOS != platform.OS || record.PlatformArch != platform.Architecture || record.PlatformVariant != platform.Variant {
		return p, cleanup, queue.ErrLiquibaseBinding
	}
	image := engineRuntime.RunnerImageBinding{RequestedTag: record.RequestedTag, Platform: platform, IndexDigest: record.ImageIndexDigest, ManifestDigest: record.ImageManifestDigest, DigestRef: "liquibase/liquibase@" + record.ImageManifestDigest}
	if _, err := deps.Runtime.EnsureRunnerImage(ctx, image); err != nil {
		return p, cleanup, err
	}
	for i := range p.liquibaseMounts {
		logical := strings.TrimPrefix(p.liquibaseMounts[i].ContainerPath, "/sqlrs/mnt/")
		p.liquibaseMounts[i].HostPath = filepath.Join(destination, logical)
	}
	if record.DriverMode == "mounted" {
		if deps.Drivers == nil {
			return p, cleanup, fmt.Errorf("saved JDBC artifact unavailable")
		}
		driver, err := deps.Drivers.Materialize(ctx, driverbundle.Binding{Mode: record.DriverMode, Digest: record.DriverDigest, ArtifactRef: record.DriverArtifactRef}, filepath.Join(copy.Driver, "files"))
		if err != nil {
			return p, cleanup, err
		}
		p.liquibaseMounts = append(p.liquibaseMounts, engineRuntime.Mount{HostPath: driver.HostPath, ContainerPath: driver.ContainerPath, ReadOnly: true})
	}
	p.liquibaseImageID, p.liquibaseFingerprint = image.DigestRef, record.ExecutionFingerprint
	p.liquibaseLockPaths, p.liquibaseSearchPaths = nil, nil
	return p, cleanup, nil
}

func digestLiquibaseValues(values ...any) string {
	bytes, _ := json.Marshal(values)
	sum := sha256.Sum256(bytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Settings are read from the admitted view only. Connection/classpath overrides
// are rejected by the same protected-properties parser used at execution.
func readAdmittedLiquibaseSettings(root string, p preparedRequest) (map[string]string, string, error) {
	file := p.liquibaseContainerWorkDir + "/liquibase.properties"
	explicit := false
	for i, arg := range p.normalizedArgs {
		if arg == "--defaults-file" && i+1 < len(p.normalizedArgs) {
			file = p.normalizedArgs[i+1]
			explicit = true
		}
		if strings.HasPrefix(arg, "--defaults-file=") {
			file = strings.TrimPrefix(arg, "--defaults-file=")
			explicit = true
		}
	}
	if !strings.HasPrefix(file, "/sqlrs/mnt/") {
		return nil, "", fmt.Errorf("defaults are outside admitted inputs")
	}
	rel := strings.TrimPrefix(file, "/sqlrs/mnt/")
	if strings.Contains(rel, "..") || strings.Contains(rel, "\\") {
		return nil, "", fmt.Errorf("invalid admitted defaults path")
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if os.IsNotExist(err) && !explicit {
		properties := map[string]string{}
		return properties, digestLiquibaseValues(properties), nil
	}
	if err != nil {
		return nil, "", err
	}
	properties, err := effectiveconfig.ParseProperties(strings.NewReader(string(data)))
	if err != nil {
		return nil, "", err
	}
	return properties, digestLiquibaseValues(properties), nil
}
