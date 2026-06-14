// Package rpc implements teled's Telegram RPC handlers over tg.ServerDispatcher.
package rpc

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"

	"github.com/gotd/log"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
	"github.com/gotd/teled/internal/mtproto"
	"github.com/gotd/teled/internal/obs"
)

// Handler owns the server dispatcher and backs RPCs with storage.
type Handler struct {
	lg    log.Logger
	db    teled.DB
	store teled.ObjectStore
	dc    int
	host  string
	port  int

	sessions   *sessionRegistry
	staging    *uploadStaging
	dispatcher *tg.ServerDispatcher

	obs observability
}

// New builds a Handler and registers all supported RPCs. database and store may
// be nil, in which case the corresponding RPCs return an error. providers
// supplies the OpenTelemetry tracer and meter for this layer.
func New(lg log.Logger, database teled.DB, store teled.ObjectStore, dc int, host string, port int, providers obs.Providers) *Handler {
	h := &Handler{
		lg: lg, db: database, store: store, dc: dc, host: host, port: port,
		sessions: newSessionRegistry(), staging: newUploadStaging(),
		obs: newObservability(providers),
	}
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

	// Resolve the RPC method name from the request type id for span naming and
	// metric labeling.
	method := "unknown"
	if id, err := req.Buf.PeekID(); err == nil {
		if name := tg.TypesMap()[id]; name != "" {
			method = name
		}
	}

	ctx, span := h.obs.tracer.Start(ctx, method)
	defer span.End()
	start := time.Now()

	// Register the session for push if it belongs to a logged-in user. Resolve
	// temp (PFS) keys to their permanent key first, since the binding lives there.
	if h.db != nil {
		if keyID, err := h.effectiveKeyID(ctx, req.Session.AuthKey.ID); err == nil {
			if userID, ok, err := h.db.SessionUserID(ctx, keyID); err == nil && ok {
				h.sessions.track(userID, req.Session)
			}
		}
	}

	e, err := h.dispatcher.Handle(ctx, req.Buf)

	status := "ok"
	if err != nil {
		status = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	h.obs.duration.Record(ctx, time.Since(start).Seconds(),
		metric.WithAttributes(attribute.String("rpc.method", method)))
	h.obs.requests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("rpc.method", method),
		attribute.String("rpc.status", status),
	))

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
func (h *Handler) internal(ctx context.Context, op string, err error) error {
	log.For(h.lg).Error(ctx, "RPC internal error", log.String("op", op), log.Error(err))
	return tgerr.New(500, "INTERNAL")
}

// fallback answers unregistered RPCs without crashing the server.
func (h *Handler) fallback(ctx context.Context, b *bin.Buffer) (bin.Encoder, error) {
	id, _ := b.PeekID()
	log.For(h.lg).Debug(ctx, "Unhandled RPC", log.String("type", tg.TypesMap()[id]))
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

func serverFrom(ctx context.Context) *mtproto.Server {
	s, _ := ctx.Value(keySrv{}).(*mtproto.Server)
	return s
}

// callerKeyID returns the auth-key id of the session that issued the request.
func callerKeyID(ctx context.Context) ([8]byte, bool) {
	r := requestFrom(ctx)
	if r == nil {
		return [8]byte{}, false
	}
	return r.Session.AuthKey.ID, true
}

// requireCaller returns the logged-in user for the request, or an
// AUTH_KEY_UNREGISTERED error when the session is not authenticated.
func (h *Handler) requireCaller(ctx context.Context) (teled.User, error) {
	if err := h.requireDB(); err != nil {
		return teled.User{}, err
	}
	u, ok, err := h.callerUser(ctx)
	if err != nil {
		return teled.User{}, h.internal(ctx, "caller", err)
	}
	if !ok {
		return teled.User{}, tgerr.New(401, "AUTH_KEY_UNREGISTERED")
	}
	return u, nil
}
