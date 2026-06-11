package rpc

import (
	"context"

	"github.com/go-faster/errors"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
)

// toTGUser maps a stored user to its tg.User wire form.
func toTGUser(u teled.User, self bool) *tg.User {
	user := &tg.User{
		ID:         u.ID,
		AccessHash: u.AccessHash,
		FirstName:  u.FirstName,
		LastName:   u.LastName,
		Self:       self,
		Photo:      &tg.UserProfilePhotoEmpty{},
		Status:     &tg.UserStatusEmpty{},
	}
	if u.Username != "" {
		user.Username = u.Username
	}
	if u.Phone != "" {
		user.Phone = u.Phone
	}
	user.SetFlags()
	return user
}

// callerUser resolves the user logged in on the requesting session, if any.
func (h *Handler) callerUser(ctx context.Context) (teled.User, bool, error) {
	keyID, ok := callerKeyID(ctx)
	if !ok || h.db == nil {
		return teled.User{}, false, nil
	}
	userID, ok, err := h.db.SessionUserID(ctx, keyID)
	if err != nil || !ok {
		return teled.User{}, false, err
	}
	u, ok, err := h.db.UserByID(ctx, userID)
	if err != nil || !ok {
		return teled.User{}, false, err
	}
	return *u, true, nil
}

// resolvePeerUser resolves an InputPeer to a stored user (DMs only).
func (h *Handler) resolvePeerUser(ctx context.Context, caller teled.User, peer tg.InputPeerClass) (teled.User, error) {
	switch p := peer.(type) {
	case *tg.InputPeerSelf:
		return caller, nil
	case *tg.InputPeerUser:
		u, ok, err := h.db.UserByID(ctx, p.UserID)
		if err != nil {
			return teled.User{}, h.internal("lookup peer", err)
		}
		if !ok || u.AccessHash != p.AccessHash {
			return teled.User{}, tgerr.New(400, "PEER_ID_INVALID")
		}
		return *u, nil
	default:
		return teled.User{}, tgerr.New(400, "PEER_ID_INVALID")
	}
}

// dmMessage builds a viewer-relative tg.Message for a DM.
func dmMessage(m teled.Message) *tg.Message {
	msg := &tg.Message{
		ID:      int(m.LocalID),
		Out:     m.Out,
		FromID:  &tg.PeerUser{UserID: m.FromUserID},
		PeerID:  &tg.PeerUser{UserID: m.PeerUserID},
		Message: m.Text,
		Date:    int(m.Date.Unix()),
	}
	if !m.EditDate.IsZero() {
		msg.EditDate = int(m.EditDate.Unix())
	}
	if m.Media != nil {
		msg.Media = photoMedia(*m.Media)
	}
	msg.SetFlags()
	return msg
}

// bindCaller binds the requesting session's auth key to userID.
func (h *Handler) bindCaller(ctx context.Context, userID int64) error {
	keyID, ok := callerKeyID(ctx)
	if !ok {
		return errors.New("no session on request")
	}
	return h.db.BindSession(ctx, keyID, userID)
}
