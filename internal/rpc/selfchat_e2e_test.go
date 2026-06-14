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

// TestSelfChat verifies that messages sent to Saved Messages (self) persist and
// come back from history and dialogs, both via InputPeerSelf and the explicit
// self InputPeerUser a client may use.
func TestSelfChat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	storage := &session.StorageMemory{}

	var self *tg.User

	env.runClient(ctx, t, storage, func(api *tg.Client) {
		self = signUp(ctx, t, api, "+15557770001", "Ada")

		// Send to self via InputPeerSelf.
		_, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer: &tg.InputPeerSelf{}, Message: "https://t.me/fzfkek", RandomID: 0x111,
		})
		require.NoError(t, err)

		// Send to self via explicit self InputPeerUser.
		_, err = api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer: inputPeer(self), Message: "second note", RandomID: 0x222,
		})
		require.NoError(t, err)

		// History via InputPeerSelf returns both, newest first.
		hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: &tg.InputPeerSelf{}, Limit: 10})
		require.NoError(t, err)

		msgs := hist.(*tg.MessagesMessages).Messages
		require.Len(t, msgs, 2, "both self messages should persist")
		require.Equal(t, "second note", msgs[0].(*tg.Message).Message)

		// The URL message carries a URL entity so clients highlight the link.
		link := msgs[1].(*tg.Message)
		require.Equal(t, "https://t.me/fzfkek", link.Message)
		require.Len(t, link.Entities, 1)
		require.IsType(t, &tg.MessageEntityURL{}, link.Entities[0])

		// Saved Messages is loaded via getSavedHistory by modern clients.
		saved, err := api.MessagesGetSavedHistory(ctx, &tg.MessagesGetSavedHistoryRequest{
			Peer: &tg.InputPeerSelf{}, Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, saved.(*tg.MessagesMessages).Messages, 2, "saved history should return persisted self messages")

		// History via explicit self peer returns the same.
		hist2, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(self), Limit: 10})
		require.NoError(t, err)
		require.Len(t, hist2.(*tg.MessagesMessages).Messages, 2)

		// The Saved Messages dialog is listed with the self peer.
		dlgs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 10, OffsetPeer: &tg.InputPeerEmpty{}})
		require.NoError(t, err)

		d := dlgs.(*tg.MessagesDialogs)
		require.Len(t, d.Dialogs, 1)
		require.Equal(t, self.ID, d.Dialogs[0].(*tg.Dialog).Peer.(*tg.PeerUser).UserID)
	})
}
