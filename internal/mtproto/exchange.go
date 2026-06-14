package mtproto

import (
	"context"

	"github.com/gotd/log"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/exchange"
	"github.com/gotd/td/transport"
)

// exchange runs the server side of the MTProto key exchange.
func (s *Server) exchange(ctx context.Context, conn transport.Conn) (crypto.AuthKey, error) {
	r, err := exchange.NewExchanger(conn, s.dcID).
		WithClock(s.clock).
		WithLogger(log.Named(s.log, "exchange")).
		WithRand(s.cipher.Rand()).
		Server(s.key).
		Run(ctx)
	if err != nil {
		return crypto.AuthKey{}, err
	}

	return r.Key, nil
}
