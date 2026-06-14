package mtproto

import (
	"context"
	"sync"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/transport"
)

// bufferedConn wraps a transport.Conn allowing already-read frames to be pushed
// back so the key-exchange flow can re-read the first frame.
type bufferedConn struct {
	conn transport.Conn

	recvMux sync.Mutex
	recv    []bin.Buffer
}

func newBufferedConn(conn transport.Conn) *bufferedConn {
	return &bufferedConn{conn: conn}
}

// Push enqueues a buffer to be returned by the next Recv.
func (c *bufferedConn) Push(b *bin.Buffer) {
	c.recvMux.Lock()
	c.recv = append(c.recv, bin.Buffer{Buf: b.Copy()})
	c.recvMux.Unlock()
}

func (c *bufferedConn) pop() (r bin.Buffer, ok bool) {
	c.recvMux.Lock()
	defer c.recvMux.Unlock()

	if len(c.recv) < 1 {
		return
	}

	r, c.recv = c.recv[len(c.recv)-1], c.recv[:len(c.recv)-1]
	ok = true

	return
}

func (c *bufferedConn) Send(ctx context.Context, b *bin.Buffer) error {
	return c.conn.Send(ctx, b)
}

func (c *bufferedConn) Recv(ctx context.Context, b *bin.Buffer) error {
	if e, ok := c.pop(); ok {
		b.ResetTo(e.Copy())
		return nil
	}

	return c.conn.Recv(ctx, b)
}

func (c *bufferedConn) Close() error {
	return c.conn.Close()
}
