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

// TestMessagesSearch verifies in-chat message text search.
func TestMessagesSearch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	env.runClient(ctx, t, &session.StorageMemory{}, func(api *tg.Client) {
		_ = signUp(ctx, t, api, "+15551112233", "Searcher")

		for i, text := range []string{"hello world", "grocery list: milk", "hello again"} {
			_, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer: &tg.InputPeerSelf{}, Message: text, RandomID: int64(100 + i),
			})
			require.NoError(t, err)
		}

		res, err := api.MessagesSearch(ctx, &tg.MessagesSearchRequest{
			Peer: &tg.InputPeerSelf{}, Q: "hello", Filter: &tg.InputMessagesFilterEmpty{}, Limit: 10,
		})
		require.NoError(t, err)

		msgs := res.(*tg.MessagesMessages).Messages
		require.Len(t, msgs, 2, "two messages contain 'hello'")

		res, err = api.MessagesSearch(ctx, &tg.MessagesSearchRequest{
			Peer: &tg.InputPeerSelf{}, Q: "milk", Filter: &tg.InputMessagesFilterEmpty{}, Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, res.(*tg.MessagesMessages).Messages, 1)
	})
}

// TestMessagesSearchGlobal verifies text search across all of the caller's
// dialogs, with offset_id paging.
func TestMessagesSearchGlobal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	var bob *tg.User

	env.runClient(ctx, t, &session.StorageMemory{}, func(api *tg.Client) {
		bob = signUp(ctx, t, api, "+15551113001", "Bob")
	})

	env.runClient(ctx, t, &session.StorageMemory{}, func(api *tg.Client) {
		_ = signUp(ctx, t, api, "+15551113002", "Alice")

		send := func(peer tg.InputPeerClass, text string, randomID int64) {
			_, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer: peer, Message: text, RandomID: randomID,
			})
			require.NoError(t, err)
		}
		send(&tg.InputPeerSelf{}, "note: hello self", 1)
		send(inputPeer(bob), "hello bob", 2)
		send(inputPeer(bob), "unrelated", 3)

		search := func(offsetID, limit int) tg.MessagesMessagesClass {
			res, err := api.MessagesSearchGlobal(ctx, &tg.MessagesSearchGlobalRequest{
				Q: "HELLO", Filter: &tg.InputMessagesFilterEmpty{},
				OffsetPeer: &tg.InputPeerEmpty{}, OffsetID: offsetID, Limit: limit,
			})
			require.NoError(t, err)

			return res
		}

		all, ok := search(0, 10).(*tg.MessagesMessages)
		require.True(t, ok)
		require.Len(t, all.Messages, 2, "both dialogs match")
		require.Equal(t, "hello bob", all.Messages[0].(*tg.Message).Message, "newest first")
		require.Equal(t, "note: hello self", all.Messages[1].(*tg.Message).Message)
		require.Len(t, all.Users, 2, "self and bob")

		first, ok := search(0, 1).(*tg.MessagesMessagesSlice)
		require.True(t, ok)
		require.Equal(t, 2, first.Count)
		require.Len(t, first.Messages, 1)

		next := search(first.Messages[0].(*tg.Message).ID, 1)
		msgs, _ := next.AsModified()
		require.Len(t, msgs.GetMessages(), 1)
		require.Equal(t, "note: hello self", msgs.GetMessages()[0].(*tg.Message).Message)
	})
}

// TestDrafts verifies saveDraft persists, getAllDrafts returns it, getDialogs
// carries it, and a blank draft clears it.
func TestDrafts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	env.runClient(ctx, t, &session.StorageMemory{}, func(api *tg.Client) {
		self := signUp(ctx, t, api, "+15552220001", "Drafter")

		ok, err := api.MessagesSaveDraft(ctx, &tg.MessagesSaveDraftRequest{
			Peer: &tg.InputPeerSelf{}, Message: "remember https://t.me/x",
		})
		require.NoError(t, err)
		require.True(t, ok)

		all, err := api.MessagesGetAllDrafts(ctx)
		require.NoError(t, err)

		ups := all.(*tg.Updates).Updates
		require.Len(t, ups, 1)
		dm := ups[0].(*tg.UpdateDraftMessage).Draft.(*tg.DraftMessage)
		require.Equal(t, "remember https://t.me/x", dm.Message)
		require.NotEmpty(t, dm.Entities) // URL highlighted

		// Dialog carries the draft.
		dlgs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 10, OffsetPeer: &tg.InputPeerEmpty{}})
		require.NoError(t, err)

		var found bool

		for _, dlg := range dlgs.(*tg.MessagesDialogs).Dialogs {
			d := dlg.(*tg.Dialog)
			if d.Peer.(*tg.PeerUser).UserID == self.ID {
				_, ok := d.Draft.(*tg.DraftMessage)
				found = ok
			}
		}

		require.True(t, found, "self dialog should carry the draft")

		// Blank clears it.
		_, err = api.MessagesSaveDraft(ctx, &tg.MessagesSaveDraftRequest{Peer: &tg.InputPeerSelf{}, Message: ""})
		require.NoError(t, err)
		all, err = api.MessagesGetAllDrafts(ctx)
		require.NoError(t, err)
		require.Empty(t, all.(*tg.Updates).Updates)
	})
}
