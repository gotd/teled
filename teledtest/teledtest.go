// Package teledtest runs a real teled Telegram server in-process for tests.
//
// New starts a fully-migrated server on a random local port backed by a
// throwaway PostgreSQL container, shut down automatically on test cleanup. It is
// the embeddable counterpart to gotd's tgtest: where tgtest makes you stub each
// RPC by hand, teledtest gives you the actual teled implementation (auth, DMs,
// media, bots, BotFather) to test a client against.
//
//	srv := teledtest.New(t)
//	err := srv.Run(ctx, nil, func(api *tg.Client) error {
//	    _, err := api.HelpGetConfig(ctx)
//	    return err
//	})
package teledtest

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"

	"github.com/gotd/teled/internal/db"
	"github.com/gotd/teled/internal/objstore"
	"github.com/gotd/teled/internal/obs"
	"github.com/gotd/teled/internal/pgtest"
	"github.com/gotd/teled/server"
)

// dc is the datacenter id the in-process server advertises and clients dial.
const dc = 2

// Server is a running in-process teled server with helpers to connect clients.
type Server struct {
	// DC is the datacenter id clients should dial.
	DC int
	// Addr is the server's listen address.
	Addr *net.TCPAddr
	// Keys are the server public keys, for telegram.Options.PublicKeys.
	Keys []telegram.PublicKey

	tb   testing.TB
	pool *pgxpool.Pool
}

// New starts a teled server backed by a throwaway PostgreSQL container and
// returns it. The server, database and connections are torn down on test
// cleanup. The test is skipped on hosts without container support.
func New(tb testing.TB) *Server {
	tb.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	tb.Cleanup(cancel)

	dsn := pgtest.New(tb) // skips the test when containers are unavailable.
	if err := server.Migrate(dsn); err != nil {
		tb.Fatalf("teledtest: migrate: %v", err)
	}
	pool, err := db.Open(ctx, dsn, obs.Providers{})
	if err != nil {
		tb.Fatalf("teledtest: open db: %v", err)
	}
	tb.Cleanup(pool.Close)

	key, err := rsa.GenerateKey(rand.Reader, crypto.RSAKeyBits)
	if err != nil {
		tb.Fatalf("teledtest: generate key: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("teledtest: listen: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	store, err := objstore.NewFS(tb.TempDir(), obs.Providers{})
	if err != nil {
		tb.Fatalf("teledtest: object store: %v", err)
	}

	lg := zaptest.NewLogger(tb, zaptest.Level(zapcore.WarnLevel))
	srv := server.New(key, pool, store, server.Options{
		DC:     dc,
		Host:   addr.IP.String(),
		Port:   addr.Port,
		Logger: lg,
	})

	done := make(chan struct{})
	var serveErr error
	go func() {
		defer close(done)
		serveErr = srv.Serve(ctx, ln)
	}()
	tb.Cleanup(func() {
		cancel()
		<-done
		if serveErr != nil && !errors.Is(serveErr, context.Canceled) {
			tb.Errorf("teledtest: serve: %v", serveErr)
		}
	})

	return &Server{
		DC:   dc,
		Addr: addr,
		Keys: []telegram.PublicKey{srv.Key()},
		tb:   tb,
		pool: pool,
	}
}

// Pool returns the server's PostgreSQL pool, for tests that assert on stored
// state directly.
func (s *Server) Pool() *pgxpool.Pool { return s.pool }

// Client builds a gotd client wired to this server. A nil storage uses a fresh
// in-memory session.
func (s *Server) Client(storage session.Storage) *telegram.Client {
	if storage == nil {
		storage = &session.StorageMemory{}
	}
	return telegram.NewClient(telegram.TestAppID, telegram.TestAppHash, telegram.Options{
		PublicKeys:     s.Keys,
		DC:             s.DC,
		DCList:         dcs.List{Options: []tg.DCOption{{ID: s.DC, IPAddress: s.Addr.IP.String(), Port: s.Addr.Port}}},
		Resolver:       dcs.Plain(dcs.PlainOptions{}),
		NoUpdates:      true,
		Logger:         zaptest.NewLogger(s.tb, zaptest.Level(zapcore.WarnLevel)).Named("client"),
		SessionStorage: storage,
		RetryInterval:  100 * time.Millisecond,
	})
}

// Run connects a client (using storage, or a fresh in-memory session when nil),
// invokes fn with the raw API, and disconnects.
func (s *Server) Run(ctx context.Context, storage session.Storage, fn func(api *tg.Client) error) error {
	client := s.Client(storage)
	return client.Run(ctx, func(ctx context.Context) error {
		return fn(client.API())
	})
}
