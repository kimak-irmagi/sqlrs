// Package remotesource implements the CLI side of the remote source-sync
// protocol documented in docs/architecture/remote-source-input-sync-flow.md.
package remotesource

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/inputset"
)

const DefaultMaxRounds = 8
const uploadProgressCheckpoint = int64(1024 * 1024)

type ProgressStage string

const (
	ProgressStageStart              ProgressStage = "start"
	ProgressStageRound              ProgressStage = "round"
	ProgressStageFileHashStart      ProgressStage = "file_hash_start"
	ProgressStageFileHashed         ProgressStage = "file_hashed"
	ProgressStageDirectoryListStart ProgressStage = "directory_list_start"
	ProgressStageDirectoryListed    ProgressStage = "directory_listed"
	ProgressStageUploadStart        ProgressStage = "upload_start"
	ProgressStageUploadBytes        ProgressStage = "upload_bytes"
	ProgressStageUploadComplete     ProgressStage = "upload_complete"
	ProgressStageRetry              ProgressStage = "retry"
	ProgressStageComplete           ProgressStage = "complete"
	ProgressStageError              ProgressStage = "error"
)

// ProgressEvent is a presentation-neutral source synchronization milestone.
// Paths are workspace-relative. Bytes reports upload content consumed for the
// event (or the successful aggregate on completion), while UploadIndex and
// UploadCount identify one unique digest transfer in the current request set.
type ProgressEvent struct {
	Stage             ProgressStage
	Round             int
	Path              string
	Digest            string
	Bytes             int64
	TotalBytes        int64
	ManifestEntries   int
	Blobs             int
	FileHashes        int
	DirectoryListings int
	UploadedBlobs     int
	UploadIndex       int
	UploadCount       int
	Error             string
}

type Progress interface {
	Update(ProgressEvent)
}

// ProgressFinisher closes presentation resources without adding a semantic
// event. Implementations use it to stop delayed renderers on the silent path.
type ProgressFinisher interface {
	Finish()
}

type Uploader interface {
	PutSourceBlob(context.Context, string, io.Reader) error
}

type Options struct {
	Enabled       bool
	MaxRounds     int
	WorkspaceRoot string
	WorkDir       string
	WorkspaceID   string
	FileSystem    inputset.FileSystem
	Uploader      Uploader
	Progress      Progress
}

// Execute sends a prepare-shaped request through the remote source-sync loop.
// The server remains authoritative: the client only adds manifest entries and
// uploads blobs named by a recoverable source_inputs_missing response.
func Execute[T any](ctx context.Context, opts Options, req client.PrepareJobRequest, execute func(context.Context, client.PrepareJobRequest) (T, error)) (T, error) {
	if !opts.Enabled {
		return execute(ctx, req)
	}
	defer finishProgress(opts.Progress)
	state := newRoundState(opts)
	maxRounds := opts.MaxRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxRounds
	}

	req.SourceManifest = state.manifest()
	for round := 1; ; round++ {
		result, err := execute(ctx, req)
		if err == nil {
			if state.started {
				state.emit(ProgressEvent{Stage: ProgressStageComplete, Round: round, FileHashes: len(state.files), DirectoryListings: len(state.directories), UploadedBlobs: len(state.uploaded), Bytes: state.uploadedBytes})
			}
			return result, nil
		}

		var missing *client.SourceInputsMissingError
		if !errors.As(err, &missing) {
			var zero T
			if state.started {
				state.emit(ProgressEvent{Stage: ProgressStageError, Round: round, Error: err.Error()})
			}
			return zero, err
		}
		if !state.started {
			state.started = true
			state.emit(ProgressEvent{Stage: ProgressStageStart})
		}
		if round > maxRounds {
			var zero T
			resultErr := fmt.Errorf("source sync reached max rounds (%d): %w", maxRounds, err)
			state.emit(ProgressEvent{Stage: ProgressStageError, Round: round, Error: resultErr.Error()})
			return zero, resultErr
		}

		state.emit(ProgressEvent{Stage: ProgressStageRound, Round: round, ManifestEntries: len(missing.Response.MissingManifestEntries), Blobs: len(missing.Response.MissingBlobs)})
		stats, changed, applyErr := state.applyMissing(ctx, missing.Response)
		if applyErr != nil {
			var zero T
			state.emit(ProgressEvent{Stage: ProgressStageError, Round: round, Error: applyErr.Error()})
			return zero, applyErr
		}
		if !changed {
			var zero T
			resultErr := fmt.Errorf("source sync made no progress after round %d: %w", round, err)
			state.emit(ProgressEvent{Stage: ProgressStageError, Round: round, Error: resultErr.Error()})
			return zero, resultErr
		}
		req.SourceManifest = state.manifest()
		state.emit(ProgressEvent{Stage: ProgressStageRetry, Round: round, FileHashes: stats.FileHashes, DirectoryListings: stats.DirectoryListings, UploadedBlobs: stats.UploadedBlobs})
	}
}

type roundState struct {
	root          string
	workDir       string
	rootID        string
	fs            inputset.FileSystem
	uploader      Uploader
	progress      Progress
	started       bool
	uploadedBytes int64
	files         map[string]string
	directories   map[string]client.SourceDirectoryListing
	uploaded      map[string]struct{}
}

type applyStats struct {
	FileHashes        int
	DirectoryListings int
	UploadedBlobs     int
}

func newRoundState(opts Options) *roundState {
	root := strings.TrimSpace(opts.WorkspaceRoot)
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err == nil {
		root = absRoot
	}
	root = filepath.Clean(root)
	workDir := strings.TrimSpace(opts.WorkDir)
	if workDir == "" {
		workDir = root
	}
	if absWorkDir, err := filepath.Abs(workDir); err == nil {
		workDir = absWorkDir
	}
	workDir = filepath.Clean(workDir)
	rootID := strings.TrimSpace(opts.WorkspaceID)
	if rootID == "" {
		rootID = filepath.Base(root)
	}
	if rootID == "" || rootID == "." {
		rootID = "default"
	}
	sourceFS := opts.FileSystem
	if sourceFS == nil {
		sourceFS = inputset.OSFileSystem{}
	}
	return &roundState{
		root:        root,
		workDir:     workDir,
		rootID:      rootID,
		fs:          sourceFS,
		uploader:    opts.Uploader,
		progress:    opts.Progress,
		files:       map[string]string{},
		directories: map[string]client.SourceDirectoryListing{},
		uploaded:    map[string]struct{}{},
	}
}

func (s *roundState) manifest() *client.SourceManifest {
	manifest := &client.SourceManifest{
		WorkspaceRef: &client.SourceWorkspaceRef{
			RootID:   s.rootID,
			RootPath: s.root,
			WorkDir:  s.workDir,
		},
	}
	if len(s.files) > 0 {
		manifest.Files = make(map[string]string, len(s.files))
		for key, value := range s.files {
			manifest.Files[key] = value
		}
	}
	if len(s.directories) > 0 {
		manifest.Directories = make(map[string]client.SourceDirectoryListing, len(s.directories))
		for key, value := range s.directories {
			entries := append([]client.SourceDirectoryEntry(nil), value.Entries...)
			manifest.Directories[key] = client.SourceDirectoryListing{Entries: entries, Complete: value.Complete}
		}
	}
	return manifest
}

func (s *roundState) applyMissing(ctx context.Context, missing client.SourceInputsMissingErrorResponse) (applyStats, bool, error) {
	stats := applyStats{}
	changed := false

	entries := append([]client.SourceMissingManifestEntry(nil), missing.MissingManifestEntries...)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path == entries[j].Path {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Path < entries[j].Path
	})
	for _, entry := range entries {
		switch entry.Kind {
		case "file_hash":
			added, err := s.addFileHash(entry.Path)
			if err != nil {
				return stats, changed, err
			}
			if added {
				stats.FileHashes++
				changed = true
			}
		case "directory_listing":
			added, err := s.addDirectoryListing(entry.Path)
			if err != nil {
				return stats, changed, err
			}
			if added {
				stats.DirectoryListings++
				changed = true
			}
		default:
			return stats, changed, fmt.Errorf("unsupported source manifest entry kind %q for %s", entry.Kind, entry.Path)
		}
	}

	blobs := append([]client.SourceMissingBlob(nil), missing.MissingBlobs...)
	sort.Slice(blobs, func(i, j int) bool {
		if blobs[i].Hash == blobs[j].Hash {
			return blobs[i].Path < blobs[j].Path
		}
		return blobs[i].Hash < blobs[j].Hash
	})
	pendingDigests := make(map[string]struct{})
	for _, blob := range blobs {
		digest, ok := validBlobDigest(blob.Hash)
		if !ok {
			continue
		}
		if _, uploaded := s.uploaded[digest]; !uploaded {
			pendingDigests[digest] = struct{}{}
		}
	}
	uploadOrdinals := make(map[string]int, len(pendingDigests))
	nextUploadIndex := 0
	for _, blob := range blobs {
		digest, valid := validBlobDigest(blob.Hash)
		uploadIndex := 0
		if valid {
			if _, pending := pendingDigests[digest]; pending {
				if _, assigned := uploadOrdinals[digest]; !assigned {
					nextUploadIndex++
					uploadOrdinals[digest] = nextUploadIndex
				}
				uploadIndex = uploadOrdinals[digest]
			}
		}
		uploaded, err := s.uploadBlob(ctx, blob, uploadIndex, len(pendingDigests))
		if err != nil {
			return stats, changed, err
		}
		if uploaded {
			stats.UploadedBlobs++
			changed = true
		}
	}

	return stats, changed, nil
}

func (s *roundState) addFileHash(manifestPath string) (bool, error) {
	cleaned, absPath, err := s.resolveManifestPath(manifestPath)
	if err != nil {
		return false, err
	}
	s.emit(ProgressEvent{Stage: ProgressStageFileHashStart, Path: cleaned})
	hash, _, err := s.readFileHash(absPath)
	if err != nil {
		return false, fmt.Errorf("hash source file %s: %w", cleaned, err)
	}
	s.emit(ProgressEvent{Stage: ProgressStageFileHashed, Path: cleaned, Digest: shortDigest(hash)})
	if previous, ok := s.files[cleaned]; ok {
		if previous != hash {
			return false, fmt.Errorf("source file changed during sync: %s", cleaned)
		}
		return false, nil
	}
	s.files[cleaned] = hash
	return true, nil
}

func (s *roundState) addDirectoryListing(manifestPath string) (bool, error) {
	cleaned, absPath, err := s.resolveManifestPath(manifestPath)
	if err != nil {
		return false, err
	}
	s.emit(ProgressEvent{Stage: ProgressStageDirectoryListStart, Path: cleaned})
	entries, err := s.fs.ReadDir(absPath)
	if err != nil {
		return false, fmt.Errorf("list source directory %s: %w", cleaned, err)
	}
	listing := client.SourceDirectoryListing{
		Entries:  make([]client.SourceDirectoryEntry, 0, len(entries)),
		Complete: true,
	}
	for _, entry := range entries {
		kind := "file"
		if entry.IsDir() {
			kind = "directory"
		}
		listing.Entries = append(listing.Entries, client.SourceDirectoryEntry{
			Name: entry.Name(),
			Kind: kind,
		})
	}
	sort.Slice(listing.Entries, func(i, j int) bool {
		return listing.Entries[i].Name < listing.Entries[j].Name
	})
	s.emit(ProgressEvent{Stage: ProgressStageDirectoryListed, Path: cleaned})
	if previous, ok := s.directories[cleaned]; ok {
		if !sameDirectoryListing(previous, listing) {
			return false, fmt.Errorf("source directory changed during sync: %s", cleaned)
		}
		return false, nil
	}
	s.directories[cleaned] = listing
	return true, nil
}

func (s *roundState) uploadBlob(ctx context.Context, blob client.SourceMissingBlob, uploadIndex, uploadCount int) (bool, error) {
	if s.uploader == nil {
		return false, fmt.Errorf("source blob upload is not configured")
	}
	expected := strings.TrimSpace(blob.Hash)
	digest, ok := validBlobDigest(expected)
	if !ok {
		return false, fmt.Errorf("invalid source blob hash for %s: %s", blob.Path, blob.Hash)
	}
	cleaned, absPath, err := s.resolveManifestPath(blob.Path)
	if err != nil {
		return false, err
	}
	actual, content, err := s.readFileHash(absPath)
	if err != nil {
		return false, fmt.Errorf("read source blob %s: %w", cleaned, err)
	}
	if actual != expected {
		return false, fmt.Errorf("source blob hash mismatch for %s: expected %s, got %s", cleaned, expected, actual)
	}
	if previous, ok := s.files[cleaned]; ok && previous != actual {
		return false, fmt.Errorf("source file changed during sync: %s", cleaned)
	}
	changed := false
	if _, ok := s.files[cleaned]; !ok {
		s.files[cleaned] = actual
		changed = true
	}
	if _, ok := s.uploaded[digest]; ok {
		return changed, nil
	}
	s.emit(ProgressEvent{Stage: ProgressStageUploadStart, Path: cleaned, Digest: shortDigest(expected), TotalBytes: int64(len(content)), UploadIndex: uploadIndex, UploadCount: uploadCount})
	reader := &progressReader{reader: bytes.NewReader(content), total: int64(len(content)), path: cleaned, digest: shortDigest(expected), uploadIndex: uploadIndex, uploadCount: uploadCount, emit: s.emit}
	if err := s.uploader.PutSourceBlob(ctx, digest, reader); err != nil {
		return false, err
	}
	s.emit(ProgressEvent{Stage: ProgressStageUploadComplete, Path: cleaned, Digest: shortDigest(expected), Bytes: reader.read, TotalBytes: int64(len(content)), UploadIndex: uploadIndex, UploadCount: uploadCount})
	s.uploaded[digest] = struct{}{}
	s.uploadedBytes += reader.read
	return true, nil
}

func (s *roundState) resolveManifestPath(value string) (string, string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", "", fmt.Errorf("source manifest path is empty")
	}
	if strings.Contains(raw, "\\") {
		return "", "", fmt.Errorf("source manifest path must use slash separators: %s", value)
	}
	cleaned := path.Clean(raw)
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") || driveQualified(cleaned) {
		return "", "", fmt.Errorf("source manifest path escapes workspace: %s", value)
	}
	absPath := filepath.Join(s.root, filepath.FromSlash(cleaned))
	canonicalRoot := inputset.CanonicalizeBoundaryPath(s.root)
	canonicalPath := inputset.CanonicalizeBoundaryPath(absPath)
	if !inputset.IsWithin(canonicalRoot, canonicalPath) {
		return "", "", fmt.Errorf("source manifest path escapes workspace: %s", value)
	}
	return cleaned, absPath, nil
}

func (s *roundState) readFileHash(absPath string) (string, []byte, error) {
	info, err := s.fs.Stat(absPath)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		return "", nil, fs.ErrInvalid
	}
	content, err := s.fs.ReadFile(absPath)
	if err != nil {
		return "", nil, err
	}
	return "sha256:" + inputset.HashContent(content), content, nil
}

func (s *roundState) emit(event ProgressEvent) {
	if s.progress != nil {
		s.progress.Update(event)
	}
}

func finishProgress(progress Progress) {
	if finisher, ok := progress.(ProgressFinisher); ok {
		finisher.Finish()
	}
}

type progressReader struct {
	reader                   io.Reader
	emit                     func(ProgressEvent)
	path, digest             string
	total, read, next        int64
	uploadIndex, uploadCount int
}

func (r *progressReader) Read(p []byte) (int, error) {
	if r.next == 0 {
		r.next = uploadProgressCheckpoint
	}
	if remaining := r.next - r.read; remaining > 0 && int64(len(p)) > remaining {
		p = p[:remaining]
	}
	n, err := r.reader.Read(p)
	r.read += int64(n)
	if r.read >= r.next && r.read < r.total {
		r.emit(ProgressEvent{Stage: ProgressStageUploadBytes, Path: r.path, Digest: r.digest, Bytes: r.read, TotalBytes: r.total, UploadIndex: r.uploadIndex, UploadCount: r.uploadCount})
		for r.next <= r.read {
			r.next += uploadProgressCheckpoint
		}
	}
	return n, err
}

func shortDigest(value string) string {
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) > 12 {
		value = value[:12]
	}
	return value
}

func sameDirectoryListing(left, right client.SourceDirectoryListing) bool {
	if left.Complete != right.Complete || len(left.Entries) != len(right.Entries) {
		return false
	}
	for i := range left.Entries {
		if left.Entries[i] != right.Entries[i] {
			return false
		}
	}
	return true
}

func isLowerHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func validBlobDigest(value string) (string, bool) {
	digest, ok := strings.CutPrefix(strings.TrimSpace(value), "sha256:")
	return digest, ok && isLowerHexDigest(digest)
}

func driveQualified(value string) bool {
	if len(value) < 2 || value[1] != ':' {
		return false
	}
	first := value[0]
	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z')
}
