// Package server assembles teled's storage, RPC handlers and MTProto transport
// into a single embeddable Telegram server.
//
// It is the same wiring the teled binary uses, exposed as a library so that
// programs and tests can run a real Telegram server in-process. For a turnkey
// test harness (throwaway PostgreSQL, random port, ready-made client) see
// github.com/gotd/teled/teledtest.
package server

import (
	"context"
	"crypto/rsa"
	"net"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/gotd/log"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/transport"

	"github.com/gotd/teled"
	"github.com/gotd/teled/internal/db"
	"github.com/gotd/teled/internal/mtproto"
	"github.com/gotd/teled/internal/obs"
	"github.com/gotd/teled/internal/rpc"
)

// Migrate applies teled's schema migrations to the PostgreSQL database at dsn.
// Call it once before opening a pool for New.
func Migrate(dsn string) error { return db.Migrate(dsn) }

// Options configures a Server. The zero value is valid: DC defaults to 1,
// Logger to a no-op, and telemetry to no-op providers.
type Options struct {
	// DC is this server's datacenter id, advertised in help.getConfig.
	DC int
	// Host and Port are the address advertised to clients in the DC options of
	// help.getConfig. They should match how clients reach the listener.
	Host string
	Port int
	// Logger receives server logs. Defaults to a no-op logger.
	Logger log.Logger
	// DB overrides the storage backend. When nil, New builds the PostgreSQL
	// backend from pool (or runs with no persistence when pool is also nil).
	// Set it to plug in an alternative, e.g. github.com/gotd/teled/memory.
	DB teled.DB
	// Obfuscated wraps the listener in MTProto's obfuscated transport, as the
	// production server does. Leave false to auto-detect the transport codec,
	// which is what in-process test clients use.
	Obfuscated bool
	// TracerProvider and MeterProvider instrument the server. Nil yields no-op.
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
}

// Server is an embeddable teled Telegram server. Construct it with New and run
// it with Serve.
type Server struct {
	srv        *mtproto.Server
	dc         int
	obfuscated bool
}

// New assembles a server from a private key and storage. pool is a PostgreSQL
// pool already migrated with Migrate; pass nil to run with in-memory auth keys
// and no persistence (most RPCs then return NOT_IMPLEMENTED). store backs media
// upload/download and may be nil when media is unused.
func New(key *rsa.PrivateKey, pool *pgxpool.Pool, store teled.ObjectStore, opts Options) *Server {
	if opts.DC == 0 {
		opts.DC = 1
	}
	lg := log.OrNop(opts.Logger)
	providers := obs.Providers{
		TracerProvider: opts.TracerProvider,
		MeterProvider:  opts.MeterProvider,
	}

	database := opts.DB
	var keys mtproto.KeyStorage
	if pool != nil {
		if database == nil {
			database = db.New(pool)
		}
		keys = db.NewKeyStore(pool)
	} else {
		keys = mtproto.NewInMemoryKeys()
	}

	handler := rpc.New(log.Named(lg, "rpc"), database, store, opts.DC, opts.Host, opts.Port, providers)
	srv := mtproto.NewServer(mtproto.NewPrivateKey(key), mtproto.UnpackInvoke(handler), mtproto.ServerOptions{
		DC:        opts.DC,
		Logger:    log.Named(lg, "mtproto"),
		Keys:      keys,
		Providers: providers,
	})
	return &Server{srv: srv, dc: opts.DC, obfuscated: opts.Obfuscated}
}

// Serve accepts and handles MTProto connections on ln until ctx is canceled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	var l transport.Listener
	if s.obfuscated {
		l = transport.Listen(transport.ObfuscatedListener(ln))
	} else {
		l = transport.ListenCodec(nil, ln)
	}
	return s.srv.Serve(ctx, l)
}

// Key returns the server's public key, for clients to pin via
// telegram.Options.PublicKeys.
func (s *Server) Key() telegram.PublicKey { return s.srv.Key() }

// DC returns the server's datacenter id.
func (s *Server) DC() int { return s.dc }
