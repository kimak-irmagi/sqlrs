package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"path/filepath"
)

type ArtifactStore interface {
	PublishVerified(context.Context, io.Reader, string) (Artifact, error)
}
type DirectoryArtifactStore struct{ root string }

func NewDirectoryArtifactStore(root string) (*DirectoryArtifactStore, error) {
	abs, err := preparePrivateDirectory(root)
	if err != nil {
		return nil, err
	}
	return &DirectoryArtifactStore{root: abs}, nil
}
func (s *DirectoryArtifactStore) PublishVerified(ctx context.Context, source io.Reader, expected string) (Artifact, error) {
	return s.publishVerified(ctx, source, expected, replaceFile)
}

func (s *DirectoryArtifactStore) publishVerified(ctx context.Context, source io.Reader, expected string, replace func(string, string) error) (Artifact, error) {
	if !validDigest(expected) {
		return nil, ErrInvalidDeclaration
	}
	target := filepath.Join(s.root, expected[7:])
	if info, err := os.Lstat(target); err == nil {
		if isLinkLike(info) || !info.Mode().IsRegular() {
			return nil, ErrUnsafePath
		}
		existing, err := os.Open(target)
		if err != nil {
			return nil, err
		}
		got, hashErr := hashReader(ctx, existing)
		if hashErr == nil && got == expected {
			_, _ = existing.Seek(0, io.SeekStart)
			return &fileArtifact{File: existing}, nil
		}
		existing.Close()
		_ = os.Remove(target)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	temporary, err := os.CreateTemp(s.root, ".tmp-")
	if err != nil {
		return nil, err
	}
	name := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	publicationBarrier("artifact", publicationTemporaryCreated)
	if err := temporary.Chmod(0o600); err != nil {
		return nil, err
	}
	hashWriter := newHashingWriter(temporary)
	if _, err := copyContext(ctx, hashWriter, source); err != nil {
		return nil, err
	}
	if hashWriter.digest() != expected {
		return nil, ErrChanged
	}
	publicationBarrier("artifact", publicationContentWritten)
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	publicationBarrier("artifact", publicationFileSynced)
	if err := temporary.Chmod(0o400); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := replace(name, target); err != nil {
		// Another process may have won publication of the same digest while this
		// writer was syncing. On Windows its validation handle can transiently
		// block replacement, so accept only the already-published object whose
		// bytes independently verify against the requested content address.
		if winner, winnerErr := openVerifiedArtifact(ctx, target, expected); winnerErr == nil {
			return winner, nil
		}
		return nil, err
	}
	committed = true
	publicationBarrier("artifact", publicationReplaced)
	if err := syncDirectory(s.root); err != nil {
		return nil, err
	}
	publicationBarrier("artifact", publicationDirectorySynced)
	return openVerifiedArtifact(ctx, target, expected)
}

func openVerifiedArtifact(ctx context.Context, target, expected string) (Artifact, error) {
	info, err := os.Lstat(target)
	if err != nil {
		return nil, err
	}
	if isLinkLike(info) || !info.Mode().IsRegular() {
		return nil, ErrUnsafePath
	}
	file, err := os.Open(target)
	if err != nil {
		return nil, err
	}
	got, err := hashReader(ctx, file)
	if err != nil || got != expected {
		file.Close()
		return nil, ErrChanged
	}
	_, _ = file.Seek(0, io.SeekStart)
	return &fileArtifact{File: file}, nil
}

type fileArtifact struct{ *os.File }

func (*fileArtifact) Kind() string { return "file" }

type hashingWriter struct {
	writer io.Writer
	hash   hash.Hash
}

func newHashingWriter(writer io.Writer) *hashingWriter {
	return &hashingWriter{writer: writer, hash: sha256.New()}
}
func (w *hashingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		_, _ = w.hash.Write(p[:n])
	}
	return n, err
}
func (w *hashingWriter) digest() string { return "sha256:" + hex.EncodeToString(w.hash.Sum(nil)) }
func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buffer := make([]byte, 64*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, rerr := src.Read(buffer)
		if n > 0 {
			written, werr := dst.Write(buffer[:n])
			total += int64(written)
			if werr != nil {
				return total, werr
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if rerr == io.EOF {
			return total, nil
		}
		if rerr != nil {
			return total, rerr
		}
	}
}
