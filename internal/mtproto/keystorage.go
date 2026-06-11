package mtproto

import (
	"context"
	"sync"

	"github.com/gotd/td/crypto"
)

// KeyStorage persists MTProto auth keys.
//
// Owning the connection loop lets the server resolve an incoming auth_key_id
// through this seam, so a client can reconnect with an existing key after a
// restart instead of re-running key exchange. The in-memory default keeps keys
// only for the process lifetime; a PostgreSQL-backed implementation is added in
// internal/db (see docs/architecture.md).
type KeyStorage interface {
	// Save persists an auth key. Saving an existing key is not an error.
	Save(ctx context.Context, key crypto.AuthKey) error
	// Get returns the key by its 8-byte ID. The boolean is false when absent.
	Get(ctx context.Context, id [8]byte) (crypto.AuthKey, bool, error)
}

// InMemoryKeys is a KeyStorage backed by a map. Safe for concurrent use.
type InMemoryKeys struct {
	mux  sync.RWMutex
	keys map[[8]byte]crypto.AuthKey
}

// NewInMemoryKeys creates an empty in-memory KeyStorage.
func NewInMemoryKeys() *InMemoryKeys {
	return &InMemoryKeys{keys: map[[8]byte]crypto.AuthKey{}}
}

// Save implements KeyStorage.
func (s *InMemoryKeys) Save(_ context.Context, key crypto.AuthKey) error {
	s.mux.Lock()
	s.keys[key.ID] = key
	s.mux.Unlock()
	return nil
}

// Get implements KeyStorage.
func (s *InMemoryKeys) Get(_ context.Context, id [8]byte) (crypto.AuthKey, bool, error) {
	s.mux.RLock()
	key, ok := s.keys[id]
	s.mux.RUnlock()
	return key, ok, nil
}

// Len returns the number of stored keys.
func (s *InMemoryKeys) Len() int {
	s.mux.RLock()
	n := len(s.keys)
	s.mux.RUnlock()
	return n
}
