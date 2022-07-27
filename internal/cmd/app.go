package cmd

import (
	"net"
	"strconv"

	"github.com/gotd/td/tgtest"

	"go.uber.org/zap"
)

// application holds application state.
type application struct {
	lg             *zap.Logger
	Host           string
	Port           int
	PrivateKeyPath string
}

func (a application) Addr() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

func (a application) OnMessage(server *tgtest.Server, req *tgtest.Request) error {
	return nil
}
