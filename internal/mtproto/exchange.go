package mtproto

import (
	"context"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/exchange"
	"github.com/gotd/td/proto/codec"
	"github.com/gotd/td/transport"
)

// exchangeConn rejects frames carrying a non-zero auth key id during the key
// exchange flow, which expects only unencrypted messages.
type exchangeConn struct {
	transport.Conn
}

func (e exchangeConn) Recv(ctx context.Context, b *bin.Buffer) error {
	for {
		if err := e.Conn.Recv(ctx, b); err != nil {
			return err
		}

		var authKeyID [8]byte
		if err := b.PeekN(authKeyID[:], len(authKeyID)); err != nil {
			return errors.Wrap(err, "peek auth key id")
		}
		if authKeyID != ([8]byte{}) {
			var buf bin.Buffer
			buf.PutInt32(-codec.CodeAuthKeyNotFound)
			if err := e.Send(ctx, &buf); err != nil {
				return errors.Wrap(err, "send")
			}
			continue
		}

		return nil
	}
}

// exchange runs the server side of the MTProto key exchange.
func (s *Server) exchange(ctx context.Context, conn transport.Conn) (crypto.AuthKey, error) {
	r, err := exchange.NewExchanger(conn, s.dcID).
		WithClock(s.clock).
		WithLogger(s.log.Named("exchange")).
		WithRand(s.cipher.Rand()).
		Server(s.key).
		Run(ctx)
	if err != nil {
		return crypto.AuthKey{}, err
	}

	return r.Key, nil
}
