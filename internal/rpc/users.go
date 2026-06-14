package rpc

import (
	"context"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
)

// usersGetUsers resolves the requested input users. It is also the client's
// self-detection and authorization gate: an unauthenticated InputUserSelf
// yields AUTH_KEY_UNREGISTERED.
func (h *Handler) usersGetUsers(ctx context.Context, ids []tg.InputUserClass) ([]tg.UserClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	caller, loggedIn, err := h.callerUser(ctx)
	if err != nil {
		return nil, h.internal(ctx, "caller", err)
	}

	out := make([]tg.UserClass, 0, len(ids))

	for _, id := range ids {
		switch v := id.(type) {
		case *tg.InputUserSelf:
			if !loggedIn {
				return nil, tgerr.New(401, "AUTH_KEY_UNREGISTERED")
			}

			out = append(out, toTGUser(caller, true))
		case *tg.InputUser:
			u, ok, err := h.db.UserByID(ctx, v.UserID)
			if err != nil {
				return nil, h.internal(ctx, "lookup user", err)
			}

			if !ok || u.AccessHash != v.AccessHash {
				out = append(out, &tg.UserEmpty{ID: v.UserID})
				continue
			}

			out = append(out, toTGUser(*u, loggedIn && u.ID == caller.ID))
		default:
			// InputUserEmpty / unsupported.
			out = append(out, &tg.UserEmpty{})
		}
	}

	return out, nil
}

// usersGetFullUser returns the full profile of a user.
func (h *Handler) usersGetFullUser(ctx context.Context, id tg.InputUserClass) (*tg.UsersUserFull, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	caller, loggedIn, err := h.callerUser(ctx)
	if err != nil {
		return nil, h.internal(ctx, "caller", err)
	}

	var target teled.User

	switch v := id.(type) {
	case *tg.InputUserSelf:
		if !loggedIn {
			return nil, tgerr.New(401, "AUTH_KEY_UNREGISTERED")
		}

		target = caller
	case *tg.InputUser:
		u, ok, err := h.db.UserByID(ctx, v.UserID)
		if err != nil {
			return nil, h.internal(ctx, "lookup user", err)
		}

		if !ok || u.AccessHash != v.AccessHash {
			return nil, tgerr.New(400, "USER_ID_INVALID")
		}

		target = *u
	default:
		return nil, tgerr.New(400, "USER_ID_INVALID")
	}

	self := loggedIn && target.ID == caller.ID
	full := tg.UserFull{
		ID:             target.ID,
		About:          target.About,
		NotifySettings: tg.PeerNotifySettings{},
		Settings:       tg.PeerSettings{},
	}
	full.SetFlags()

	return &tg.UsersUserFull{
		FullUser: full,
		Users:    []tg.UserClass{toTGUser(target, self)},
	}, nil
}
