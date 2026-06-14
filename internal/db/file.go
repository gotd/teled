package db

import (
	"context"
	"crypto/rand"
	"errors"

	gerrors "github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"

	"github.com/gotd/teled"
)

// SaveFile records uploaded media, generating an access hash and file
// reference, and returns the populated File.
func (db *DB) SaveFile(ctx context.Context, f teled.File) (teled.File, error) {
	f.AccessHash = genAccessHash()
	f.FileReference = make([]byte, 16)

	if _, err := rand.Read(f.FileReference); err != nil {
		return teled.File{}, gerrors.Wrap(err, "rand")
	}

	if f.Kind == "" {
		f.Kind = "photo"
	}

	if err := db.pool.QueryRow(ctx,
		`INSERT INTO files (owner_user_id, access_hash, object_key, size, mime, sha256, file_reference, kind)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at`,
		f.OwnerUserID, f.AccessHash, f.ObjectKey, f.Size, f.Mime, f.SHA256, f.FileReference, f.Kind,
	).Scan(&f.ID, &f.CreatedAt); err != nil {
		return teled.File{}, gerrors.Wrap(err, "insert")
	}

	return f, nil
}

// FileByID returns stored media by id.
func (db *DB) FileByID(ctx context.Context, id int64) (teled.File, bool, error) {
	var f teled.File

	err := db.pool.QueryRow(ctx,
		`SELECT id, owner_user_id, access_hash, object_key, size, mime, sha256, file_reference, kind, created_at
		 FROM files WHERE id = $1`, id,
	).Scan(&f.ID, &f.OwnerUserID, &f.AccessHash, &f.ObjectKey, &f.Size, &f.Mime, &f.SHA256, &f.FileReference, &f.Kind, &f.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return teled.File{}, false, nil
	}

	if err != nil {
		return teled.File{}, false, gerrors.Wrap(err, "scan")
	}

	return f, true, nil
}
