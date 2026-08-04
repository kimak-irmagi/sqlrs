package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sqlrs/cli/internal/remotesource"
)

type sourceSyncProgress struct {
	writer      io.Writer
	verbose     bool
	interactive bool
	mu          sync.Mutex
	label       string
	done        chan struct{}
	finished    chan struct{}
	once        sync.Once
}

func newSourceSyncProgress(writer io.Writer, verbose bool) remotesource.Progress {
	if writer == nil {
		return nil
	}
	interactive := false
	if file, ok := writer.(*os.File); ok {
		interactive = isTerminalWriterFn(file)
	}
	if !verbose && !interactive {
		return nil
	}
	p := &sourceSyncProgress{writer: writer, verbose: verbose, interactive: interactive}
	if interactive && !verbose {
		p.done = make(chan struct{})
		p.finished = make(chan struct{})
		go p.runSpinner()
	}
	return p
}

func (p *sourceSyncProgress) Update(event remotesource.ProgressEvent) {
	if p == nil {
		return
	}
	if p.verbose {
		if line := formatSourceSyncProgressLine(event); line != "" {
			fmt.Fprintln(p.writer, line)
		}
		return
	}
	if event.Stage == remotesource.ProgressStageComplete || event.Stage == remotesource.ProgressStageError {
		p.stop()
		return
	}
	p.mu.Lock()
	if label := formatSourceSyncOperation(event); label != "" {
		p.label = label
	}
	p.mu.Unlock()
}

func (p *sourceSyncProgress) runSpinner() {
	defer close(p.finished)
	timer := time.NewTimer(spinnerInitialDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-p.done:
		return
	}
	ticker := time.NewTicker(spinnerTickInterval)
	defer ticker.Stop()
	frames, index, width := []string{"-", "\\", "|", "/"}, 0, 1
	for {
		select {
		case <-p.done:
			clearLineOut(p.writer, width)
			return
		case <-ticker.C:
			p.mu.Lock()
			label := p.label
			p.mu.Unlock()
			if label == "" {
				continue
			}
			clearLineOut(p.writer, width)
			line := label + " " + frames[index]
			fmt.Fprint(p.writer, line)
			width = len(line)
			index = (index + 1) % len(frames)
		}
	}
}

func (p *sourceSyncProgress) stop() {
	if p.done == nil {
		return
	}
	p.once.Do(func() { close(p.done); <-p.finished })
}

func (p *sourceSyncProgress) Finish() {
	if p != nil {
		p.stop()
	}
}

func formatSourceSyncOperation(event remotesource.ProgressEvent) string {
	switch event.Stage {
	case remotesource.ProgressStageStart:
		return "source sync: starting"
	case remotesource.ProgressStageRound:
		return fmt.Sprintf("source sync: resolving round %d", event.Round)
	case remotesource.ProgressStageFileHashStart:
		return "source sync: hashing " + event.Path
	case remotesource.ProgressStageDirectoryListStart:
		return "source sync: listing " + event.Path
	case remotesource.ProgressStageUploadStart, remotesource.ProgressStageUploadBytes:
		return fmt.Sprintf("source sync: uploading %s %s%s", event.Path, formatByteProgress(event.Bytes, event.TotalBytes), formatUploadOrdinal(event))
	case remotesource.ProgressStageRetry:
		return fmt.Sprintf("source sync: retrying round %d", event.Round+1)
	case remotesource.ProgressStageFileHashed, remotesource.ProgressStageDirectoryListed, remotesource.ProgressStageUploadComplete:
		return ""
	default:
		return "source sync"
	}
}

func formatSourceSyncProgressLine(event remotesource.ProgressEvent) string {
	digest := event.Digest
	if digest != "" {
		digest = " sha256:" + digest
	}
	switch event.Stage {
	case remotesource.ProgressStageStart:
		return "source sync: started"
	case remotesource.ProgressStageRound:
		return fmt.Sprintf("source sync: round %d requested manifest=%d blobs=%d", event.Round, event.ManifestEntries, event.Blobs)
	case remotesource.ProgressStageFileHashStart:
		return "source sync: hashing " + event.Path
	case remotesource.ProgressStageFileHashed:
		return fmt.Sprintf("source sync: hashed %s%s", event.Path, digest)
	case remotesource.ProgressStageDirectoryListStart:
		return "source sync: listing " + event.Path
	case remotesource.ProgressStageDirectoryListed:
		return "source sync: listed " + event.Path
	case remotesource.ProgressStageUploadStart:
		return fmt.Sprintf("source sync: uploading %s%s %s%s", event.Path, digest, formatByteProgress(0, event.TotalBytes), formatUploadOrdinal(event))
	case remotesource.ProgressStageUploadBytes:
		return fmt.Sprintf("source sync: uploading %s%s %s%s", event.Path, digest, formatByteProgress(event.Bytes, event.TotalBytes), formatUploadOrdinal(event))
	case remotesource.ProgressStageUploadComplete:
		return fmt.Sprintf("source sync: uploaded %s%s %s%s", event.Path, digest, formatByteSize(event.Bytes), formatUploadOrdinal(event))
	case remotesource.ProgressStageRetry:
		return fmt.Sprintf("source sync: retry after round %d (+%d hashes, +%d listings, +%d blobs)", event.Round, event.FileHashes, event.DirectoryListings, event.UploadedBlobs)
	case remotesource.ProgressStageComplete:
		return fmt.Sprintf("source sync: complete files=%d listings=%d uploaded=%d bytes=%s", event.FileHashes, event.DirectoryListings, event.UploadedBlobs, formatByteSize(event.Bytes))
	case remotesource.ProgressStageError:
		return "source sync: failed: " + strings.TrimSpace(event.Error)
	default:
		return ""
	}
}

func formatUploadOrdinal(event remotesource.ProgressEvent) string {
	if event.UploadIndex <= 0 || event.UploadCount <= 0 {
		return ""
	}
	return fmt.Sprintf(" (%d/%d)", event.UploadIndex, event.UploadCount)
}

func formatByteSize(value int64) string {
	const unit = int64(1024)
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	scaled := float64(value)
	selected := units[0]
	for _, candidate := range units {
		scaled /= float64(unit)
		selected = candidate
		if scaled < float64(unit) || candidate == units[len(units)-1] {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", scaled, selected)
}

func formatByteProgress(current, total int64) string {
	if total < 1024 {
		return fmt.Sprintf("%d/%d B", current, total)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	divisor := float64(1024)
	selected := units[0]
	for _, candidate := range units {
		selected = candidate
		if float64(total)/divisor < 1024 || candidate == units[len(units)-1] {
			break
		}
		divisor *= 1024
	}
	if current == 0 {
		return fmt.Sprintf("0/%.1f %s", float64(total)/divisor, selected)
	}
	return fmt.Sprintf("%.1f/%.1f %s", float64(current)/divisor, float64(total)/divisor, selected)
}

var _ remotesource.Progress = (*sourceSyncProgress)(nil)
