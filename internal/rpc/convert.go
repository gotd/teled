package rpc

import (
	"context"

	"github.com/go-faster/errors"

	"github.com/gotd/td/tg"

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

// bindCaller binds the requesting session's auth key to userID.
func (h *Handler) bindCaller(ctx context.Context, userID int64) error {
	keyID, ok := callerKeyID(ctx)
	if !ok {
		return errors.New("no session on request")
	}
	return h.db.BindSession(ctx, keyID, userID)
}
