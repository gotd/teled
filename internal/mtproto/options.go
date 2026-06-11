package mtproto

import (
	"io"
	"time"

	"go.uber.org/zap"

	"github.com/gotd/td/clock"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/mt"
	"github.com/gotd/td/mtproto"
	"github.com/gotd/td/proto"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tmap"
)

// ServerOptions configures a Server.
type ServerOptions struct {
	// DC is the DC ID of this server. Defaults to 2.
	DC int
	// Random is the random source. Defaults to crypto.DefaultRand.
	Random io.Reader
	// Logger is the zap logger. No logs by default.
	Logger *zap.Logger
	// Keys persists auth keys. Defaults to an in-memory store.
	Keys KeyStorage
	// Clock to use. Defaults to clock.System.
	Clock clock.Clock
	// MessageID generates server message IDs. Defaults to proto.NewMessageIDGen.
	MessageID mtproto.MessageIDSource
	// Types is a type map used in verbose logging of incoming messages.
	Types *tmap.Map
	// ReadTimeout is the connection read timeout.
	ReadTimeout time.Duration
	// WriteTimeout is the connection write timeout.
	WriteTimeout time.Duration
}

func (opt *ServerOptions) setDefaults() {
	if opt.DC == 0 {
		opt.DC = 2
	}
	if opt.Random == nil {
		opt.Random = crypto.DefaultRand()
	}
	if opt.Logger == nil {
		opt.Logger = zap.NewNop()
	}
	if opt.Keys == nil {
		opt.Keys = NewInMemoryKeys()
	}
	if opt.Clock == nil {
		opt.Clock = clock.System
	}
	if opt.MessageID == nil {
		opt.MessageID = proto.NewMessageIDGen(opt.Clock.Now)
	}
	if opt.Types == nil {
		opt.Types = tmap.New(
			tg.TypesMap(),
			mt.TypesMap(),
			proto.TypesMap(),
		)
	}
	if opt.ReadTimeout == 0 {
		opt.ReadTimeout = 30 * time.Second
	}
	if opt.WriteTimeout == 0 {
		opt.WriteTimeout = 30 * time.Second
	}
}
