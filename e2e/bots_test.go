package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled/teledtest"
)

// TestBotImportAuthorizationAndCommands covers the bot lifecycle: token login
// (auto-provisioning then reuse), self-resolution carrying the Bot flag, and
// the set/get/reset bot command round trip.
func TestBotImportAuthorizationAndCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	srv := teledtest.New(t)

	const token = "424242:secret-bot-token"

	storage := &session.StorageMemory{}

	var botID int64

	require.NoError(t, srv.Run(ctx, storage, func(api *tg.Client) error {
		self := importBot(ctx, t, api, token)
		require.True(t, self.Self)
		require.True(t, self.Bot)
		require.NotZero(t, self.ID)
		botID = self.ID

		// users.getUsers(self) preserves the bot flag.
		users, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
		require.NoError(t, err)
		require.Len(t, users, 1)
		require.True(t, users[0].(*tg.User).Bot)

		// No commands published yet.
		cmds, err := api.BotsGetBotCommands(ctx, &tg.BotsGetBotCommandsRequest{Scope: &tg.BotCommandScopeDefault{}})
		require.NoError(t, err)
		require.Empty(t, cmds)

		// Publish, then read back in order.
		want := []tg.BotCommand{
			{Command: "start", Description: "Start the bot"},
			{Command: "help", Description: "Show help"},
		}
		ok, err := api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
			Scope: &tg.BotCommandScopeDefault{}, Commands: want,
		})
		require.NoError(t, err)
		require.True(t, ok)

		cmds, err = api.BotsGetBotCommands(ctx, &tg.BotsGetBotCommandsRequest{Scope: &tg.BotCommandScopeDefault{}})
		require.NoError(t, err)
		require.Equal(t, want, cmds)

		// A different scope is independent and still empty.
		usersScope, err := api.BotsGetBotCommands(ctx, &tg.BotsGetBotCommandsRequest{Scope: &tg.BotCommandScopeUsers{}})
		require.NoError(t, err)
		require.Empty(t, usersScope)

		// Reset clears the default scope.
		ok, err = api.BotsResetBotCommands(ctx, &tg.BotsResetBotCommandsRequest{Scope: &tg.BotCommandScopeDefault{}})
		require.NoError(t, err)
		require.True(t, ok)

		cmds, err = api.BotsGetBotCommands(ctx, &tg.BotsGetBotCommandsRequest{Scope: &tg.BotCommandScopeDefault{}})
		require.NoError(t, err)
		require.Empty(t, cmds)

		return nil
	}))

	// Re-login with the same token reuses the account (no new bot).
	require.NoError(t, srv.Run(ctx, nil, func(api *tg.Client) error {
		self := importBot(ctx, t, api, token)
		require.Equal(t, botID, self.ID)

		return nil
	}))
}

// TestBotImportAuthorizationInvalidToken rejects a token without the
// "<id>:<secret>" shape.
func TestBotImportAuthorizationInvalidToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	srv := teledtest.New(t)

	require.NoError(t, srv.Run(ctx, nil, func(api *tg.Client) error {
		_, err := api.AuthImportBotAuthorization(ctx, &tg.AuthImportBotAuthorizationRequest{
			APIID: telegram.TestAppID, APIHash: telegram.TestAppHash, BotAuthToken: "not-a-valid-token",
		})
		require.True(t, tgerr.Is(err, "ACCESS_TOKEN_INVALID"))

		return nil
	}))
}

// TestBotCommandsRequireBot rejects bot command management from a human account.
func TestBotCommandsRequireBot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	srv := teledtest.New(t)

	require.NoError(t, srv.Run(ctx, nil, func(api *tg.Client) error {
		signUp(ctx, t, api, "+3333333333", "Carol")
		_, err := api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
			Scope:    &tg.BotCommandScopeDefault{},
			Commands: []tg.BotCommand{{Command: "start", Description: "x"}},
		})
		require.True(t, tgerr.Is(err, "USER_BOT_REQUIRED"))

		return nil
	}))
}
