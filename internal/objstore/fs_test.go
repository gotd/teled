package objstore_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/teled"
	"github.com/gotd/teled/internal/objstore"
	"github.com/gotd/teled/internal/obs"
)

func TestFS(t *testing.T) {
	ctx := context.Background()
	fs, err := objstore.NewFS(t.TempDir(), obs.Providers{})
	require.NoError(t, err)

	const key = "abcdef0123456789"
	data := []byte(strings.Repeat("teled-media-", 1000))

	require.NoError(t, fs.Put(ctx, key, bytes.NewReader(data), int64(len(data)), teled.PutOptions{}))

	// Stat.
	info, err := fs.Stat(ctx, key)
	require.NoError(t, err)
	require.Equal(t, int64(len(data)), info.Size)

	// Full Get.
	rc, err := fs.Get(ctx, key)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.Equal(t, data, got)

	// Ranged Get, as used by upload.getFile.
	rc, err = fs.GetRange(ctx, key, 12, 24)
	require.NoError(t, err)
	chunk, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.Equal(t, data[12:36], chunk)

	// Range past EOF yields a short read (the getFile EOF signal).
	rc, err = fs.GetRange(ctx, key, int64(len(data))-5, 512)
	require.NoError(t, err)
	tail, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())
	require.Len(t, tail, 5)

	// Delete; missing delete is not an error.
	require.NoError(t, fs.Delete(ctx, key))
	require.NoError(t, fs.Delete(ctx, key))
	_, err = fs.Get(ctx, key)
	require.Error(t, err)
}
