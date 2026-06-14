package rpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/tg"

	"github.com/gotd/teled"
	"github.com/gotd/teled/internal/mtproto"
)

func TestToTGUserStatus(t *testing.T) {
	h := &Handler{sessions: newSessionRegistry()}

	// Self is reported online.
	self := h.tgUser(teled.User{ID: 1, FirstName: "Ada"}, true)
	require.IsType(t, &tg.UserStatusOnline{}, self.Status)

	// A never-seen user is "recently", not UserStatusEmpty (which renders as
	// "last seen a long time ago").
	unseen := h.tgUser(teled.User{ID: 2, FirstName: "Bob"}, false)
	require.IsType(t, &tg.UserStatusRecently{}, unseen.Status)

	// A user active just now is reported online...
	h.sessions.track(3, mtproto.Session{ID: 1})
	online := h.tgUser(teled.User{ID: 3, FirstName: "Cleo"}, false)
	require.IsType(t, &tg.UserStatusOnline{}, online.Status)

	// ...and one last active beyond the window is offline with that timestamp.
	h.sessions.mu.Lock()
	h.sessions.lastSeen[4] = time.Now().Add(-2 * onlineWindow)
	h.sessions.mu.Unlock()
	offline := h.tgUser(teled.User{ID: 4, FirstName: "Dan"}, false)
	off, ok := offline.Status.(*tg.UserStatusOffline)
	require.True(t, ok)
	require.Positive(t, off.WasOnline)
}
