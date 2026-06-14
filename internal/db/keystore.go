package db

import (
	"context"
	"errors"

	gerrors "github.com/go-faster/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gotd/td/crypto"
)

// KeyStore persists MTProto auth keys, implementing mtproto.KeyStorage.
type KeyStore struct {
	pool *pgxpool.Pool
}

// NewKeyStore creates a KeyStore over the given pool.
func NewKeyStore(pool *pgxpool.Pool) *KeyStore {
	return &KeyStore{pool: pool}
}

// Save persists an auth key. Saving an existing key is a no-op.
func (s *KeyStore) Save(ctx context.Context, key crypto.AuthKey) error {
	q := psql.Insert("auth_keys").
		Columns("key_id", "auth_key").
		Values(key.ID[:], key.Value[:]).
		Suffix("ON CONFLICT (key_id) DO NOTHING")

	sql, args, err := q.ToSql()
	if err != nil {
		return gerrors.Wrap(err, "build query")
	}

	if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
		return gerrors.Wrap(err, "exec")
	}

	return nil
}

// Get returns the key by its 8-byte ID. The boolean is false when absent.
func (s *KeyStore) Get(ctx context.Context, id [8]byte) (crypto.AuthKey, bool, error) {
	q := psql.Select("auth_key").From("auth_keys").Where("key_id = ?", id[:])

	sql, args, err := q.ToSql()
	if err != nil {
		return crypto.AuthKey{}, false, gerrors.Wrap(err, "build query")
	}

	var value []byte
	if err := s.pool.QueryRow(ctx, sql, args...).Scan(&value); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return crypto.AuthKey{}, false, nil
		}

		return crypto.AuthKey{}, false, gerrors.Wrap(err, "scan")
	}

	var k crypto.Key

	copy(k[:], value)
	// WithID recomputes the key ID, which must match the stored key_id.
	return k.WithID(), true, nil
}
