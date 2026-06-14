package rpc

import (
	"context"
	"regexp"

	"github.com/gotd/log"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// usernameRe is a lenient public-username validator: 5-32 characters of
// letters, digits and underscores, starting with a letter. It approximates
// Telegram's rules closely enough for the test server.
var usernameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{4,31}$`)

// accountCheckUsername reports whether username is available for the caller to
// claim: it must be well-formed and either unoccupied or already owned by the
// caller.
func (h *Handler) accountCheckUsername(ctx context.Context, username string) (bool, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return false, err
	}

	if !usernameRe.MatchString(username) {
		return false, tgerr.New(400, "USERNAME_INVALID")
	}

	u, ok, err := h.db.UserByUsername(ctx, username)
	if err != nil {
		return false, h.internal(ctx, "lookup username", err)
	}

	return !ok || u.ID == caller.ID, nil
}

// accountUpdateUsername sets (or, when username is empty, clears) the caller's
// username and returns the updated account.
func (h *Handler) accountUpdateUsername(ctx context.Context, username string) (tg.UserClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if username != "" && !usernameRe.MatchString(username) {
		return nil, tgerr.New(400, "USERNAME_INVALID")
	}

	if username == caller.Username {
		return nil, tgerr.New(400, "USERNAME_NOT_MODIFIED")
	}

	if username != "" {
		if u, ok, err := h.db.UserByUsername(ctx, username); err != nil {
			return nil, h.internal(ctx, "lookup username", err)
		} else if ok && u.ID != caller.ID {
			return nil, tgerr.New(400, "USERNAME_OCCUPIED")
		}
	}

	updated, err := h.db.SetUsername(ctx, caller.ID, username)
	if err != nil {
		return nil, h.internal(ctx, "set username", err)
	}

	log.For(h.lg).Debug(ctx, "account.updateUsername",
		log.Int64("user_id", caller.ID), log.String("username", username))

	return toTGUser(updated, true), nil
}
