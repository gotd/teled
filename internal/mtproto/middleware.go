package mtproto

import (
	"github.com/go-faster/errors"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

// UnpackInvoke is a Handler middleware that unpacks Invoke*-like wrappers:
//
//	tg.InvokeWithLayerRequest
//	tg.InitConnectionRequest
//	tg.InvokeWithoutUpdatesRequest
//
// so the inner query reaches the next handler.
func UnpackInvoke(next Handler) Handler {
	return HandlerFunc(func(srv *Server, req *Request) error {
		id, err := req.Buf.PeekID()
		if err != nil {
			return err
		}

		var (
			obj peekIDObject
			r   bin.Decoder
		)
		for {
			switch id {
			case tg.InvokeWithLayerRequestTypeID:
				r = &tg.InvokeWithLayerRequest{Query: &obj}
			case tg.InitConnectionRequestTypeID:
				r = &tg.InitConnectionRequest{Query: &obj}
			case tg.InvokeWithoutUpdatesRequestTypeID:
				r = &tg.InvokeWithoutUpdatesRequest{Query: &obj}
			default:
				return next.OnMessage(srv, req)
			}

			if err := r.Decode(req.Buf); err != nil {
				return err
			}
			id = obj.TypeID
		}
	})
}

// peekIDObject is a bin.Decoder that captures the type ID of the wrapped query
// without consuming it.
type peekIDObject struct {
	TypeID uint32
}

func (t *peekIDObject) Decode(b *bin.Buffer) error {
	id, err := b.PeekID()
	if err != nil {
		return errors.Wrap(err, "peek id")
	}
	t.TypeID = id
	return nil
}

func (t *peekIDObject) Encode(*bin.Buffer) error {
	return errors.New("peekIDObject must not be encoded")
}
