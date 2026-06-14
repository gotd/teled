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

// TestAuthSignUpAndSelf drives a real gotd client through the M2 auth flow
// (sendCode -> signUp) against the storage-backed handler, then verifies the
// account is persisted and self-resolution works.
func TestAuthSignUpAndSelf(t *testing.T) {
	const (
		dcID  = 2
		phone = "+19998887766"
	)

	log := zaptest.NewLogger(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	dsn := pgtest.New(t)
	require.NoError(t, db.Migrate(dsn))
	pool, err := db.Open(ctx, dsn, obs.Providers{})
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	database := db.New(pool)

	rsaKey, err := rsa.GenerateKey(rand.Reader, crypto.RSAKeyBits)
	require.NoError(t, err)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := ln.Addr().(*net.TCPAddr)

	store, err := objstore.NewFS(t.TempDir(), obs.Providers{})
	require.NoError(t, err)

	handler := New(logzap.New(log.Named("rpc")), database, store, dcID, addr.IP.String(), addr.Port, obs.Providers{})
	srv := mtproto.NewServer(mtproto.NewPrivateKey(rsaKey), mtproto.UnpackInvoke(handler), mtproto.ServerOptions{
		DC:     dcID,
		Keys:   db.NewKeyStore(pool), // Persisted: sessions.key_id FK -> auth_keys.
		Logger: logzap.New(log.Named("server")),
	})

	g := tdsync.NewCancellableGroup(ctx)
	g.Go(func(ctx context.Context) error {
		return srv.Serve(ctx, transport.ListenCodec(nil, ln))
	})

	g.Go(func(ctx context.Context) error {
		defer g.Cancel()

		client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
			PublicKeys:     []telegram.PublicKey{srv.Key()},
			DC:             dcID,
			DCList:         dcs.List{Options: []tg.DCOption{{ID: dcID, IPAddress: addr.IP.String(), Port: addr.Port}}},
			Resolver:       dcs.Plain(dcs.PlainOptions{}),
			NoUpdates:      true,
			Logger:         logzap.New(log.Named("client")),
			SessionStorage: &session.StorageMemory{},
			RetryInterval:  100 * time.Millisecond,
		})

		return client.Run(ctx, func(ctx context.Context) error {
			api := client.API()

			// sendCode.
			sent, err := api.AuthSendCode(ctx, &tg.AuthSendCodeRequest{
				PhoneNumber: phone,
				APIID:       telegram.TestAppID,
				APIHash:     telegram.TestAppHash,
				Settings:    tg.CodeSettings{},
			})
			require.NoError(t, err)

			code, ok := sent.(*tg.AuthSentCode)
			require.True(t, ok)

			// signUp (new account).
			authResp, err := api.AuthSignUp(ctx, &tg.AuthSignUpRequest{
				PhoneNumber:   phone,
				PhoneCodeHash: code.PhoneCodeHash,
				FirstName:     "Ada",
				LastName:      "Lovelace",
			})
			require.NoError(t, err)

			auth, ok := authResp.(*tg.AuthAuthorization)
			require.True(t, ok)
			self, ok := auth.User.(*tg.User)
			require.True(t, ok)
			require.True(t, self.Self)
			// The server normalizes phone numbers to bare digits (no "+"), as
			// real Telegram does, so the stored/returned value drops the "+".
			require.Equal(t, normalizePhone(phone), self.Phone)
			require.Equal(t, "Ada", self.FirstName)

			// users.getUsers(self) now resolves after binding.
			users, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
			require.NoError(t, err)
			require.Len(t, users, 1)
			got, ok := users[0].(*tg.User)
			require.True(t, ok)
			require.Equal(t, self.ID, got.ID)

			return nil
		})
	})

	require.NoError(t, g.Wait())

	// Account persisted under the normalized phone.
	u, ok, err := database.UserByPhone(ctx, normalizePhone(phone))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Ada", u.FirstName)
}
