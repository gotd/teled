package rpc

import (
	"context"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
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

// resolveInputUser maps an InputUser to a stored user, validating the access
// hash.
func (h *Handler) resolveInputUser(ctx context.Context, caller teled.User, in tg.InputUserClass) (teled.User, error) {
	switch v := in.(type) {
	case *tg.InputUserSelf:
		return caller, nil
	case *tg.InputUser:
		u, ok, err := h.db.UserByID(ctx, v.UserID)
		if err != nil {
			return teled.User{}, h.internal(ctx, "lookup user", err)
		}

		if !ok || u.AccessHash != v.AccessHash {
			return teled.User{}, tgerr.New(400, "USER_ID_INVALID")
		}

		return *u, nil
	default:
		return teled.User{}, tgerr.New(400, "USER_ID_INVALID")
	}
}

// contactsGetContacts returns the caller's saved contact list.
func (h *Handler) contactsGetContacts(ctx context.Context, _ int64) (tg.ContactsContactsClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	contacts, err := h.db.Contacts(ctx, caller.ID)
	if err != nil {
		return nil, h.internal(ctx, "get contacts", err)
	}

	tgContacts := make([]tg.Contact, 0, len(contacts))
	users := make([]tg.UserClass, 0, len(contacts))

	for _, c := range contacts {
		u, ok, err := h.db.UserByID(ctx, c.UserID)
		if err != nil {
			return nil, h.internal(ctx, "contact user", err)
		}

		if !ok {
			continue
		}

		tgContacts = append(tgContacts, tg.Contact{UserID: c.UserID})
		users = append(users, h.tgUser(*u, false))
	}

	return &tg.ContactsContacts{Contacts: tgContacts, SavedCount: len(tgContacts), Users: users}, nil
}

// contactsImportContacts matches phone contacts to existing accounts and adds
// the matches to the caller's contact list.
func (h *Handler) contactsImportContacts(ctx context.Context, in []tg.InputPhoneContact) (*tg.ContactsImportedContacts, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	var (
		imported []tg.ImportedContact
		users    []tg.UserClass
		seen     = map[int64]bool{}
	)

	for _, c := range in {
		u, ok, err := h.db.UserByPhone(ctx, normalizePhone(c.Phone))
		if err != nil {
			return nil, h.internal(ctx, "lookup contact", err)
		}

		if !ok || u.ID == caller.ID {
			continue
		}

		if err := h.db.AddContact(ctx, caller.ID, u.ID, c.FirstName, c.LastName); err != nil {
			return nil, h.internal(ctx, "add contact", err)
		}

		imported = append(imported, tg.ImportedContact{UserID: u.ID, ClientID: c.ClientID})

		if !seen[u.ID] {
			users = append(users, h.tgUser(*u, false))
			seen[u.ID] = true
		}
	}

	return &tg.ContactsImportedContacts{Imported: imported, Users: users}, nil
}

// contactsAddContact adds a known user to the caller's contact list.
func (h *Handler) contactsAddContact(ctx context.Context, req *tg.ContactsAddContactRequest) (tg.UpdatesClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	target, err := h.resolveInputUser(ctx, caller, req.ID)
	if err != nil {
		return nil, err
	}

	if err := h.db.AddContact(ctx, caller.ID, target.ID, req.FirstName, req.LastName); err != nil {
		return nil, h.internal(ctx, "add contact", err)
	}

	return &tg.Updates{Users: []tg.UserClass{h.tgUser(target, false)}, Date: int(time.Now().Unix())}, nil
}

// contactsDeleteContacts removes the given users from the caller's contact list.
func (h *Handler) contactsDeleteContacts(ctx context.Context, ids []tg.InputUserClass) (tg.UpdatesClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	var (
		contactIDs []int64
		users      []tg.UserClass
	)

	for _, in := range ids {
		u, err := h.resolveInputUser(ctx, caller, in)
		if err != nil {
			continue
		}

		contactIDs = append(contactIDs, u.ID)
		users = append(users, h.tgUser(u, false))
	}

	if err := h.db.DeleteContacts(ctx, caller.ID, contactIDs); err != nil {
		return nil, h.internal(ctx, "delete contacts", err)
	}

	return &tg.Updates{Users: users, Date: int(time.Now().Unix())}, nil
}
