package slowa

import (
	"github.com/gotd/td/tgtest"
	"go.uber.org/ratelimit"
)

func Handler(rps int, next tgtest.Handler) tgtest.Handler {
	limit := ratelimit.New(rps)
	return tgtest.HandlerFunc(func(srv *tgtest.Server, req *tgtest.Request) error {
		limit.Take()
		return next.OnMessage(srv, req)
	})
}
