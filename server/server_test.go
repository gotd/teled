package server_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/gotd/log/logzap"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"

	"github.com/gotd/teled/memory"
	"github.com/gotd/teled/server"
)

// TestInMemoryServer runs the whole teled stack on the in-memory backends (no
// PostgreSQL, no filesystem), driving it with a real gotd client. It proves the
// memory.DB and memory.ObjectStore are correct drop-ins behind the teled.DB and
// teled.ObjectStore ports the RPC handlers depend on.
func TestInMemoryServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	g := tdsync.NewCancellableGroup(ctx)

	const dc = 2
	lg := logzap.New(zaptest.NewLogger(t))

	rsaKey, err := rsa.GenerateKey(rand.Reader, crypto.RSAKeyBits)
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().(*net.TCPAddr)

	// pool is nil: the server uses in-memory auth keys, the injected in-memory
	// DB, and the in-memory object store.
	srv := server.New(rsaKey, nil, memory.NewObjectStore(), server.Options{
		DC:     dc,
		Host:   addr.IP.String(),
		Port:   addr.Port,
		Logger: lg,
		DB:     memory.NewDB(),
	})
	g.Go(func(ctx context.Context) error { return srv.Serve(ctx, ln) })

	keys := []telegram.PublicKey{srv.Key()}
	runClient := func(storage session.Storage, fn func(api *tg.Client)) {
		client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
			PublicKeys:     keys,
			DC:             dc,
			DCList:         dcs.List{Options: []tg.DCOption{{ID: dc, IPAddress: addr.IP.String(), Port: addr.Port}}},
			Resolver:       dcs.Plain(dcs.PlainOptions{}),
			NoUpdates:      true,
			Logger:         logzap.New(zaptest.NewLogger(t).Named("client")),
			SessionStorage: storage,
			RetryInterval:  100 * time.Millisecond,
		})
		require.NoError(t, client.Run(ctx, func(ctx context.Context) error {
			fn(client.API())
			return nil
		}))
	}

	signUp := func(api *tg.Client, phone, first string) *tg.User {
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
	inputPeer := func(u *tg.User) *tg.InputPeerUser {
		return &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
	}

	storageA, storageB := &session.StorageMemory{}, &session.StorageMemory{}
	var userA, userB *tg.User
	var basePts int

	runClient(storageB, func(api *tg.Client) {
		userB = signUp(api, "+2222222222", "Bob")
		st, err := api.UpdatesGetState(ctx)
		require.NoError(t, err)
		basePts = st.Pts
	})

	runClient(storageA, func(api *tg.Client) {
		userA = signUp(api, "+1111111111", "Alice")
		_, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer: inputPeer(userB), Message: "hello bob", RandomID: 0x5151,
		})
		require.NoError(t, err)

		hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(userB), Limit: 10})
		require.NoError(t, err)
		msgs := hist.(*tg.MessagesMessages).Messages
		require.Len(t, msgs, 1)
		require.True(t, msgs[0].(*tg.Message).Out)
	})

	runClient(storageB, func(api *tg.Client) {
		// Incoming view + unread dialog, then read it.
		hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: inputPeer(userA), Limit: 10})
		require.NoError(t, err)
		m := hist.(*tg.MessagesMessages).Messages[0].(*tg.Message)
		require.False(t, m.Out)
		require.Equal(t, "hello bob", m.Message)

		dlgs, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{Limit: 10, OffsetPeer: &tg.InputPeerEmpty{}})
		require.NoError(t, err)
		require.Equal(t, 1, dlgs.(*tg.MessagesDialogs).Dialogs[0].(*tg.Dialog).UnreadCount)

		// Recover the missed message via getDifference.
		diff, err := api.UpdatesGetDifference(ctx, &tg.UpdatesGetDifferenceRequest{Pts: basePts, Date: 1, Qts: 0})
		require.NoError(t, err)
		require.Len(t, diff.(*tg.UpdatesDifference).NewMessages, 1)
	})

	g.Cancel()
	if err := g.Wait(); err != nil {
		require.ErrorIs(t, err, context.Canceled)
	}
}
