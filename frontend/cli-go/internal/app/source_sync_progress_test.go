package app

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/remotesource"
)

func TestSourceSyncVerboseProgressRendersEveryEvent(t *testing.T) {
	var output bytes.Buffer
	progress := newSourceSyncProgress(&output, true)
	progress.Update(remotesource.ProgressEvent{Stage: remotesource.ProgressStageRound, Round: 1, ManifestEntries: 3, Blobs: 0})
	progress.Update(remotesource.ProgressEvent{Stage: remotesource.ProgressStageUploadComplete, Path: "db/schema.sql", Digest: "0123456789ab", Bytes: 12698, TotalBytes: 20600, UploadIndex: 2, UploadCount: 3})
	progress.Update(remotesource.ProgressEvent{Stage: remotesource.ProgressStageComplete, FileHashes: 3, UploadedBlobs: 3, Bytes: 42700})

	text := output.String()
	if !strings.Contains(text, "round 1 requested manifest=3 blobs=0") || !strings.Contains(text, "uploaded db/schema.sql sha256:0123456789ab 12.4 KiB (2/3)") || !strings.Contains(text, "complete files=3 listings=0 uploaded=3 bytes=41.7 KiB") {
		t.Fatalf("verbose progress = %q", text)
	}
}

func TestSourceSyncProgressFormatsUploadContextAndHumanBytes(t *testing.T) {
	tests := []struct {
		event remotesource.ProgressEvent
		want  string
	}{
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageUploadStart, Path: "schema.sql", TotalBytes: 20600, UploadIndex: 2, UploadCount: 3}, want: "source sync: uploading schema.sql 0/20.1 KiB (2/3)"},
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageUploadBytes, Path: "schema.sql", Bytes: 12698, TotalBytes: 20600, UploadIndex: 2, UploadCount: 3}, want: "source sync: uploading schema.sql 12.4/20.1 KiB (2/3)"},
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageUploadComplete, Path: "schema.sql", Bytes: 12698, UploadIndex: 2, UploadCount: 3}, want: "source sync: uploaded schema.sql 12.4 KiB (2/3)"},
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageUploadComplete, Path: "small.sql", Bytes: 42, UploadIndex: 1, UploadCount: 1}, want: "source sync: uploaded small.sql 42 B (1/1)"},
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageComplete, FileHashes: 3, DirectoryListings: 1, UploadedBlobs: 3, Bytes: 42700}, want: "source sync: complete files=3 listings=1 uploaded=3 bytes=41.7 KiB"},
	}
	for _, test := range tests {
		if got := formatSourceSyncProgressLine(test.event); got != test.want {
			t.Errorf("formatSourceSyncProgressLine(%+v) = %q, want %q", test.event, got, test.want)
		}
	}
}

func TestSourceSyncOperationUsesOnlyActiveOperationEvents(t *testing.T) {
	tests := []struct {
		event remotesource.ProgressEvent
		want  string
	}{
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageFileHashStart, Path: "a.sql"}, want: "source sync: hashing a.sql"},
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageFileHashed, Path: "a.sql"}, want: ""},
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageDirectoryListStart, Path: "db"}, want: "source sync: listing db"},
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageDirectoryListed, Path: "db"}, want: ""},
		{event: remotesource.ProgressEvent{Stage: remotesource.ProgressStageUploadComplete, Path: "a.sql"}, want: ""},
	}
	for _, test := range tests {
		if got := formatSourceSyncOperation(test.event); got != test.want {
			t.Errorf("formatSourceSyncOperation(%+v) = %q, want %q", test.event, got, test.want)
		}
	}
}

func TestSourceSyncNormalNonTTYProgressIsSilent(t *testing.T) {
	var output bytes.Buffer
	if progress := newSourceSyncProgress(&output, false); progress != nil {
		t.Fatalf("normal non-TTY progress = %#v, want nil", progress)
	}
	if strings.Contains(output.String(), "\x1b") {
		t.Fatalf("unexpected ANSI output: %q", output.String())
	}
}

func TestSourceSyncProgressCoversNilAndNonTerminalFileBoundaries(t *testing.T) {
	if progress := newSourceSyncProgress(nil, false); progress != nil {
		t.Fatalf("nil writer progress = %#v", progress)
	}
	var nilProgress *sourceSyncProgress
	nilProgress.Update(remotesource.ProgressEvent{Stage: remotesource.ProgressStageStart})
	(&sourceSyncProgress{}).stop()

	oldTerminal := isTerminalWriterFn
	isTerminalWriterFn = func(*os.File) bool { return false }
	t.Cleanup(func() { isTerminalWriterFn = oldTerminal })
	file, err := os.CreateTemp(t.TempDir(), "source-sync-nontty")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if progress := newSourceSyncProgress(file, false); progress != nil {
		t.Fatalf("non-terminal file progress = %#v", progress)
	}
}

func TestSourceSyncTTYProgressUsesDelayedSpinnerAndClears(t *testing.T) {
	oldTerminal := isTerminalWriterFn
	oldDelay, oldTick := spinnerInitialDelay, spinnerTickInterval
	isTerminalWriterFn = func(*os.File) bool { return true }
	spinnerInitialDelay, spinnerTickInterval = time.Millisecond, time.Millisecond
	t.Cleanup(func() { isTerminalWriterFn = oldTerminal; spinnerInitialDelay, spinnerTickInterval = oldDelay, oldTick })

	file, err := os.CreateTemp(t.TempDir(), "source-sync-progress")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	progress := newSourceSyncProgress(file, false)
	progress.Update(remotesource.ProgressEvent{Stage: remotesource.ProgressStageUploadStart, Path: "db/schema.sql"})
	time.Sleep(8 * time.Millisecond)
	progress.Update(remotesource.ProgressEvent{Stage: remotesource.ProgressStageComplete})
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "source sync: uploading db/schema.sql") || !strings.Contains(text, "\r") {
		t.Fatalf("spinner output = %q", text)
	}
	if strings.Contains(text, "\x1b") {
		t.Fatalf("spinner emitted ANSI: %q", text)
	}
}

func TestSourceSyncProgressFormattersCoverSemanticEvents(t *testing.T) {
	events := []remotesource.ProgressEvent{
		{Stage: remotesource.ProgressStageStart},
		{Stage: remotesource.ProgressStageRound, Round: 2, ManifestEntries: 1, Blobs: 3},
		{Stage: remotesource.ProgressStageFileHashStart, Path: "a.sql"},
		{Stage: remotesource.ProgressStageFileHashed, Path: "a.sql", Digest: "0123456789ab"},
		{Stage: remotesource.ProgressStageDirectoryListStart, Path: "db"},
		{Stage: remotesource.ProgressStageDirectoryListed, Path: "db"},
		{Stage: remotesource.ProgressStageUploadStart, Path: "a.sql", TotalBytes: 10},
		{Stage: remotesource.ProgressStageUploadBytes, Path: "a.sql", Bytes: 5, TotalBytes: 10},
		{Stage: remotesource.ProgressStageUploadComplete, Path: "a.sql", Bytes: 10},
		{Stage: remotesource.ProgressStageRetry, Round: 2, FileHashes: 1, DirectoryListings: 1, UploadedBlobs: 1},
		{Stage: remotesource.ProgressStageComplete, FileHashes: 1, DirectoryListings: 1, UploadedBlobs: 1},
		{Stage: remotesource.ProgressStageError, Error: " boom "},
	}
	for _, event := range events {
		if got := formatSourceSyncProgressLine(event); got == "" {
			t.Fatalf("empty verbose line for %+v", event)
		}
	}
	if got := formatSourceSyncProgressLine(remotesource.ProgressEvent{Stage: "unknown"}); got != "" {
		t.Fatalf("unknown verbose event = %q", got)
	}
	if got := formatSourceSyncOperation(remotesource.ProgressEvent{Stage: "unknown"}); got != "source sync" {
		t.Fatalf("unknown operation = %q", got)
	}
}

func TestSourceSyncSpinnerCanStopBeforeDelayAndStopAgain(t *testing.T) {
	oldTerminal := isTerminalWriterFn
	isTerminalWriterFn = func(*os.File) bool { return true }
	t.Cleanup(func() { isTerminalWriterFn = oldTerminal })
	file, err := os.CreateTemp(t.TempDir(), "source-sync-early-stop")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	progress := newSourceSyncProgress(file, false)
	progress.Update(remotesource.ProgressEvent{Stage: remotesource.ProgressStageError, Error: "upload failed"})
	progress.Update(remotesource.ProgressEvent{Stage: remotesource.ProgressStageComplete})
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("spinner wrote before delay: %d bytes", info.Size())
	}
}

func TestSourceSyncProgressFinishStopsSilentSpinnerLifecycle(t *testing.T) {
	oldTerminal := isTerminalWriterFn
	isTerminalWriterFn = func(*os.File) bool { return true }
	t.Cleanup(func() { isTerminalWriterFn = oldTerminal })
	file, err := os.CreateTemp(t.TempDir(), "source-sync-finish")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	progress := newSourceSyncProgress(file, false)
	finisher, ok := progress.(interface{ Finish() })
	if !ok {
		t.Fatalf("progress %T does not expose lifecycle finish", progress)
	}
	finisher.Finish()
	if info, err := file.Stat(); err != nil || info.Size() != 0 {
		t.Fatalf("silent finish stat=%+v err=%v", info, err)
	}
}
