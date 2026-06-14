package rpc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/tg"
)

// TestAccountUsername covers check/update username and resolution: a name is
// free until claimed, claiming it updates the account and makes it resolvable,
// and a second user cannot take it.
func TestAccountUsername(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	storageA := &session.StorageMemory{}
	storageB := &session.StorageMemory{}

	var userA *tg.User

	env.runClient(ctx, t, storageA, func(api *tg.Client) {
		userA = signUp(ctx, t, api, "+15550000001", "Alice")

		// Available before anyone claims it.
		free, err := api.AccountCheckUsername(ctx, "alice")
		require.NoError(t, err)
		require.True(t, free)

		// Malformed usernames are rejected.
		_, err = api.AccountCheckUsername(ctx, "no")
		require.Error(t, err)

		// Claim it; the returned account reflects the new username.
		upd, err := api.AccountUpdateUsername(ctx, "alice")
		require.NoError(t, err)
		require.Equal(t, "alice", upd.(*tg.User).Username)

		// Re-setting the same username is a no-op error, mirroring Telegram.
		_, err = api.AccountUpdateUsername(ctx, "alice")
		require.Error(t, err)

		// The caller may still "check" their own username as available.
		mine, err := api.AccountCheckUsername(ctx, "alice")
		require.NoError(t, err)
		require.True(t, mine)
	})

	// A second account cannot take Alice's username and resolves to her.
	env.runClient(ctx, t, storageB, func(api *tg.Client) {
		_ = signUp(ctx, t, api, "+15550000002", "Bob")

		free, err := api.AccountCheckUsername(ctx, "alice")
		require.NoError(t, err)
		require.False(t, free)

		_, err = api.AccountUpdateUsername(ctx, "alice")
		require.Error(t, err)

		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: "alice"})
		require.NoError(t, err)
		require.Equal(t, userA.ID, resolved.Peer.(*tg.PeerUser).UserID)
	})
}

// TestAccountUpdateProfile verifies profile edits persist and are reflected by
// getFullUser.
func TestAccountUpdateProfile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	g := tdsync.NewCancellableGroup(ctx)
	env := newTestEnv(t, ctx, g)

	env.runClient(ctx, t, &session.StorageMemory{}, func(api *tg.Client) {
		self := signUp(ctx, t, api, "+15559990001", "Grace")

		updated, err := api.AccountUpdateProfile(ctx, &tg.AccountUpdateProfileRequest{
			FirstName: "Grace", LastName: "Hopper", About: "compiler pioneer",
		})
		require.NoError(t, err)
		require.Equal(t, "Hopper", updated.(*tg.User).LastName)

		// Persisted: a fresh full-user fetch reflects the new name and about.
		full, err := api.UsersGetFullUser(ctx, &tg.InputUserSelf{})
		require.NoError(t, err)
		require.Equal(t, "compiler pioneer", full.FullUser.About)
		require.Equal(t, "Hopper", full.Users[0].(*tg.User).LastName)
		require.Equal(t, self.ID, full.Users[0].(*tg.User).ID)
	})
}
