package teled

import "time"

// File is stored media metadata. The blob lives in the ObjectStore at ObjectKey.
type File struct {
	ID            int64
	OwnerUserID   int64
	AccessHash    int64
	ObjectKey     string
	Size          int64
	Mime          string
	SHA256        []byte
	FileReference []byte
	Kind          string // "photo" or "document"
	CreatedAt     time.Time
}
