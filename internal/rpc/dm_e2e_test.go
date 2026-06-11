package rpc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/transport"

	"github.com/gotd/teled/internal/db"
	"github.com/gotd/teled/internal/mtproto"
	"github.com/gotd/teled/internal/objstore"
	"github.com/gotd/teled/internal/pgtest"
)

// testEnv is a running server a test client can connect to.
type testEnv struct {
	dc   int
	addr *net.TCPAddr
	key  []telegram.PublicKey
	db   *db.DB
}

func newTestEnv(t *testing.T, ctx context.Context, g *tdsync.CancellableGroup) *testEnv {
	t.Helper()
	const dcID = 2
	log := zaptest.NewLogger(t)

	dsn := pgtest.New(t)
	require.NoError(t, db.Migrate(dsn))
	pool, err := db.Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	database := db.New(pool)

	rsaKey, err := rsa.GenerateKey(rand.Reader, crypto.RSAKeyBits)
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().(*net.TCPAddr)

	store, err := objstore.NewFS(t.TempDir())
	require.NoError(t, err)
	handler := New(log.Named("rpc"), database, store, dcID, addr.IP.String(), addr.Port)
	srv := mtproto.NewServer(mtproto.NewPrivateKey(rsaKey), mtproto.UnpackInvoke(handler), mtproto.ServerOptions{
		DC:     dcID,
		Keys:   db.NewKeyStore(pool),
		Logger: log.Named("server"),
	})
	g.Go(func(ctx context.Context) error { return srv.Serve(ctx, transport.ListenCodec(nil, ln)) })

	return &testEnv{dc: dcID, addr: addr, key: []telegram.PublicKey{srv.Key()}, db: database}
}

// runClient connects a client backed by storage and invokes fn with the API.
func (e *testEnv) runClient(ctx context.Context, t *testing.T, storage session.Storage, fn func(api *tg.Client)) {
	t.Helper()
	client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
		PublicKeys:     e.key,
		DC:             e.dc,
		DCList:         dcs.List{Options: []tg.DCOption{{ID: e.dc, IPAddress: e.addr.IP.String(), Port: e.addr.Port}}},
		Resolver:       dcs.Plain(dcs.PlainOptions{}),
		NoUpdates:      true,
		Logger:         zaptest.NewLogger(t).Named("client"),
		SessionStorage: storage,
		RetryInterval:  100 * time.Millisecond,
	})
	require.NoError(t, client.Run(ctx, func(ctx context.Context) error {
		fn(client.API())
		return nil
	}))
}

func signUp(ctx context.Context, t *testing.T, api *tg.Client, phone, first string) *tg.User {
	t.Helper()
	sent, err := api.AuthSendCode(ctx, &tg.AuthSendCodeRequest{
		PhoneNumber: phone, APIID: telegram.TestAppID, APIHash: telegram.TestAppHash, Settings: tg.CodeSettings{},
	})
	require.NoError(t, err)
	code := sent.(*tg.AuthSentCode)
	authResp, err := api.AuthSignUp(ctx, &tg.AuthSignUpRequest{
		PhoneNumber: phone, PhoneCodeHash: code.PhoneCodeHash, FirstName: first,
	})
	require.NoError(t, err)
	return authResp.(*tg.AuthAuthorization).User.(*tg.User)
}

func inputPeer(u *tg.User) *tg.InputPeerUser {
	return &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
}

// TestDMSendAndHistory verifies the per-account message model: A sends to B and
// each side reads its own local view of the conversation.
func TestDMSendAndHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	storageA := &session.StorageMemory{}
	storageB := &session.StorageMemory{}
	var userA, userB *tg.User

	// B signs up first so A can address it.
	env.runClient(ctx, t, storageB, func(api *tg.Client) {
		userB = signUp(ctx, t, api, "+2222222222", "Bob")
	})

	// A signs up and sends a message to B.
	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		userA = signUp(ctx, t, api, "+1111111111", "Alice")

		const randomID = 0x5151
		updResp, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer: inputPeer(userB), Message: "hello bob", RandomID: randomID,
		})
		require.NoError(t, err)
		upd := updResp.(*tg.Updates)
		require.IsType(t, &tg.UpdateMessageID{}, upd.Updates[0])
		require.Equal(t, int64(randomID), upd.Updates[0].(*tg.UpdateMessageID).RandomID)
		nm := upd.Updates[1].(*tg.UpdateNewMessage).Message.(*tg.Message)
		require.True(t, nm.Out)
		require.Equal(t, "hello bob", nm.Message)

		// A's own history shows the outgoing message.
		hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(userB), Limit: 10})
		require.NoError(t, err)
		msgs := hist.(*tg.MessagesMessages).Messages
		require.Len(t, msgs, 1)
		require.True(t, msgs[0].(*tg.Message).Out)
	})

	// B reads its own (incoming) view, sees a dialog with 1 unread, reads it.
	env.runClient(ctx, t, storageB, func(api *tg.Client) {
		hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(userA), Limit: 10})
		require.NoError(t, err)
		msgs := hist.(*tg.MessagesMessages).Messages
		require.Len(t, msgs, 1)
		m := msgs[0].(*tg.Message)
		require.False(t, m.Out)
		require.Equal(t, "hello bob", m.Message)

		dlgs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 10, OffsetPeer: &tg.InputPeerEmpty{}})
		require.NoError(t, err)
		d := dlgs.(*tg.MessagesDialogs)
		require.Len(t, d.Dialogs, 1)
		require.Equal(t, 1, d.Dialogs[0].(*tg.Dialog).UnreadCount)

		aff, err := api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{Peer: inputPeer(userA), MaxID: m.ID})
		require.NoError(t, err)
		require.Positive(t, aff.Pts)

		dlgs, err = api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 10, OffsetPeer: &tg.InputPeerEmpty{}})
		require.NoError(t, err)
		require.Equal(t, 0, dlgs.(*tg.MessagesDialogs).Dialogs[0].(*tg.Dialog).UnreadCount)
	})

	// A edits then deletes its message.
	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(userB), Limit: 10})
		require.NoError(t, err)
		id := hist.(*tg.MessagesMessages).Messages[0].(*tg.Message).ID

		edited, err := api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer: inputPeer(userB), ID: id, Message: "edited",
		})
		require.NoError(t, err)
		em := edited.(*tg.Updates).Updates[0].(*tg.UpdateEditMessage).Message.(*tg.Message)
		require.Equal(t, "edited", em.Message)

		aff, err := api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{ID: []int{id}})
		require.NoError(t, err)
		require.Equal(t, 1, aff.PtsCount)

		hist, err = api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(userB), Limit: 10})
		require.NoError(t, err)
		require.Empty(t, hist.(*tg.MessagesMessages).Messages)
	})

	g.Cancel()
	if err := g.Wait(); err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}
}

// TestMediaRoundTrip uploads a photo, sends it as a DM, and downloads it back.
func TestMediaRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	storageA, storageB := &session.StorageMemory{}, &session.StorageMemory{}
	var userB *tg.User
	data := bytes.Repeat([]byte("teled-photo-bytes!"), 500) // ~9KB, one part.

	env.runClient(ctx, t, storageB, func(api *tg.Client) { userB = signUp(ctx, t, api, "+14000000001", "Bob") })

	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		_ = signUp(ctx, t, api, "+14000000002", "Alice")

		const fileID = 0xBEEF
		ok, err := api.UploadSaveFilePart(ctx, &tg.UploadSaveFilePartRequest{FileID: fileID, FilePart: 0, Bytes: data})
		require.NoError(t, err)
		require.True(t, ok)

		updResp, err := api.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
			Peer:     inputPeer(userB),
			Media:    &tg.InputMediaUploadedPhoto{File: &tg.InputFile{ID: fileID, Parts: 1, Name: "p.jpg"}},
			RandomID: 42,
		})
		require.NoError(t, err)
		msg := updResp.(*tg.Updates).Updates[1].(*tg.UpdateNewMessage).Message.(*tg.Message)
		photo := msg.Media.(*tg.MessageMediaPhoto).Photo.(*tg.Photo)

		// Download it back via getFile.
		uf, err := api.UploadGetFile(ctx, &tg.UploadGetFileRequest{
			Location: &tg.InputPhotoFileLocation{
				ID: photo.ID, AccessHash: photo.AccessHash, FileReference: photo.FileReference, ThumbSize: "x",
			},
			Offset: 0, Limit: 1024 * 1024,
		})
		require.NoError(t, err)
		require.Equal(t, data, uf.(*tg.UploadFile).Bytes)

		// The media must re-render when the message is fetched from history.
		hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(userB), Limit: 10})
		require.NoError(t, err)
		msgs := hist.(*tg.MessagesMessages).Messages
		require.Len(t, msgs, 1)
		histMsg := msgs[0].(*tg.Message)
		histPhoto := histMsg.Media.(*tg.MessageMediaPhoto).Photo.(*tg.Photo)
		require.Equal(t, photo.ID, histPhoto.ID)
		require.Equal(t, photo.AccessHash, histPhoto.AccessHash)
		require.Equal(t, photo.FileReference, histPhoto.FileReference)
	})

	g.Cancel()
	if err := g.Wait(); err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}
}

// TestUpdatesDifference verifies that messages received while a client is
// offline are recovered via updates.getState + getDifference.
func TestUpdatesDifference(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	storageA, storageB := &session.StorageMemory{}, &session.StorageMemory{}
	var userA, userB *tg.User
	var basePts int

	env.runClient(ctx, t, storageA, func(api *tg.Client) { userA = signUp(ctx, t, api, "+13111111111", "Alice") })

	// B signs up and records its baseline pts, then goes offline.
	env.runClient(ctx, t, storageB, func(api *tg.Client) {
		userB = signUp(ctx, t, api, "+13222222222", "Bob")
		st, err := api.UpdatesGetState(ctx)
		require.NoError(t, err)
		basePts = st.Pts
	})

	// A sends two messages while B is offline.
	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		for i, text := range []string{"one", "two"} {
			_, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer: inputPeer(userB), Message: text, RandomID: int64(100 + i),
			})
			require.NoError(t, err)
		}
	})

	// B reconnects and recovers the missed messages via getDifference.
	env.runClient(ctx, t, storageB, func(api *tg.Client) {
		diff, err := api.UpdatesGetDifference(ctx, &tg.UpdatesGetDifferenceRequest{
			Pts: basePts, Date: 1, Qts: 0,
		})
		require.NoError(t, err)
		d := diff.(*tg.UpdatesDifference)
		require.Len(t, d.NewMessages, 2)
		require.Greater(t, d.State.Pts, basePts)

		// Caught up: a second call returns empty.
		empty, err := api.UpdatesGetDifference(ctx, &tg.UpdatesGetDifferenceRequest{
			Pts: d.State.Pts, Date: 1, Qts: 0,
		})
		require.NoError(t, err)
		require.IsType(t, &tg.UpdatesDifferenceEmpty{}, empty)
		_ = userA
	})

	g.Cancel()
	if err := g.Wait(); err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}
}
