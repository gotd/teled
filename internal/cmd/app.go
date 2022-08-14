package cmd

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgtest"

	"go.uber.org/zap"
)

// application holds application state.
type application struct {
	lg *zap.Logger
	d  *tg.ServerDispatcher

	Host           string
	Port           int
	PrivateKeyPath string
}

func (a application) Addr() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

type (
	keyReq struct{}
	keySrv struct{}
)

func (a *application) OnMessage(server *tgtest.Server, req *tgtest.Request) error {
	ctx := context.WithValue(req.RequestCtx, keyReq{}, req)
	ctx = context.WithValue(ctx, keySrv{}, server)
	e, err := a.d.Handle(ctx, req.Buf)
	if err != nil {
		a.lg.Debug("Handle", zap.Error(err))
		return errors.Wrap(err, "handle")
	}

	if err := server.SendResult(req, e); err != nil {
		a.lg.Debug("SendResult", zap.Error(err))
		return errors.Wrap(err, "send result")
	}

	return nil
}

func (a *application) Fallback(ctx context.Context, b *bin.Buffer) (bin.Encoder, error) {
	id, err := b.PeekID()
	if err != nil {
		panic(err)
	}
	v, ok := tg.TypesMap()[id]
	if !ok {
		v = fmt.Sprintf("#%x", id)
	}
	a.lg.Fatal("Unexpected message",
		zap.String("type", v),
	)
	panic("unreachable")
}
