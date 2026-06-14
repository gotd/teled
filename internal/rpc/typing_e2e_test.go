package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/tg"
)

// TestSetTyping verifies that messages.setTyping is accepted for a peer and for
// Saved Messages, and rejects an unknown peer.
func TestSetTyping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	storageA, storageB := &session.StorageMemory{}, &session.StorageMemory{}

	var userB *tg.User

	env.runClient(ctx, t, storageB, func(api *tg.Client) { userB = signUp(ctx, t, api, "+15000000001", "Bob") })

	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		_ = signUp(ctx, t, api, "+15000000002", "Alice")

		typing := &tg.SendMessageTypingAction{}

		ok, err := api.MessagesSetTyping(ctx, &tg.MessagesSetTypingRequest{Peer: inputPeer(userB), Action: typing})
		require.NoError(t, err)
		require.True(t, ok)

		// Saved Messages (self) is accepted with no one to notify.
		ok, err = api.MessagesSetTyping(ctx, &tg.MessagesSetTypingRequest{Peer: &tg.InputPeerSelf{}, Action: typing})
		require.NoError(t, err)
		require.True(t, ok)

		// Unknown peer is rejected.
		_, err = api.MessagesSetTyping(ctx, &tg.MessagesSetTypingRequest{
			Peer:   &tg.InputPeerUser{UserID: 999999, AccessHash: 1},
			Action: typing,
		})
		require.Error(t, err)
	})

	g.Cancel()

	if err := g.Wait(); err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}
}
