package remotesource

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/inputset"
)

type recordingUploader struct {
	digests []string
	bodies  []string
	err     error
}

type recordingProgress struct {
	events   []ProgressEvent
	finished bool
}

func (p *recordingProgress) Update(event ProgressEvent) { p.events = append(p.events, event) }
func (p *recordingProgress) Finish()                    { p.finished = true }

func (p *recordingProgress) lastStage() ProgressStage {
	if len(p.events) == 0 {
		return ""
	}
	return p.events[len(p.events)-1].Stage
}

type progressCheckingFS struct {
	inputset.FileSystem
	progress *recordingProgress
	t        *testing.T
}

func (f progressCheckingFS) Stat(name string) (fs.FileInfo, error) {
	if f.progress.lastStage() != ProgressStageFileHashStart {
		f.t.Fatalf("stat %s observed stage %q, want %q", name, f.progress.lastStage(), ProgressStageFileHashStart)
	}
	return f.FileSystem.Stat(name)
}

func (f progressCheckingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.progress.lastStage() != ProgressStageDirectoryListStart {
		f.t.Fatalf("readdir %s observed stage %q, want %q", name, f.progress.lastStage(), ProgressStageDirectoryListStart)
	}
	return f.FileSystem.ReadDir(name)
}

func (u *recordingUploader) PutSourceBlob(_ context.Context, digest string, body io.Reader) error {
	if u.err != nil {
		return u.err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	u.digests = append(u.digests, digest)
	u.bodies = append(u.bodies, string(data))
	return nil
}

func TestExecuteExpandsManifestUploadsBlobAndRetries(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "db/changelog/master.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceFile(root, "db/changelog/changes/002.sql", "select 2;\n"); err != nil {
		t.Fatal(err)
	}
	hash := "sha256:" + inputset.HashContent([]byte("select 1;\n"))
	uploader := &recordingUploader{}
	progress := &recordingProgress{}
	var manifests []*client.SourceManifest
	calls := 0

	got, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		WorkDir:       filepath.Join(root, "db"),
		WorkspaceID:   "remote",
		FileSystem:    inputset.OSFileSystem{},
		Uploader:      uploader,
		Progress:      progress,
	}, client.PrepareJobRequest{PrepareKind: "psql"}, func(_ context.Context, req client.PrepareJobRequest) (string, error) {
		calls++
		manifests = append(manifests, req.SourceManifest)
		if calls == 1 {
			return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
				Code:    "source_inputs_missing",
				Message: "missing",
				MissingManifestEntries: []client.SourceMissingManifestEntry{
					{Path: "db/changelog", Kind: "directory_listing"},
					{Path: "db/changelog/master.sql", Kind: "file_hash"},
				},
				MissingBlobs: []client.SourceMissingBlob{
					{Path: "db/changelog/master.sql", Hash: hash},
				},
			}}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "ok" || calls != 2 {
		t.Fatalf("result=%q calls=%d, want ok and 2 calls", got, calls)
	}
	if len(manifests) != 2 || manifests[0] == nil || manifests[1] == nil {
		t.Fatalf("expected source manifests on both calls: %+v", manifests)
	}
	if got := manifests[0].WorkspaceRef; got == nil || got.RootPath != root || got.WorkDir != filepath.Join(root, "db") {
		t.Fatalf("workspace_ref = %+v, want root=%q work_dir=%q", got, root, filepath.Join(root, "db"))
	}
	if manifests[1].Files["db/changelog/master.sql"] != hash {
		t.Fatalf("manifest file hash = %q, want %q", manifests[1].Files["db/changelog/master.sql"], hash)
	}
	if gotEntries := manifests[1].Directories["db/changelog"].Entries; len(gotEntries) != 2 || gotEntries[0].Name != "changes" || gotEntries[0].Kind != "directory" || gotEntries[1].Name != "master.sql" {
		t.Fatalf("directory entries = %+v", gotEntries)
	}
	if len(uploader.digests) != 1 || uploader.digests[0] != strings.TrimPrefix(hash, "sha256:") || uploader.bodies[0] != "select 1;\n" {
		t.Fatalf("uploads = %+v bodies=%+v", uploader.digests, uploader.bodies)
	}
	if len(progress.events) == 0 || progress.events[0].Stage != ProgressStageStart || progress.events[len(progress.events)-1].Stage != ProgressStageComplete {
		t.Fatalf("progress events = %+v", progress.events)
	}
	var uploadComplete, syncComplete *ProgressEvent
	for i := range progress.events {
		if progress.events[i].Stage == ProgressStageUploadComplete {
			uploadComplete = &progress.events[i]
		}
		if progress.events[i].Stage == ProgressStageComplete {
			syncComplete = &progress.events[i]
		}
	}
	if uploadComplete == nil || uploadComplete.Path != "db/changelog/master.sql" || uploadComplete.Bytes != int64(len("select 1;\n")) || uploadComplete.UploadIndex != 1 || uploadComplete.UploadCount != 1 || strings.Contains(uploadComplete.Path, root) {
		t.Fatalf("upload completion = %+v", uploadComplete)
	}
	if syncComplete == nil || syncComplete.Bytes != int64(len("select 1;\n")) {
		t.Fatalf("sync completion = %+v", syncComplete)
	}
}

func TestExecuteKeepsInitialAcceptedAndInitialErrorSilent(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "accepted"},
		{name: "initial error", err: errors.New("gateway unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			progress := &recordingProgress{}
			_, err := Execute[string](context.Background(), Options{Enabled: true, Progress: progress}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
				return "ok", test.err
			})
			if !errors.Is(err, test.err) {
				t.Fatalf("error = %v, want %v", err, test.err)
			}
			if len(progress.events) != 0 {
				t.Fatalf("initial request events = %+v, want none", progress.events)
			}
			if !progress.finished {
				t.Fatal("progress lifecycle was not finished")
			}
		})
	}
}

func TestExecuteEmitsFilesystemOperationStartBeforeAccess(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceFile(root, "db/one.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	progress := &recordingProgress{}
	checks := progressCheckingFS{FileSystem: inputset.OSFileSystem{}, progress: progress, t: t}
	calls := 0
	_, err := Execute[string](context.Background(), Options{Enabled: true, WorkspaceRoot: root, FileSystem: checks, Progress: progress}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		calls++
		if calls == 1 {
			return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "query.sql", Kind: "file_hash"},
				{Path: "db", Kind: "directory_listing"},
			}}}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []ProgressStage{ProgressStageStart, ProgressStageRound, ProgressStageDirectoryListStart, ProgressStageDirectoryListed, ProgressStageFileHashStart, ProgressStageFileHashed, ProgressStageRetry, ProgressStageComplete}
	if len(progress.events) != len(want) {
		t.Fatalf("events = %+v, want stages %v", progress.events, want)
	}
	for i, stage := range want {
		if progress.events[i].Stage != stage {
			t.Fatalf("event %d stage = %q, want %q; events=%+v", i, progress.events[i].Stage, stage, progress.events)
		}
	}
}

func TestExecuteReportsActualUploadByteCheckpoint(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("x", int(uploadProgressCheckpoint)+17)
	if err := writeSourceFile(root, "large.sql", content); err != nil {
		t.Fatal(err)
	}
	hash := "sha256:" + inputset.HashContent([]byte(content))
	progress := &recordingProgress{}
	calls := 0
	_, err := Execute[string](context.Background(), Options{Enabled: true, WorkspaceRoot: root, Uploader: &recordingUploader{}, Progress: progress}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		calls++
		if calls == 1 {
			return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{MissingBlobs: []client.SourceMissingBlob{{Path: "large.sql", Hash: hash}}}}
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint, complete *ProgressEvent
	for i := range progress.events {
		event := &progress.events[i]
		if event.Stage == ProgressStageUploadBytes {
			checkpoint = event
		}
		if event.Stage == ProgressStageUploadComplete {
			complete = event
		}
	}
	if checkpoint == nil || checkpoint.Bytes < uploadProgressCheckpoint || checkpoint.Bytes >= int64(len(content)) || checkpoint.UploadIndex != 1 || checkpoint.UploadCount != 1 {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
	if complete == nil || complete.Bytes != int64(len(content)) || complete.TotalBytes != int64(len(content)) {
		t.Fatalf("completion = %+v", complete)
	}
}

func TestExecutePropagatesNonRecoverableError(t *testing.T) {
	want := errors.New("boom")
	_, err := Execute[string](context.Background(), Options{
		Enabled: true,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestExecuteDisabledBypassesManifest(t *testing.T) {
	calls := 0
	got, err := Execute[string](context.Background(), Options{}, client.PrepareJobRequest{}, func(_ context.Context, req client.PrepareJobRequest) (string, error) {
		calls++
		if req.SourceManifest != nil {
			t.Fatalf("SourceManifest = %+v, want nil", req.SourceManifest)
		}
		return "ok", nil
	})
	if err != nil || got != "ok" || calls != 1 {
		t.Fatalf("Execute disabled result=%q calls=%d err=%v", got, calls, err)
	}
}

func TestExecuteRejectsUnsafeManifestPaths(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "empty", path: "", want: "empty"},
		{name: "backslash", path: `db\query.sql`, want: "slash separators"},
		{name: "parent", path: "../query.sql", want: "escapes workspace"},
		{name: "absolute", path: "/query.sql", want: "escapes workspace"},
		{name: "drive", path: "C:/query.sql", want: "escapes workspace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Execute[string](context.Background(), Options{
				Enabled:       true,
				WorkspaceRoot: root,
			}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
				return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
					Code:    "source_inputs_missing",
					Message: "missing",
					MissingManifestEntries: []client.SourceMissingManifestEntry{
						{Path: tt.path, Kind: "file_hash"},
					},
				}}
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}

func TestExecuteRejectsUnsupportedManifestEntryKind(t *testing.T) {
	_, err := Execute[string](context.Background(), Options{
		Enabled: true,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "query.sql", Kind: "symlink"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported source manifest entry kind") {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
}

func TestExecuteRejectsMissingSourceFile(t *testing.T) {
	root := t.TempDir()
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "missing.sql", Kind: "file_hash"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "hash source file missing.sql") {
		t.Fatalf("expected missing source file error, got %v", err)
	}
}

func TestExecuteHandlesDuplicateManifestEntryInOneRound(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	progress := &recordingProgress{}
	got, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		Progress:      progress,
	}, client.PrepareJobRequest{}, func(_ context.Context, req client.PrepareJobRequest) (string, error) {
		calls++
		if calls == 1 {
			return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
				Code:    "source_inputs_missing",
				Message: "missing",
				MissingManifestEntries: []client.SourceMissingManifestEntry{
					{Path: "query.sql", Kind: "file_hash"},
					{Path: "query.sql", Kind: "file_hash"},
				},
			}}
		}
		if got := req.SourceManifest.Files["query.sql"]; got == "" {
			t.Fatalf("expected query.sql hash in retry manifest: %+v", req.SourceManifest)
		}
		return "ok", nil
	})
	if err != nil || got != "ok" || calls != 2 {
		t.Fatalf("result=%q calls=%d err=%v", got, calls, err)
	}
	starts, completes := 0, 0
	for _, event := range progress.events {
		if event.Stage == ProgressStageFileHashStart {
			starts++
		}
		if event.Stage == ProgressStageFileHashed {
			completes++
		}
	}
	if starts != 2 || completes != 2 {
		t.Fatalf("hash operation pairs: starts=%d completes=%d events=%+v", starts, completes, progress.events)
	}
}

func TestExecuteRejectsDirectoryRequestedAsFileHash(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o700); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "dir", Kind: "file_hash"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "hash source file dir") {
		t.Fatalf("expected directory-as-file error, got %v", err)
	}
}

func TestExecuteRejectsMissingSourceDirectory(t *testing.T) {
	root := t.TempDir()
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "missing-dir", Kind: "directory_listing"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "list source directory missing-dir") {
		t.Fatalf("expected missing source directory error, got %v", err)
	}
}

func TestExecuteRejectsBlobHashMismatch(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		Uploader:      &recordingUploader{},
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingBlobs: []client.SourceMissingBlob{
				{Path: "query.sql", Hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("expected hash mismatch, got %v", err)
	}
}

func TestExecuteUploadsMultipleSortedBlobs(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "a.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceFile(root, "b.sql", "select 2;\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeSourceFile(root, "c.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	hashA := "sha256:" + inputset.HashContent([]byte("select 1;\n"))
	hashB := "sha256:" + inputset.HashContent([]byte("select 2;\n"))
	uploader := &recordingUploader{}
	progress := &recordingProgress{}
	calls := 0
	var retryManifest *client.SourceManifest

	got, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		Uploader:      uploader,
		Progress:      progress,
	}, client.PrepareJobRequest{}, func(_ context.Context, req client.PrepareJobRequest) (string, error) {
		calls++
		if calls == 1 {
			return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
				Code:    "source_inputs_missing",
				Message: "missing",
				MissingBlobs: []client.SourceMissingBlob{
					{Path: "c.sql", Hash: hashA},
					{Path: "b.sql", Hash: hashB},
					{Path: "a.sql", Hash: hashA},
				},
			}}
		}
		retryManifest = req.SourceManifest
		return "ok", nil
	})
	if err != nil || got != "ok" || calls != 2 {
		t.Fatalf("result=%q calls=%d err=%v", got, calls, err)
	}
	if len(uploader.digests) != 2 {
		t.Fatalf("uploads = %+v, want two unique digests", uploader.digests)
	}
	if retryManifest == nil || retryManifest.Files["a.sql"] != hashA || retryManifest.Files["c.sql"] != hashA {
		t.Fatalf("retry manifest files = %+v, want both duplicate-content paths", retryManifest)
	}
	var starts []ProgressEvent
	var complete ProgressEvent
	for _, event := range progress.events {
		if event.Stage == ProgressStageUploadStart {
			starts = append(starts, event)
		}
		if event.Stage == ProgressStageComplete {
			complete = event
		}
	}
	if len(starts) != 2 || starts[0].UploadIndex != 1 || starts[0].UploadCount != 2 || starts[1].UploadIndex != 2 || starts[1].UploadCount != 2 {
		t.Fatalf("upload starts = %+v", starts)
	}
	wantBytes := int64(len("select 1;\n") + len("select 2;\n"))
	if complete.UploadedBlobs != 2 || complete.Bytes != wantBytes {
		t.Fatalf("complete = %+v, want bytes=%d", complete, wantBytes)
	}
}

func TestExecuteRejectsInvalidBlobHash(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	for _, hash := range []string{
		"sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"sha256:aaaa",
		"sha512:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		t.Run(hash, func(t *testing.T) {
			_, err := Execute[string](context.Background(), Options{
				Enabled:       true,
				WorkspaceRoot: root,
				Uploader:      &recordingUploader{},
			}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
				return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
					Code:    "source_inputs_missing",
					Message: "missing",
					MissingBlobs: []client.SourceMissingBlob{
						{Path: "query.sql", Hash: hash},
					},
				}}
			})
			if err == nil || !strings.Contains(err.Error(), "invalid source blob hash") {
				t.Fatalf("expected invalid blob hash, got %v", err)
			}
		})
	}
}

func TestExecuteRejectsMissingBlobPath(t *testing.T) {
	root := t.TempDir()
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		Uploader:      &recordingUploader{},
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingBlobs: []client.SourceMissingBlob{
				{Path: "missing.sql", Hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "read source blob missing.sql") {
		t.Fatalf("expected missing blob path error, got %v", err)
	}
}

func TestExecuteRejectsUnsafeBlobPath(t *testing.T) {
	root := t.TempDir()
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		Uploader:      &recordingUploader{},
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingBlobs: []client.SourceMissingBlob{
				{Path: "../query.sql", Hash: "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected unsafe blob path error, got %v", err)
	}
}

func TestExecuteRejectsBlobUploadWithoutUploader(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	hash := "sha256:" + inputset.HashContent([]byte("select 1;\n"))
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingBlobs: []client.SourceMissingBlob{
				{Path: "query.sql", Hash: hash},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "source blob upload is not configured") {
		t.Fatalf("expected missing uploader error, got %v", err)
	}
}

func TestExecuteReturnsUploaderError(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	hash := "sha256:" + inputset.HashContent([]byte("select 1;\n"))
	want := errors.New("upload failed")
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		Uploader:      &recordingUploader{err: want},
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingBlobs: []client.SourceMissingBlob{
				{Path: "query.sql", Hash: hash},
			},
		}}
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected uploader error %v, got %v", want, err)
	}
}

func TestExecuteDetectsFileChangeBeforeBlobUpload(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		MaxRounds:     3,
		Uploader:      &recordingUploader{},
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		calls++
		if calls == 1 {
			return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
				Code:    "source_inputs_missing",
				Message: "missing",
				MissingManifestEntries: []client.SourceMissingManifestEntry{
					{Path: "query.sql", Kind: "file_hash"},
				},
			}}
		}
		if err := writeSourceFile(root, "query.sql", "select 2;\n"); err != nil {
			t.Fatal(err)
		}
		hash := "sha256:" + inputset.HashContent([]byte("select 2;\n"))
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingBlobs: []client.SourceMissingBlob{
				{Path: "query.sql", Hash: hash},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "source file changed during sync") {
		t.Fatalf("expected file changed error, got %v", err)
	}
}

func TestExecuteDetectsFileHashChangeDuringSync(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		MaxRounds:     3,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		calls++
		if calls == 2 {
			if err := writeSourceFile(root, "query.sql", "select 2;\n"); err != nil {
				t.Fatal(err)
			}
		}
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "query.sql", Kind: "file_hash"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "source file changed during sync") {
		t.Fatalf("expected file changed error, got %v", err)
	}
}

func TestExecuteStopsWhenRepeatedRequestMakesNoProgress(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		MaxRounds:     3,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		calls++
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "query.sql", Kind: "file_hash"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "made no progress") {
		t.Fatalf("expected no progress error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestExecuteStopsWhenRepeatedDirectoryListingMakesNoProgress(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "dir/a.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		MaxRounds:     3,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		calls++
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "dir", Kind: "directory_listing"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "made no progress") {
		t.Fatalf("expected no progress error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestExecuteDetectsDirectoryChangeDuringSync(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "dir/a.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		MaxRounds:     3,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		calls++
		if calls == 2 {
			if err := writeSourceFile(root, "dir/b.sql", "select 2;\n"); err != nil {
				t.Fatal(err)
			}
		}
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "dir", Kind: "directory_listing"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "source directory changed during sync") {
		t.Fatalf("expected directory changed error, got %v", err)
	}
}

func TestExecuteStopsWhenDuplicateBlobMakesNoProgress(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	hash := "sha256:" + inputset.HashContent([]byte("select 1;\n"))
	uploader := &recordingUploader{}
	calls := 0
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		MaxRounds:     3,
		Uploader:      uploader,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		calls++
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingBlobs: []client.SourceMissingBlob{
				{Path: "query.sql", Hash: hash},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "made no progress") {
		t.Fatalf("expected no progress error, got %v", err)
	}
	if calls != 2 || len(uploader.digests) != 1 {
		t.Fatalf("calls=%d uploads=%d, want 2 calls and 1 upload", calls, len(uploader.digests))
	}
}

func TestExecuteStopsAtMaxRounds(t *testing.T) {
	root := t.TempDir()
	if err := writeSourceFile(root, "query.sql", "select 1;\n"); err != nil {
		t.Fatal(err)
	}
	_, err := Execute[string](context.Background(), Options{
		Enabled:       true,
		WorkspaceRoot: root,
		MaxRounds:     1,
	}, client.PrepareJobRequest{}, func(context.Context, client.PrepareJobRequest) (string, error) {
		return "", &client.SourceInputsMissingError{Response: client.SourceInputsMissingErrorResponse{
			Code:    "source_inputs_missing",
			Message: "missing",
			MissingManifestEntries: []client.SourceMissingManifestEntry{
				{Path: "query.sql", Kind: "file_hash"},
			},
		}}
	})
	if err == nil || !strings.Contains(err.Error(), "max rounds") {
		t.Fatalf("expected max rounds error, got %v", err)
	}
}

func writeSourceFile(root, rel, content string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := mkdirParent(path); err != nil {
		return err
	}
	return writeFile(path, content)
}

func mkdirParent(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
