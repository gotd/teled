// Package objstore provides ObjectStore implementations. The local-filesystem
// backend (FS) is the v1 default; an S3-compatible backend can be added behind
// the same teled.ObjectStore interface.
package objstore

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/go-faster/errors"

	"github.com/gotd/teled"
	"github.com/gotd/teled/internal/obs"
)

// FS is a local-filesystem ObjectStore. Objects are stored under a base
// directory, sharded by the first bytes of the key to avoid huge directories.
type FS struct {
	base string
	obs  observability
}

var _ teled.ObjectStore = (*FS)(nil)

// NewFS creates an FS rooted at base, creating the directory if needed.
// providers supplies the OpenTelemetry tracer and meter for this layer.
func NewFS(base string, providers obs.Providers) (*FS, error) {
	if err := os.MkdirAll(base, 0o750); err != nil {
		return nil, errors.Wrap(err, "mkdir base")
	}

	return &FS{base: base, obs: newObservability(providers)}, nil
}

// path maps a key to its on-disk location, sharded as base/ab/cd/<key>.
func (s *FS) path(key string) string {
	if len(key) >= 4 {
		return filepath.Join(s.base, key[0:2], key[2:4], key)
	}

	return filepath.Join(s.base, "_", key)
}

// Put implements teled.ObjectStore. It writes atomically via a temp file.
func (s *FS) Put(ctx context.Context, key string, r io.Reader, _ int64, _ teled.PutOptions) (rerr error) {
	done := s.observe(ctx, "put")
	defer func() { done(rerr) }()

	dst := s.path(key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return errors.Wrap(err, "mkdir")
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*")
	if err != nil {
		return errors.Wrap(err, "create temp")
	}

	tmpName := tmp.Name()

	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // No-op once renamed.
	}()

	if _, err := io.Copy(tmp, r); err != nil {
		return errors.Wrap(err, "copy")
	}

	if err := tmp.Close(); err != nil {
		return errors.Wrap(err, "close temp")
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return errors.Wrap(err, "rename")
	}

	return nil
}

// Get implements teled.ObjectStore.
func (s *FS) Get(ctx context.Context, key string) (_ io.ReadCloser, rerr error) {
	done := s.observe(ctx, "get")
	defer func() { done(rerr) }()

	f, err := os.Open(s.path(key))
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}

	return f, nil
}

// GetRange implements teled.ObjectStore, returning length bytes from offset.
func (s *FS) GetRange(ctx context.Context, key string, offset, length int64) (_ io.ReadCloser, rerr error) {
	done := s.observe(ctx, "get_range")
	defer func() { done(rerr) }()

	f, err := os.Open(s.path(key))
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, errors.Wrap(err, "seek")
	}

	return &limitedFile{f: f, r: io.LimitReader(f, length)}, nil
}

// Stat implements teled.ObjectStore.
func (s *FS) Stat(ctx context.Context, key string) (_ teled.ObjectInfo, rerr error) {
	done := s.observe(ctx, "stat")
	defer func() { done(rerr) }()

	fi, err := os.Stat(s.path(key))
	if err != nil {
		return teled.ObjectInfo{}, errors.Wrap(err, "stat")
	}

	return teled.ObjectInfo{Size: fi.Size()}, nil
}

// Delete implements teled.ObjectStore. A missing object is not an error.
func (s *FS) Delete(ctx context.Context, key string) (rerr error) {
	done := s.observe(ctx, "delete")
	defer func() { done(rerr) }()

	if err := os.Remove(s.path(key)); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove")
	}

	return nil
}

// limitedFile bounds reads to a range while closing the underlying file.
type limitedFile struct {
	f *os.File
	r io.Reader
}

func (l *limitedFile) Read(p []byte) (int, error) { return l.r.Read(p) }
func (l *limitedFile) Close() error               { return l.f.Close() }
