package mtproto

import (
	"context"

	"github.com/go-faster/errors"

	"github.com/gotd/log"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/exchange"
	"github.com/gotd/td/proto/codec"
	"github.com/gotd/td/transport"
)

func (s *Server) read(ctx context.Context, conn transport.Conn, b *bin.Buffer) error {
	b.Reset()

	ctx, cancel := context.WithTimeout(ctx, s.readTimeout)
	defer cancel()

	return conn.Recv(ctx, b)
}

func (s *Server) sendProtoError(ctx context.Context, conn transport.Conn, code int32) error {
	var buf bin.Buffer
	buf.PutInt32(-code)

	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()

	if err := conn.Send(ctx, &buf); err != nil {
		return errors.Wrap(err, "send")
	}
	return nil
}

// serveConn serves a single client connection until it is closed.
func (s *Server) serveConn(ctx context.Context, conn transport.Conn) error {
	log.For(s.log).Debug(ctx, "Client connected")
	s.obs.activeConns.Add(ctx, 1)
	defer func() {
		s.obs.activeConns.Add(ctx, -1)
		s.registry.removeConn(conn)
		_ = conn.Close()
		log.For(s.log).Debug(ctx, "Client disconnected")
	}()

	b := new(bin.Buffer)
	for {
		if err := s.read(ctx, conn, b); err != nil {
			return errors.Wrap(err, "read")
		}

		var authKeyID [8]byte
		if err := b.PeekN(authKeyID[:], len(authKeyID)); err != nil {
			return errors.Wrap(err, "peek auth key id")
		}

		// Known auth key (cached or persisted): handle the encrypted RPC.
		if authKeyID != ([8]byte{}) {
			_, ok, err := s.registry.getSession(ctx, authKeyID)
			if err != nil {
				return errors.Wrap(err, "lookup session")
			}
			if ok {
				if err := s.rpcHandle(ctx, conn, b); err != nil {
					return errors.Wrap(err, "handle")
				}
				continue
			}

			// Unknown, non-zero key: ask the client to re-run key exchange.
			if err := s.sendProtoError(ctx, conn, codec.CodeAuthKeyNotFound); err != nil {
				return errors.Wrap(err, "send AuthKeyNotFound")
			}
			continue
		}

		// Zero auth key id: start key exchange.
		log.For(s.log).Debug(ctx, "Starting key exchange")
		c := newBufferedConn(conn)
		c.Push(b)

		key, err := s.exchange(ctx, exchangeConn{Conn: c})
		if err != nil {
			var exchangeErr *exchange.ServerExchangeError
			if errors.As(err, &exchangeErr) {
				if sendErr := s.sendProtoError(ctx, c, exchangeErr.Code); sendErr != nil {
					return errors.Wrapf(sendErr, "send proto error %v", exchangeErr.Code)
				}
				return nil
			}
			return errors.Wrap(err, "key exchange failed")
		}

		if err := s.registry.addSession(ctx, key); err != nil {
			return errors.Wrap(err, "save session")
		}
	}
}
