// Package rpc implements teled's Telegram RPC handlers over tg.ServerDispatcher.
package rpc

import (
	"context"

	"github.com/go-faster/errors"
	"go.uber.org/zap"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled/internal/db"
	"github.com/gotd/teled/internal/mtproto"
)

// Handler owns the server dispatcher and backs RPCs with storage.
type Handler struct {
	lg   *zap.Logger
	db   *db.DB
	dc   int
	host string
	port int

	dispatcher *tg.ServerDispatcher
}

// New builds a Handler and registers all supported RPCs. database may be nil,
// in which case storage-backed RPCs return an error.
func New(lg *zap.Logger, database *db.DB, dc int, host string, port int) *Handler {
	h := &Handler{lg: lg, db: database, dc: dc, host: host, port: port}
	d := tg.NewServerDispatcher(h.fallback)
	h.register(d)
	h.dispatcher = d
	return h
}

// OnMessage implements mtproto.Handler: it dispatches the decoded request and
// sends the result. A *tgerr.Error returned by a handler is converted to an
// RPC error by the mtproto layer rather than dropping the connection.
func (h *Handler) OnMessage(server *mtproto.Server, req *mtproto.Request) error {
	ctx := context.WithValue(req.RequestCtx, keyReq{}, req)
	ctx = context.WithValue(ctx, keySrv{}, server)

	e, err := h.dispatcher.Handle(ctx, req.Buf)
	if err != nil {
		return errors.Wrap(err, "handle")
	}
	return server.SendResult(req, e)
}

// requireDB returns a NOT_IMPLEMENTED error when no database is configured.
func (h *Handler) requireDB() error {
	if h.db == nil {
		return tgerr.New(500, "NOT_IMPLEMENTED")
	}
	return nil
}

// internal logs an operational error and returns a generic RPC error, so
// internal details are not leaked to clients.
func (h *Handler) internal(op string, err error) error {
	h.lg.Error("RPC internal error", zap.String("op", op), zap.Error(err))
	return tgerr.New(500, "INTERNAL")
}

// fallback answers unregistered RPCs without crashing the server.
func (h *Handler) fallback(ctx context.Context, b *bin.Buffer) (bin.Encoder, error) {
	id, _ := b.PeekID()
	h.lg.Debug("Unhandled RPC", zap.String("type", tg.TypesMap()[id]))
	return nil, tgerr.New(500, "NOT_IMPLEMENTED")
}

type (
	keyReq struct{}
	keySrv struct{}
)

func requestFrom(ctx context.Context) *mtproto.Request {
	r, _ := ctx.Value(keyReq{}).(*mtproto.Request)
	return r
}

// callerKeyID returns the auth-key id of the session that issued the request.
func callerKeyID(ctx context.Context) ([8]byte, bool) {
	r := requestFrom(ctx)
	if r == nil {
		return [8]byte{}, false
	}
	return r.Session.AuthKey.ID, true
}
