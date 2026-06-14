package mtproto

import (
	"context"
	"crypto/rsa"
	"io"
	"net"
	"time"

	"github.com/coder/websocket"
	"github.com/go-faster/errors"

	"github.com/gotd/log"
	"github.com/gotd/td/clock"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/exchange"
	"github.com/gotd/td/mtproto"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/tmap"
	"github.com/gotd/td/transport"
)

// Server is a custom MTProto server.
type Server struct {
	dcID int
	key  exchange.PrivateKey

	cipher crypto.Cipher
	clock  clock.Clock
	msgID  mtproto.MessageIDSource

	readTimeout  time.Duration
	writeTimeout time.Duration

	handler  Handler
	registry *registry

	types *tmap.Map
	log   log.Logger
	obs   observability
}

// NewPrivateKey creates a new private key from an RSA private key.
func NewPrivateKey(k *rsa.PrivateKey) exchange.PrivateKey {
	return exchange.PrivateKey{RSA: k}
}

// NewServer creates a new Server.
func NewServer(key exchange.PrivateKey, handler Handler, opts ServerOptions) *Server {
	opts.setDefaults()

	return &Server{
		dcID:         opts.DC,
		key:          key,
		cipher:       crypto.NewServerCipher(opts.Random),
		clock:        opts.Clock,
		msgID:        opts.MessageID,
		readTimeout:  opts.ReadTimeout,
		writeTimeout: opts.WriteTimeout,
		handler:      handler,
		registry:     newRegistry(opts.Keys),
		types:        opts.Types,
		log:          opts.Logger,
		obs:          newObservability(opts.Providers),
	}
}

// Key returns the public key of this server.
func (s *Server) Key() exchange.PublicKey {
	return s.key.Public()
}

// Serve runs the server loop using the given listener until ctx is canceled or
// the listener is closed.
func (s *Server) Serve(ctx context.Context, l transport.Listener) error {
	log.For(s.log).Info(ctx, "Serving")
	defer log.For(s.log).Info(ctx, "Stopping")

	grp := tdsync.NewCancellableGroup(ctx)
	grp.Go(func(ctx context.Context) error {
		for {
			conn, err := l.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return nil
				}

				return errors.Wrap(err, "accept")
			}

			grp.Go(func(ctx context.Context) error {
				if err := s.serveConn(ctx, conn); err != nil && !isClientGone(err) {
					log.For(s.log).Info(ctx, "Serving handler error", log.Error(err))
				}

				return nil
			})
		}
	})
	grp.Go(func(ctx context.Context) error {
		<-ctx.Done()
		return l.Close()
	})

	return grp.Wait()
}

// isClientGone reports whether err is an expected client disconnect.
func isClientGone(err error) bool {
	var opErr *net.OpError

	switch {
	case errors.Is(err, io.EOF):
		return true
	case errors.As(err, &opErr) && (opErr.Op == "write" || opErr.Op == "read"):
		return true
	}

	return websocket.CloseStatus(err) >= 0
}
