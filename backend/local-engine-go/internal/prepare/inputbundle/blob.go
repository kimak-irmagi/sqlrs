package inputbundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// PublishBlob stores engine-supplied helper bytes by their exact SHA-256
// digest. A durable capture_helper lease must be committed before a worker
// may rely on the returned digest. See the planning recovery storage schema.
func (s Store) PublishBlob(ctx context.Context, data []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.Root == "" || len(data) == 0 {
		return "", fmt.Errorf("empty artifact store or blob")
	}
	digest := hash(data)
	if err := os.MkdirAll(filepath.Join(s.Root, "blobs"), 0700); err != nil {
		return "", err
	}
	if info, err := os.Lstat(s.blobPath(digest)); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("blob destination is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := publishBytes(s.blobPath(digest), data); err != nil {
		return "", err
	}
	return digest, nil
}

// ReadBlob reacquires exact bytes on a replacement worker. A missing,
// oversized, symlinked, or corrupt blob is unavailable, never replaced with
// bytes from the current engine binary.
func (s Store) ReadBlob(ctx context.Context, digest string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Root == "" || !digestPattern.MatchString(digest) || maxBytes <= 0 {
		return nil, fmt.Errorf("invalid blob reference")
	}
	path := s.blobPath(digest)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("blob is not a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != info.Size() || hash(data) != digest {
		return nil, fmt.Errorf("blob differs from its digest")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

// MaterializeBlob makes a verified job-private, read-only mount source from
// the leased digest. An existing different file is never overwritten.
func (s Store) MaterializeBlob(ctx context.Context, digest, destination string, maxBytes int64) error {
	data, err := s.ReadBlob(ctx, digest, maxBytes)
	if err != nil {
		return err
	}
	if destination == "" {
		return fmt.Errorf("empty blob destination")
	}
	if info, err := os.Lstat(destination); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("blob destination is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	return publishBytes(destination, data)
}
