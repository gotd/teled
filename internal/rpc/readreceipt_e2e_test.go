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

// TestReadReceipt verifies that when the recipient reads a message, the sender
// receives an updateReadHistoryOutbox (the "read" ticks).
func TestReadReceipt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	storageA := &session.StorageMemory{}
	storageB := &session.StorageMemory{}

	var (
		userA, userB *tg.User
		sentID       int
	)

	env.runClient(ctx, t, storageB, func(api *tg.Client) { userB = signUp(ctx, t, api, "+13900000001", "Bob") })

	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		userA = signUp(ctx, t, api, "+13900000002", "Alice")
		upd, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer: inputPeer(userB), Message: "did you read this?", RandomID: 0x9a9a,
		})
		require.NoError(t, err)

		sentID = upd.(*tg.Updates).Updates[1].(*tg.UpdateNewMessage).Message.(*tg.Message).ID
	})

	// B reads A's message.
	env.runClient(ctx, t, storageB, func(api *tg.Client) {
		hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(userA), Limit: 10})
		require.NoError(t, err)

		m := hist.(*tg.MessagesMessages).Messages[0].(*tg.Message)
		_, err = api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{Peer: inputPeer(userA), MaxID: m.ID})
		require.NoError(t, err)
	})

	// A sees the read receipt via getDifference.
	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		diff, err := api.UpdatesGetDifference(ctx, &tg.UpdatesGetDifferenceRequest{Pts: 1, Date: int(time.Now().Unix())})
		require.NoError(t, err)

		d, ok := diff.(*tg.UpdatesDifference)
		require.True(t, ok, "expected difference, got %T", diff)

		var maxID int

		for _, u := range d.OtherUpdates {
			if o, ok := u.(*tg.UpdateReadHistoryOutbox); ok {
				maxID = o.MaxID
				require.Equal(t, userB.ID, o.Peer.(*tg.PeerUser).UserID)
			}
		}

		require.Equal(t, sentID, maxID, "sender should see its message read up to sentID")

		// On reload, getDialogs persists the outbox read pointer, so the read
		// double-check survives a refresh.
		dlgs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 10, OffsetPeer: &tg.InputPeerEmpty{}})
		require.NoError(t, err)

		var found bool

		for _, dlg := range dlgs.(*tg.MessagesDialogs).Dialogs {
			dd := dlg.(*tg.Dialog)
			if dd.Peer.(*tg.PeerUser).UserID == userB.ID {
				require.Equal(t, sentID, dd.ReadOutboxMaxID)

				found = true
			}
		}

		require.True(t, found)
	})
}
