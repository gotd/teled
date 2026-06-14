package rpc

import (
	"sync"

	"github.com/gotd/teled/internal/mtproto"
)

// sessionRegistry maps logged-in users to their live MTProto sessions so the
// server can push updates. It is in-memory and single-instance; sessions are
// (re)registered as authenticated requests arrive.
type sessionRegistry struct {
	mu sync.RWMutex
	m  map[int64]map[int64]mtproto.Session // userID -> sessionID -> session
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{m: map[int64]map[int64]mtproto.Session{}}
}

// track records that userID is reachable on the given session.
func (r *sessionRegistry) track(userID int64, s mtproto.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessions, ok := r.m[userID]
	if !ok {
		sessions = map[int64]mtproto.Session{}
		r.m[userID] = sessions
	}

	sessions[s.ID] = s
}

// get returns the live sessions for userID.
func (r *sessionRegistry) get(userID int64) []mtproto.Session {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sessions := r.m[userID]
	if len(sessions) == 0 {
		return nil
	}

	out := make([]mtproto.Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s)
	}

	return out
}
