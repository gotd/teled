package mtproto

import (
	"context"
	"sync"

	"go.uber.org/atomic"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/transport"
)

// connection is a live transport connection plus per-connection state.
type connection struct {
	transport.Conn
	sent atomic.Bool
}

// sentCreated reports whether new_session_created was already sent, marking it
// as sent. It returns the previous value.
func (conn *connection) sentCreated() bool {
	return conn.sent.Swap(true)
}

// registry holds live connections and resolves auth keys via KeyStorage.
//
// Live connections cannot be persisted, so they live only in memory and are
// rebuilt as clients reconnect. Auth keys are durable: lookups consult an
// in-memory cache first, then fall back to KeyStorage.
type registry struct {
	keys KeyStorage

	cacheMux sync.RWMutex
	cache    map[[8]byte]crypto.AuthKey

	connsMux sync.Mutex
	conns    map[int64]*connection
}

func newRegistry(keys KeyStorage) *registry {
	return &registry{
		keys:  keys,
		cache: map[[8]byte]crypto.AuthKey{},
		conns: map[int64]*connection{},
	}
}

// addSession caches and persists an auth key obtained from key exchange.
func (r *registry) addSession(ctx context.Context, key crypto.AuthKey) error {
	r.cacheMux.Lock()
	r.cache[key.ID] = key
	r.cacheMux.Unlock()

	return r.keys.Save(ctx, key)
}

// getSession resolves an auth key by ID, consulting the cache then KeyStorage.
func (r *registry) getSession(ctx context.Context, id [8]byte) (crypto.AuthKey, bool, error) {
	r.cacheMux.RLock()
	key, ok := r.cache[id]
	r.cacheMux.RUnlock()
	if ok {
		return key, true, nil
	}

	key, ok, err := r.keys.Get(ctx, id)
	if err != nil {
		return crypto.AuthKey{}, false, err
	}
	if !ok {
		return crypto.AuthKey{}, false, nil
	}

	r.cacheMux.Lock()
	r.cache[id] = key
	r.cacheMux.Unlock()

	return key, true, nil
}

func (r *registry) createConnection(key int64, conn transport.Conn) *connection {
	r.connsMux.Lock()
	defer r.connsMux.Unlock()

	if v, ok := r.conns[key]; ok {
		return v
	}

	c := &connection{Conn: conn}
	r.conns[key] = c
	return c
}

func (r *registry) getConnection(key int64) (*connection, bool) {
	r.connsMux.Lock()
	conn, ok := r.conns[key]
	r.connsMux.Unlock()
	return conn, ok
}

func (r *registry) deleteConnection(key int64) {
	r.connsMux.Lock()
	if conn := r.conns[key]; conn != nil {
		_ = conn.Close()
	}
	delete(r.conns, key)
	r.connsMux.Unlock()
}

// Close closes all live connections.
func (r *registry) Close() error {
	r.connsMux.Lock()
	for _, conn := range r.conns {
		_ = conn.Close()
	}
	r.connsMux.Unlock()
	return nil
}
