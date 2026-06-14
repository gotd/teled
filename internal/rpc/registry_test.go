package rpc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/teled/internal/mtproto"
)

func TestSessionRegistryTrackUntrack(t *testing.T) {
	r := newSessionRegistry()

	r.track(1, mtproto.Session{ID: 10})
	r.track(1, mtproto.Session{ID: 11})
	require.Len(t, r.get(1), 2)

	// Dropping one leaves the other.
	r.untrack(1, 10)
	got := r.get(1)
	require.Len(t, got, 1)
	require.Equal(t, int64(11), got[0].ID)

	// Dropping the last clears the user entirely.
	r.untrack(1, 11)
	require.Empty(t, r.get(1))

	// Untracking unknown ids is a no-op.
	r.untrack(1, 99)
	r.untrack(2, 99)
	require.Empty(t, r.get(2))
}
