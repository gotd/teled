package rpc

import (
	"sync"
	"time"

	"github.com/gotd/teled/internal/mtproto"
)

// sessionRegistry maps logged-in users to their live MTProto sessions so the
// server can push updates, and records each user's last-activity time so the
// server can report real presence (online/last seen). It is in-memory and
// single-instance; sessions are (re)registered as authenticated requests arrive.
type sessionRegistry struct {
	mu       sync.RWMutex
	m        map[int64]map[int64]mtproto.Session // userID -> sessionID -> session
	lastSeen map[int64]time.Time                 // userID -> last activity
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{
		m:        map[int64]map[int64]mtproto.Session{},
		lastSeen: map[int64]time.Time{},
	}
}

// track records that userID is reachable on the given session and marks the user
// as just seen.
func (r *sessionRegistry) track(userID int64, s mtproto.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessions, ok := r.m[userID]
	if !ok {
		sessions = map[int64]mtproto.Session{}
		r.m[userID] = sessions
	}

	sessions[s.ID] = s
	r.lastSeen[userID] = time.Now()
}

// lastSeenAt returns when userID was last active, if ever recorded.
func (r *sessionRegistry) lastSeenAt(userID int64) (time.Time, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ts, ok := r.lastSeen[userID]

	return ts, ok
}

// untrack removes a session from a user's tracked set, e.g. after a push fails
// because its connection is gone. The session is re-tracked when the client
// reconnects and issues an authenticated request.
func (r *sessionRegistry) untrack(userID, sessionID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sessions, ok := r.m[userID]
	if !ok {
		return
	}

	delete(sessions, sessionID)

	if len(sessions) == 0 {
		delete(r.m, userID)
	}
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
