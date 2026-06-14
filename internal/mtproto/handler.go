// Package mtproto implements a custom MTProto server for teled.
//
// It composes the low-level gotd/td primitives (transport, exchange, crypto,
// proto/mt/mtproto) that tgtest.Server is built on, but owns the connection
// lifecycle so that auth keys and sessions can be persisted through a pluggable
// KeyStorage. See docs/architecture.md.
package mtproto

import (
	"context"
	"encoding/hex"

	"github.com/gotd/log"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/crypto"
)

// Session represents a connection session.
type Session struct {
	// ID is a session ID.
	ID int64
	// AuthKey is the attached auth key.
	AuthKey crypto.AuthKey
}

// logAttr returns the session as an inlined log group (matching zap.Inline):
// an empty key inlines session_id and key_id into the parent record.
func (s Session) logAttr() log.Attr {
	return log.Group("",
		log.Int64("session_id", s.ID),
		log.String("key_id", hex.EncodeToString(s.AuthKey.ID[:])),
	)
}

// Request represents an MTProto RPC request.
type Request struct {
	// DC is the DC ID of the server that received the request.
	DC int
	// Session is the user session the request arrived on.
	Session Session
	// MsgID is the message ID of the RPC request.
	MsgID int64
	// Buf contains the RPC request body.
	Buf *bin.Buffer
	// RequestCtx is the request context.
	RequestCtx context.Context
}

// Handler is an RPC request handler.
type Handler interface {
	OnMessage(server *Server, req *Request) error
}

var _ Handler = HandlerFunc(nil)

// HandlerFunc is a functional adapter for Handler.
type HandlerFunc func(server *Server, req *Request) error

// OnMessage implements Handler.
func (h HandlerFunc) OnMessage(server *Server, req *Request) error {
	return h(server, req)
}
