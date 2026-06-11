package db

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/crypto"

	"github.com/gotd/teled/internal/mtproto"
)

// KeyStore must satisfy the mtproto.KeyStorage seam.
var _ mtproto.KeyStorage = (*KeyStore)(nil)

func TestKeyStore(t *testing.T) {
	pool := newTestPool(t)
	store := NewKeyStore(pool)
	ctx := context.Background()

	var raw crypto.Key
	_, err := rand.Read(raw[:])
	require.NoError(t, err)
	key := raw.WithID()

	// Absent before save.
	_, ok, err := store.Get(ctx, key.ID)
	require.NoError(t, err)
	require.False(t, ok)

	require.NoError(t, store.Save(ctx, key))
	// Save is idempotent.
	require.NoError(t, store.Save(ctx, key))

	got, ok, err := store.Get(ctx, key.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, key.ID, got.ID)
	require.Equal(t, key.Value, got.Value)
}
