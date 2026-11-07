package prepare

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

var errLiquibaseProjection = errors.New("Liquibase projection namespace is unavailable or ambiguous")
var projectionCopyPattern = regexp.MustCompile(`^copy-[0-9a-f]{32}$`)
var projectionProtectedRefPattern = regexp.MustCompile(`^(copy-[0-9a-f]{32})-plan-([0-9a-f]{24})-input$`)

type projectionAuthority interface {
	CheckLiquibaseProjectionAllocation(context.Context, string) error
	CheckLiquibaseProjectionResume(context.Context, string) error
	CheckLiquibaseProjectionCleanup(context.Context, queue.FinalizationSnapshot) error
}

// liquibaseProjectionOwner owns disposable local copies, never CAS artifacts.
// All methods require the caller's exclusive keyed job gate; the managed-store
// lock excludes another engine. The queue, not owner.json, grants admission.
// Requirements: liquibase-local-projection-cleanup-design.md, protected namespace.
type liquibaseProjectionOwner struct {
	root      string
	authority projectionAuthority
	hook      func(string) error
}

type liquibaseProjectionJob struct {
	owner *liquibaseProjectionOwner
	jobID string
	root  string
}

// liquibaseProjectionCopy retains its exact locator even after partial failure.
// Close is only a per-copy optimization AFTER the caller proves its executors
// retired. It never removes a job marker/namespace or releases artifact leases.
type liquibaseProjectionCopy struct {
	job                       *liquibaseProjectionJob
	root                      string
	Settings, Sources, Driver string
}

type projectionOwnership struct {
	Revision int    `json:"revision"`
	JobID    string `json:"job_id"`
	Key      string `json:"key"`
}

func projectionJobKey(jobID string) string {
	sum := sha256.Sum256([]byte("sqlrs-liquibase-local-projections-v1\n" + jobID))
	return hex.EncodeToString(sum[:])
}

func newLiquibaseProjectionOwner(root string, authority projectionAuthority) (*liquibaseProjectionOwner, error) {
	if !filepath.IsAbs(root) || authority == nil {
		return nil, errLiquibaseProjection
	}
	root = filepath.Clean(root)
	if err := projectionDirectory(root, true); err != nil {
		return nil, err
	}
	return &liquibaseProjectionOwner{root: root, authority: authority}, nil
}

// projectionDirectory checks every ancestor without adopting links/reparse
// points. A missing directory is created only when explicitly requested; cleanup
// never creates missing parents to make an ambiguous target look owned.
func projectionDirectory(path string, create bool) error {
	if !filepath.IsAbs(path) {
		return errLiquibaseProjection
	}
	parent := filepath.Dir(path)
	if parent != path {
		if err := projectionDirectory(parent, create); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return errLiquibaseProjection
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || !plainProjectionEntry(path, info) {
		return errLiquibaseProjection
	}
	return nil
}

func plainProjectionEntry(path string, info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink == 0 && projectionPlainPlatform(path, info)
}

func (o *liquibaseProjectionOwner) checkpoint(ctx context.Context, cut string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.hook != nil {
		return o.hook(cut)
	}
	return nil
}

func (o *liquibaseProjectionOwner) job(jobID string) *liquibaseProjectionJob {
	return &liquibaseProjectionJob{owner: o, jobID: jobID, root: filepath.Join(o.root, "jobs", projectionJobKey(jobID))}
}

// liquibaseProjectionSettingsRoot derives, but never allocates, the exact
// journal-named settings directory. Retirement also uses this after files have
// disappeared, so existence is checked by the caller's creation/removal path.
func (m *PrepareService) liquibaseProjectionSettingsRoot(jobID, ref string) (string, error) {
	parts := projectionProtectedRefPattern.FindStringSubmatch(ref)
	if m == nil || m.containerLiquibase == nil || !filepath.IsAbs(m.containerLiquibase.MaterializedRoot) || jobID == "" || parts == nil {
		return "", errLiquibaseProjection
	}
	return filepath.Join(m.containerLiquibase.MaterializedRoot, "jobs", projectionJobKey(jobID), "copies", parts[1], "settings"), nil
}

// protectedRef persists only an opaque locator in the existing launch journal,
// before a settings file or executor exists; see the 2026-10-18 locator ADR.
func (c *liquibaseProjectionCopy) protectedRef(planID string) (string, error) {
	if c == nil || !planningCloneIDPattern.MatchString(planID) || !projectionCopyPattern.MatchString(filepath.Base(c.root)) {
		return "", errLiquibaseProjection
	}
	return filepath.Base(c.root) + "-plan-" + planID + "-input", nil
}

func projectionCopyHandle(job *liquibaseProjectionJob, root string) *liquibaseProjectionCopy {
	return &liquibaseProjectionCopy{job: job, root: root, Settings: filepath.Join(root, "settings"), Sources: filepath.Join(root, "sources"), Driver: filepath.Join(root, "driver")}
}

// reopenCopy performs no creation or repair. A missing original mount is a
// recovery problem, not permission to replace it beneath a live container.
func (o *liquibaseProjectionOwner) reopenCopy(ctx context.Context, jobID, protectedRef string) (*liquibaseProjectionCopy, error) {
	if err := o.authority.CheckLiquibaseProjectionResume(ctx, jobID); err != nil {
		return nil, err
	}
	parts := projectionProtectedRefPattern.FindStringSubmatch(protectedRef)
	if parts == nil {
		return nil, errLiquibaseProjection
	}
	job := o.job(jobID)
	marked, _, err := job.validate()
	if err != nil || !marked {
		return nil, errLiquibaseProjection
	}
	copy := projectionCopyHandle(job, filepath.Join(job.root, "copies", parts[1]))
	for _, path := range []string{copy.Settings, copy.Sources, copy.Driver} {
		if err := projectionDirectory(path, false); err != nil {
			return nil, errLiquibaseProjection
		}
	}
	return copy, nil
}

// OpenJob publishes a synced marker before payload allocation. Existing marked
// namespaces are validated, never overwritten. A pre-marker interrupted directory
// can be reopened only if empty or containing the sole reserved pending file.
func (o *liquibaseProjectionOwner) OpenJob(ctx context.Context, jobID string) (*liquibaseProjectionJob, error) {
	if err := o.authority.CheckLiquibaseProjectionAllocation(ctx, jobID); err != nil {
		return nil, err
	}
	j := o.job(jobID)
	if err := projectionDirectory(filepath.Dir(j.root), true); err != nil {
		return nil, err
	}
	if err := os.Mkdir(j.root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, errLiquibaseProjection
	}
	marked, _, err := j.validate()
	if err != nil {
		return nil, err
	}
	if marked {
		return j, nil
	}
	pending := filepath.Join(j.root, "owner.pending")
	if err := os.Remove(pending); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errLiquibaseProjection
	}
	file, err := os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errLiquibaseProjection
	}
	encoded, _ := json.Marshal(projectionOwnership{Revision: 1, JobID: jobID, Key: projectionJobKey(jobID)})
	_, writeErr := file.Write(encoded)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return nil, errLiquibaseProjection
	}
	if err := o.checkpoint(ctx, "before:ownership_publish"); err != nil {
		return nil, err
	}
	if err := publishProjectionOwner(pending, filepath.Join(j.root, "owner.json")); err != nil {
		return nil, errLiquibaseProjection
	}
	if err := syncProjectionDirectory(j.root); err != nil {
		return nil, errLiquibaseProjection
	}
	if err := o.checkpoint(ctx, "after:ownership_publish"); err != nil {
		return nil, err
	}
	return j, nil
}

// AllocateCopy rechecks current admission even for a previously opened job.
// Partial creation returns the handle so failure never loses its only locator.
func (j *liquibaseProjectionJob) AllocateCopy(ctx context.Context) (*liquibaseProjectionCopy, error) {
	if err := j.owner.authority.CheckLiquibaseProjectionAllocation(ctx, j.jobID); err != nil {
		return nil, err
	}
	marked, _, err := j.validate()
	if err != nil || !marked {
		return nil, errLiquibaseProjection
	}
	parent := filepath.Join(j.root, "copies")
	if err := projectionDirectory(parent, true); err != nil {
		return nil, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errLiquibaseProjection
	}
	root := filepath.Join(parent, "copy-"+hex.EncodeToString(nonce[:]))
	if err := os.Mkdir(root, 0700); err != nil {
		// Never return ownership of a pre-existing copy on collision.
		return nil, errLiquibaseProjection
	}
	copy := projectionCopyHandle(j, root)
	for _, destination := range []string{copy.Settings, copy.Sources, copy.Driver} {
		if err := os.Mkdir(destination, 0700); err != nil {
			return copy, errLiquibaseProjection
		}
	}
	return copy, nil
}

// validate checks the COMPLETE owned subtree before destructive traversal. The
// returned list is postorder; structural unknowns/links cannot be encountered
// only after an earlier legitimate payload has already been deleted.
func (j *liquibaseProjectionJob) validate() (bool, []string, error) {
	if err := projectionDirectory(j.root, false); err != nil {
		return false, nil, err
	}
	entries, err := os.ReadDir(j.root)
	if err != nil {
		return false, nil, errLiquibaseProjection
	}
	marked := false
	var paths []string
	for _, entry := range entries {
		path := filepath.Join(j.root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !plainProjectionEntry(path, info) {
			return false, nil, errLiquibaseProjection
		}
		switch entry.Name() {
		case "owner.json":
			if !info.Mode().IsRegular() || info.Size() > 65536 {
				return false, nil, errLiquibaseProjection
			}
			bytes, err := os.ReadFile(path)
			var owner projectionOwnership
			if err != nil || json.Unmarshal(bytes, &owner) != nil || owner.Revision != 1 || owner.JobID != j.jobID || owner.Key != projectionJobKey(j.jobID) {
				return false, nil, errLiquibaseProjection
			}
			canonical, _ := json.Marshal(owner)
			if string(canonical) != string(bytes) {
				return false, nil, errLiquibaseProjection
			}
			marked = true
		case "owner.pending":
			if !info.Mode().IsRegular() {
				return false, nil, errLiquibaseProjection
			}
		case "copies":
			if !info.IsDir() {
				return false, nil, errLiquibaseProjection
			}
			copies, err := os.ReadDir(path)
			if err != nil {
				return false, nil, errLiquibaseProjection
			}
			for _, copy := range copies {
				copyPath := filepath.Join(path, copy.Name())
				if !projectionCopyPattern.MatchString(copy.Name()) {
					return false, nil, errLiquibaseProjection
				}
				items, err := validateProjectionCopy(copyPath)
				if err != nil {
					return false, nil, err
				}
				paths = append(paths, items...)
				paths = append(paths, copyPath)
			}
			paths = append(paths, path)
		default:
			return false, nil, errLiquibaseProjection
		}
	}
	if !marked {
		for _, entry := range entries {
			if entry.Name() != "owner.pending" {
				return false, nil, errLiquibaseProjection
			}
		}
	}
	return marked, paths, nil
}

func validateProjectionCopy(root string) ([]string, error) {
	if err := projectionDirectory(root, false); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, errLiquibaseProjection
	}
	var paths []string
	for _, entry := range entries {
		if entry.Name() != "settings" && entry.Name() != "sources" && entry.Name() != "driver" {
			return nil, errLiquibaseProjection
		}
		path := filepath.Join(root, entry.Name())
		if err := projectionDirectory(path, false); err != nil {
			return nil, err
		}
		if err := filepath.WalkDir(path, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return errLiquibaseProjection
			}
			info, err := os.Lstat(path)
			if err != nil || !plainProjectionEntry(path, info) || !info.IsDir() && !info.Mode().IsRegular() {
				return errLiquibaseProjection
			}
			paths = append(paths, path)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	// WalkDir is preorder; reverse each complete payload walk for deletion.
	for left, right := 0, len(paths)-1; left < right; left, right = left+1, right-1 {
		paths[left], paths[right] = paths[right], paths[left]
	}
	return paths, nil
}

func (o *liquibaseProjectionOwner) remove(ctx context.Context, path, point string) error {
	if err := o.checkpoint(ctx, "before:"+point); err != nil {
		return err
	}
	if err := projectionDirectory(filepath.Dir(path), false); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !plainProjectionEntry(path, info) || !info.IsDir() && !info.Mode().IsRegular() {
		return errLiquibaseProjection
	}
	if err := prepareProjectionRemoval(path, info); err != nil {
		return errLiquibaseProjection
	}
	if err := os.Remove(path); err != nil {
		return errLiquibaseProjection
	}
	return o.checkpoint(ctx, "after:"+point)
}

func (o *liquibaseProjectionOwner) CleanupJob(ctx context.Context, intent queue.FinalizationSnapshot) error {
	if err := o.authority.CheckLiquibaseProjectionCleanup(ctx, intent); err != nil {
		return err
	}
	j := o.job(intent.JobID)
	marked, paths, err := j.validate()
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, path := range paths {
		point := "payload_remove"
		if projectionCopyPattern.MatchString(filepath.Base(path)) && filepath.Dir(path) == filepath.Join(j.root, "copies") {
			point = "copy_remove"
		}
		if err := o.remove(ctx, path, point); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(filepath.Join(j.root, "owner.pending")); err == nil {
		if err := o.remove(ctx, filepath.Join(j.root, "owner.pending"), "staging_remove"); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errLiquibaseProjection
	}
	if marked {
		if err := o.remove(ctx, filepath.Join(j.root, "owner.json"), "owner_remove"); err != nil {
			return err
		}
	}
	return o.remove(ctx, j.root, "job_remove")
}

func (o *liquibaseProjectionOwner) VerifyLiquibaseProjectionsAbsent(ctx context.Context, intent queue.FinalizationSnapshot) error {
	if err := o.authority.CheckLiquibaseProjectionCleanup(ctx, intent); err != nil {
		return err
	}
	root := o.job(intent.JobID).root
	if err := projectionDirectory(filepath.Dir(root), false); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		return errLiquibaseProjection
	}
	return nil
}

func (c *liquibaseProjectionCopy) Close() error {
	if c == nil {
		return nil
	}
	if err := projectionDirectory(filepath.Dir(c.root), false); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Lstat(c.root); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return errLiquibaseProjection
	}
	if _, _, err := c.job.validate(); err != nil {
		return err
	}
	paths, err := validateProjectionCopy(c.root)
	if err != nil {
		return err
	}
	for _, path := range append(paths, c.root) {
		if err := c.job.owner.remove(context.Background(), path, "payload_remove"); err != nil {
			return err
		}
	}
	return nil
}

var _ io.Closer = (*liquibaseProjectionCopy)(nil)
