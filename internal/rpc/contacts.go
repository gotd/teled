package rpc

import (
	"context"
	"strings"

	"github.com/gotd/td/tg"
)

const contactsSearchDefaultLimit = 20

// contactsSearch resolves a global user search (the client's search box). It
// matches stored users by username prefix or name substring, so accounts like
// the built-in BotFather are discoverable.
func (h *Handler) contactsSearch(ctx context.Context, req *tg.ContactsSearchRequest) (*tg.ContactsFound, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 || limit > contactsSearchDefaultLimit {
		limit = contactsSearchDefaultLimit
	}

	users, err := h.db.SearchUsers(ctx, strings.TrimPrefix(req.Q, "@"), limit)
	if err != nil {
		return nil, h.internal(ctx, "search users", err)
	}

	results := make([]tg.PeerClass, 0, len(users))
	out := make([]tg.UserClass, 0, len(users))

	for i := range users {
		results = append(results, &tg.PeerUser{UserID: users[i].ID})
		out = append(out, h.tgUser(users[i], false))
	}

	return &tg.ContactsFound{Results: results, Users: out}, nil
}
