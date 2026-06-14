package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/tg"

	"github.com/gotd/teled"
)

// TestContactsSearchFindsBotFather verifies the built-in BotFather account is
// discoverable through contacts.search.
func TestContactsSearchFindsBotFather(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	env.runClient(ctx, t, &session.StorageMemory{}, func(api *tg.Client) {
		_ = signUp(ctx, t, api, "+15558880001", "Searcher")

		found, err := api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: "BotFather", Limit: 10})
		require.NoError(t, err)

		var ids []int64

		for _, u := range found.Users {
			if uu, ok := u.(*tg.User); ok {
				ids = append(ids, uu.ID)
			}
		}

		require.Contains(t, ids, teled.BotFatherID)

		// Also discoverable by a lowercase prefix.
		found, err = api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: "botf", Limit: 10})
		require.NoError(t, err)
		require.NotEmpty(t, found.Results)
	})
}

// TestContactsImportAddDelete covers importing by phone, listing, and deleting.
func TestContactsImportAddDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	storageA := &session.StorageMemory{}
	storageB := &session.StorageMemory{}

	var userB *tg.User

	env.runClient(ctx, t, storageB, func(api *tg.Client) {
		userB = signUp(ctx, t, api, "+19990001111", "Bob")
	})

	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		_ = signUp(ctx, t, api, "+19990002222", "Alice")

		// Import Bob by phone.
		imp, err := api.ContactsImportContacts(ctx, []tg.InputPhoneContact{
			{ClientID: 1, Phone: "+19990001111", FirstName: "Bob", LastName: "B"},
		})
		require.NoError(t, err)
		require.Len(t, imp.Imported, 1)
		require.Equal(t, userB.ID, imp.Imported[0].UserID)

		// Listed in getContacts.
		got, err := api.ContactsGetContacts(ctx, 0)
		require.NoError(t, err)

		cc := got.(*tg.ContactsContacts)
		require.Len(t, cc.Contacts, 1)
		require.Equal(t, userB.ID, cc.Contacts[0].UserID)

		// Delete it.
		_, err = api.ContactsDeleteContacts(ctx, []tg.InputUserClass{
			&tg.InputUser{UserID: userB.ID, AccessHash: userB.AccessHash},
		})
		require.NoError(t, err)

		got, err = api.ContactsGetContacts(ctx, 0)
		require.NoError(t, err)
		require.Empty(t, got.(*tg.ContactsContacts).Contacts)
	})
}
