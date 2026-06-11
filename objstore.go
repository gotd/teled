package teled

import (
	"context"
	"io"
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	// Size is the object size in bytes.
	Size int64
}

// PutOptions are optional parameters for ObjectStore.Put.
type PutOptions struct {
	// ContentType is an advisory MIME type. Backends may ignore it; the
	// authoritative MIME is tracked in the files table.
	ContentType string
}

// ObjectStore is an S3-shaped blob store for media (photos, documents, video).
// Keys are opaque; callers use content-addressed keys. The local-filesystem
// implementation lives in internal/objstore; an S3-compatible backend can be
// added behind the same interface.
type ObjectStore interface {
	// Put stores r under key. size is the exact number of bytes in r.
	Put(ctx context.Context, key string, r io.Reader, size int64, opt PutOptions) error
	// Get returns the full object. The caller must Close the reader.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// GetRange returns length bytes starting at offset. It is the primitive
	// behind upload.getFile chunking. The caller must Close the reader.
	GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
	// Stat returns object metadata.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	// Delete removes the object. Deleting a missing object is not an error.
	Delete(ctx context.Context, key string) error
}
