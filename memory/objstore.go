// Package memory provides in-memory implementations of teled's storage ports
// (teled.DB and teled.ObjectStore). They keep everything in process memory with
// no external dependencies, which makes them convenient for tests and for
// embedding a teled server without a database. They are safe for concurrent use
// but lose all data when the process exits.
package memory

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/go-faster/errors"

	"github.com/gotd/teled"
)

// ObjectStore is an in-memory teled.ObjectStore. Objects live in a map keyed by
// their opaque key; values are copied on Put and on Get so callers cannot
// mutate stored bytes.
type ObjectStore struct {
	mu   sync.RWMutex
	objs map[string][]byte
}

var _ teled.ObjectStore = (*ObjectStore)(nil)

// NewObjectStore creates an empty in-memory object store.
func NewObjectStore() *ObjectStore {
	return &ObjectStore{objs: make(map[string][]byte)}
}

// Put stores the full contents of r under key, replacing any existing object.
// size is advisory and ignored; the stored object is exactly what r yields.
func (s *ObjectStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ teled.PutOptions) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "read")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.objs[key] = data

	return nil
}

// Get returns the full object. The returned reader is over a private copy.
func (s *ObjectStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.objs[key]
	if !ok {
		return nil, errors.Errorf("object %q not found", key)
	}

	return io.NopCloser(bytes.NewReader(clone(data))), nil
}

// GetRange returns length bytes starting at offset. A range that runs past the
// end of the object is truncated to the available bytes, mirroring an io.Reader.
func (s *ObjectStore) GetRange(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.objs[key]
	if !ok {
		return nil, errors.Errorf("object %q not found", key)
	}

	if offset < 0 || offset > int64(len(data)) {
		return nil, errors.Errorf("offset %d out of range", offset)
	}

	end := offset + length
	if length < 0 || end > int64(len(data)) {
		end = int64(len(data))
	}

	return io.NopCloser(bytes.NewReader(clone(data[offset:end]))), nil
}

// Stat returns the object's size.
func (s *ObjectStore) Stat(_ context.Context, key string) (teled.ObjectInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, ok := s.objs[key]
	if !ok {
		return teled.ObjectInfo{}, errors.Errorf("object %q not found", key)
	}

	return teled.ObjectInfo{Size: int64(len(data))}, nil
}

// Delete removes the object. Deleting a missing object is not an error.
func (s *ObjectStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objs, key)

	return nil
}

// clone returns a copy of b so stored and returned slices never alias.
func clone(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)

	return out
}
