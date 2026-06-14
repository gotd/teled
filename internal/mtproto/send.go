package mtproto

import (
	"context"
	"math"

	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/crypto"
	"github.com/gotd/td/mt"
	"github.com/gotd/td/proto"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// Send sends a message to the live connection of session k. The message type t
// must be proto.MessageServerResponse or proto.MessageFromServer.
func (s *Server) Send(ctx context.Context, k Session, t proto.MessageType, message bin.Encoder) error {
	conn, ok := s.registry.getConnection(k.ID)
	if !ok {
		return errors.Errorf("send %T: connection for session %d not found", message, k.ID)
	}

	var b bin.Buffer
	if err := message.Encode(&b); err != nil {
		return errors.Wrap(err, "encode")
	}

	data := crypto.EncryptedMessageData{
		SessionID: k.ID,
		// MTProto message bodies are far below the int32 limit.
		MessageDataLen:         int32(b.Len()), // #nosec G115
		MessageDataWithPadding: b.Copy(),
		MessageID:              s.msgID.New(t),
	}

	if err := s.cipher.Encrypt(k.AuthKey, data, &b); err != nil {
		return errors.Wrap(err, "encrypt")
	}

	ctx, cancel := context.WithTimeout(ctx, s.writeTimeout)
	defer cancel()

	if err := conn.Send(ctx, &b); err != nil {
		return errors.Wrap(err, "send")
	}

	return nil
}

func (s *Server) sendReq(req *Request, t proto.MessageType, encoder bin.Encoder) error {
	return s.Send(req.RequestCtx, req.Session, t, encoder)
}

// SendResult sends an RPC result for req.
func (s *Server) SendResult(req *Request, msg bin.Encoder) error {
	var buf bin.Buffer
	if err := msg.Encode(&buf); err != nil {
		return errors.Wrap(err, "encode result")
	}

	if err := s.sendReq(req, proto.MessageServerResponse, &proto.Result{
		RequestMessageID: req.MsgID,
		Result:           buf.Raw(),
	}); err != nil {
		return errors.Wrapf(err, "send result [%T]", msg)
	}

	return nil
}

// SendErr sends an RPC error result for req.
func (s *Server) SendErr(req *Request, e *tgerr.Error) error {
	return s.SendResult(req, &mt.RPCError{
		ErrorCode:    e.Code,
		ErrorMessage: e.Message,
	})
}

// SendBool sends a bool RPC result for req.
func (s *Server) SendBool(req *Request, r bool) error {
	var msg tg.BoolClass = &tg.BoolTrue{}
	if !r {
		msg = &tg.BoolFalse{}
	}

	return s.SendResult(req, msg)
}

// SendUpdates pushes updates to the live connection of session k.
func (s *Server) SendUpdates(ctx context.Context, k Session, updates ...tg.UpdateClass) error {
	if len(updates) == 0 {
		return nil
	}

	if err := s.Send(ctx, k, proto.MessageFromServer, &tg.Updates{
		Updates: updates,
		Date:    int(s.clock.Now().Unix()),
	}); err != nil {
		return errors.Wrap(err, "send updates")
	}

	return nil
}

// SendAck acknowledges received messages on session k.
func (s *Server) SendAck(ctx context.Context, k Session, ids ...int64) error {
	if err := s.Send(ctx, k, proto.MessageFromServer, &mt.MsgsAck{MsgIDs: ids}); err != nil {
		return errors.Wrap(err, "send ack")
	}

	return nil
}

// SendPong responds to a ping request.
func (s *Server) SendPong(req *Request, pingID int64) error {
	if err := s.sendReq(req, proto.MessageServerResponse, &mt.Pong{
		MsgID:  req.MsgID,
		PingID: pingID,
	}); err != nil {
		return errors.Wrap(err, "send pong")
	}

	return nil
}

// SendEternalSalt sends a salt valid until the maximum possible date.
func (s *Server) SendEternalSalt(req *Request) error {
	return s.SendFutureSalts(req, mt.FutureSalt{
		ValidSince: 1,
		ValidUntil: math.MaxInt32,
		Salt:       10,
	})
}

// SendFutureSalts responds to a get_future_salts request.
func (s *Server) SendFutureSalts(req *Request, salts ...mt.FutureSalt) error {
	if err := s.Send(req.RequestCtx, req.Session, proto.MessageServerResponse, &mt.FutureSalts{
		ReqMsgID: req.MsgID,
		Now:      int(s.clock.Now().Unix()),
		Salts:    salts,
	}); err != nil {
		return errors.Wrap(err, "send future salts")
	}

	return nil
}

// sendSessionCreated notifies the client that a new session was created.
func (s *Server) sendSessionCreated(ctx context.Context, k Session, serverSalt int64) error {
	if err := s.Send(ctx, k, proto.MessageFromServer, &mt.NewSessionCreated{
		FirstMsgID: s.msgID.New(proto.MessageFromClient),
		ServerSalt: serverSalt,
	}); err != nil {
		return errors.Wrap(err, "send new_session_created")
	}

	return nil
}

// ForceDisconnect drops the live connection for session k. The auth key is kept.
func (s *Server) ForceDisconnect(k Session) {
	s.registry.deleteConnection(k.ID)
}
