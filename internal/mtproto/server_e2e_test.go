package mtproto_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"testing"
	"time"

	"github.com/go-faster/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/gotd/log/logzap"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/gotd/td/transport"

	"github.com/gotd/teled/internal/mtproto"
)

// TestServerEndToEnd verifies that a real gotd/td client can complete MTProto
// key exchange against our from-scratch server and round-trip an encrypted RPC
// (help.getConfig). It also exercises the persistent KeyStorage seam: the key
// produced by exchange must land in the store.
func TestServerEndToEnd(t *testing.T) {
	const dcID = 2

	log := zaptest.NewLogger(t)
	defer func() { _ = log.Sync() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rsaKey, err := rsa.GenerateKey(rand.Reader, crypto.RSAKeyBits)
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().(*net.TCPAddr)

	config := &tg.Config{
		ThisDC: dcID,
		DCOptions: []tg.DCOption{{
			ID:        dcID,
			IPAddress: addr.IP.String(),
			Port:      addr.Port,
		}},
	}

	// Minimal dispatcher: answer help.getConfig, reject everything else.
	disp := tg.NewServerDispatcher(func(context.Context, *bin.Buffer) (bin.Encoder, error) {
		return nil, tgerr.New(500, "NOT_IMPLEMENTED")
	})
	disp.OnHelpGetConfig(func(context.Context) (*tg.Config, error) {
		return config, nil
	})

	keys := mtproto.NewInMemoryKeys()
	handler := mtproto.UnpackInvoke(mtproto.HandlerFunc(func(s *mtproto.Server, req *mtproto.Request) error {
		e, err := disp.Handle(req.RequestCtx, req.Buf)
		if err != nil {
			return err
		}
		return s.SendResult(req, e)
	}))

	srv := mtproto.NewServer(mtproto.NewPrivateKey(rsaKey), handler, mtproto.ServerOptions{
		DC:     dcID,
		Keys:   keys,
		Logger: log.Named("server"),
	})

	g := tdsync.NewCancellableGroup(ctx)
	g.Go(func(ctx context.Context) error {
		// Auto-detecting listener: the gotd test client (dcs.Plain) speaks plain
		// intermediate transport. Production (root.go) wraps this in
		// transport.ObfuscatedListener for real Telegram clients; the server
		// logic under test is transport-agnostic.
		return srv.Serve(ctx, transport.ListenCodec(nil, ln))
	})

	g.Go(func(ctx context.Context) error {
		defer g.Cancel()

		client := telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
			PublicKeys:     []telegram.PublicKey{srv.Key()},
			DC:             dcID,
			DCList:         dcs.List{Options: config.DCOptions},
			Resolver:       dcs.Plain(dcs.PlainOptions{}),
			NoUpdates:      true,
			Logger:         logzap.New(log.Named("client")),
			SessionStorage: &session.StorageMemory{},
			RetryInterval:  100 * time.Millisecond,
		})

		return client.Run(ctx, func(ctx context.Context) error {
			// Connecting already required a successful key exchange. Now prove an
			// encrypted RPC round-trips end to end.
			got, err := client.API().HelpGetConfig(ctx)
			if err != nil {
				return errors.Wrap(err, "get config")
			}
			require.Equal(t, dcID, got.ThisDC)
			return nil
		})
	})

	require.NoError(t, g.Wait())

	// The key negotiated during exchange must be persisted in KeyStorage.
	require.NotEmpty(t, keys.Len(), "exchange must persist an auth key")
}
