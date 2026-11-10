package mtproto

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

func TestUnpackInvoke(t *testing.T) {
	inner := &tg.MessagesSendMessageRequest{
		Peer:     &tg.InputPeerSelf{},
		Message:  "hello",
		RandomID: 42,
	}

	for _, tt := range []struct {
		name string
		req  bin.Encoder
	}{
		{"Plain", inner},
		{"InvokeAfterMsg", &tg.InvokeAfterMsgRequest{MsgID: 1, Query: inner}},
		{"InvokeAfterMsgs", &tg.InvokeAfterMsgsRequest{MsgIDs: []int64{1, 2}, Query: inner}},
		{"Nested", &tg.InvokeWithLayerRequest{
			Layer: tg.Layer,
			Query: &tg.InvokeAfterMsgRequest{MsgID: 1, Query: inner},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var b bin.Buffer
			require.NoError(t, tt.req.Encode(&b))

			var got tg.MessagesSendMessageRequest

			h := UnpackInvoke(HandlerFunc(func(_ *Server, req *Request) error {
				return got.Decode(req.Buf)
			}))
			require.NoError(t, h.OnMessage(nil, &Request{Buf: &b}))
			require.Equal(t, inner.Message, got.Message)
			require.Equal(t, inner.RandomID, got.RandomID)
		})
	}
}
