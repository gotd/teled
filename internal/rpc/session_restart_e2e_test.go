package rpc

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
	"github.com/gotd/td/transport"

	"github.com/gotd/teled/internal/db"
	"github.com/gotd/teled/internal/mtproto"
	"github.com/gotd/teled/internal/objstore"
	"github.com/gotd/teled/internal/obs"
	"github.com/gotd/teled/internal/pgtest"
)

// TestSessionSurvivesRestart verifies that a logged-in client stays authorized
// across a server restart: the auth key and its user binding live in Postgres,
// so a fresh process (new in-memory registry, same DB and RSA key) must resolve
// the session without forcing a re-login.
func TestSessionSurvivesRestart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const dcID = 2

	lg := zaptest.NewLogger(t)

	dsn := pgtest.New(t)
	require.NoError(t, db.Migrate(dsn))
	pool, err := db.Open(ctx, dsn, obs.Providers{})
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	database := db.New(pool)

	rsaKey, err := rsa.GenerateKey(rand.Reader, crypto.RSAKeyBits)
	require.NoError(t, err)
	store, err := objstore.NewFS(t.TempDir(), obs.Providers{})
	require.NoError(t, err)

	// boot starts a fresh server instance over the shared pool and RSA key,
	// simulating a process restart. It returns the live address and a stop func.
	boot := func() (*net.TCPAddr, []telegram.PublicKey, func()) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		addr := ln.Addr().(*net.TCPAddr)

		handler := New(logzap.New(lg.Named("rpc")), database, store, dcID, addr.IP.String(), addr.Port, obs.Providers{})
		srv := mtproto.NewServer(mtproto.NewPrivateKey(rsaKey), mtproto.UnpackInvoke(handler), mtproto.ServerOptions{
			DC:     dcID,
			Keys:   db.NewKeyStore(pool),
			Logger: logzap.New(lg.Named("server")),
		})

		srvCtx, srvCancel := context.WithCancel(ctx)
		g := tdsync.NewCancellableGroup(srvCtx)
		g.Go(func(ctx context.Context) error { return srv.Serve(ctx, transport.ListenCodec(nil, ln)) })

		return addr, []telegram.PublicKey{srv.Key()}, func() { srvCancel(); _ = ln.Close() }
	}

	runClient := func(addr *net.TCPAddr, key []telegram.PublicKey, storage session.Storage, fn func(api *tg.Client)) {
		client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
			PublicKeys: key,
			DC:         dcID,
			DCList:     dcs.List{Options: []tg.DCOption{{ID: dcID, IPAddress: addr.IP.String(), Port: addr.Port}}},
			Resolver:   dcs.Plain(dcs.PlainOptions{}),
			NoUpdates:  true,
			// Use temporary auth keys (PFS), as Telegram Desktop does: the temp key
			// rotates and is re-bound to the permanent key on each connection.
			EnablePFS:      true,
			TempKeyTTL:     3600,
			Logger:         logzap.New(zaptest.NewLogger(t).Named("client")),
			SessionStorage: storage,
			RetryInterval:  100 * time.Millisecond,
		})
		require.NoError(t, client.Run(ctx, func(ctx context.Context) error {
			fn(client.API())
			return nil
		}))
	}

	storage := &session.StorageMemory{}

	var userID int64

	// First boot: sign up and capture the account id.
	addr1, key1, stop1 := boot()
	runClient(addr1, key1, storage, func(api *tg.Client) {
		u := signUp(ctx, t, api, "+15551234567", "Ada")
		userID = u.ID
	})
	stop1()

	// Second boot: reconnect with the same stored session. The client must still
	// be authorized — getUsers(self) resolves the original account.
	addr2, key2, stop2 := boot()
	defer stop2()
	runClient(addr2, key2, storage, func(api *tg.Client) {
		users, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
		require.NoError(t, err)
		require.Len(t, users, 1)
		self, ok := users[0].(*tg.User)
		require.True(t, ok, "self should resolve after restart, got %T", users[0])
		require.Equal(t, userID, self.ID)
		require.True(t, self.Self)
	})
}
