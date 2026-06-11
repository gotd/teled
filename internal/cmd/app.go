package cmd

import (
	"net"
	"strconv"

	"go.uber.org/zap"
)

// application holds command configuration and shared dependencies.
type application struct {
	lg *zap.Logger

	Host           string
	Port           int
	PrivateKeyPath string
	PostgresURI    string
	ObjectStoreDir string
}

// Addr returns the server listen address.
func (a application) Addr() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}
