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
