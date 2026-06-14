package e2e

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"

	"github.com/gotd/teled"
	"github.com/gotd/teled/teledtest"
)

// TestBotFatherNewBot drives the full BotFather /newbot flow over a real client:
// resolve @BotFather, walk the name/username dialog, and confirm the minted
// token both authenticates a bot and is listed by /mybots.
func TestBotFatherNewBot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	srv := teledtest.New(t)

	storage := &session.StorageMemory{}

	var token, botUsername string

	require.NoError(t, srv.Run(ctx, storage, func(api *tg.Client) error {
		signUp(ctx, t, api, "+4444444444", "Dave")

		// BotFather resolves by username and is a bot.
		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: "BotFather"})
		require.NoError(t, err)

		bf := resolved.Users[0].(*tg.User)
		require.Equal(t, teled.BotFatherID, bf.ID)
		require.True(t, bf.Bot)
		require.Equal(t, "BotFather", bf.Username)
		peer := inputPeer(bf)

		var seq int

		say := func(text string) string {
			seq++
			_, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer: peer, Message: text, RandomID: int64(seq),
			})
			require.NoError(t, err)
			hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: peer, Limit: 1})
			require.NoError(t, err)

			msgs := hist.(*tg.MessagesMessages).Messages
			require.NotEmpty(t, msgs)

			return msgs[0].(*tg.Message).Message
		}

		require.Contains(t, say("/newbot"), "choose a name")
		require.Contains(t, say("Dave's Test Bot"), "choose a username")
		require.Contains(t, say("not_ending_in_b0t"), "invalid") // rejected, flow stays open
		done := say("dave_e2e_bot")
		require.Contains(t, done, "Congratulations")

		token = extractToken(t, done)
		require.Contains(t, token, ":")

		botUsername = "dave_e2e_bot"

		// The token prefix is the new bot's user id.
		botID, err := strconv.ParseInt(strings.SplitN(token, ":", 2)[0], 10, 64)
		require.NoError(t, err)
		newBot, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: botUsername})
		require.NoError(t, err)
		require.Equal(t, botID, newBot.Users[0].(*tg.User).ID)
		require.True(t, newBot.Users[0].(*tg.User).Bot)

		// /mybots lists the freshly created bot and its token.
		listed := say("/mybots")
		require.Contains(t, listed, "@dave_e2e_bot")
		require.Contains(t, listed, token)

		return nil
	}))

	// The minted token authenticates as that very bot.
	require.NoError(t, srv.Run(ctx, nil, func(api *tg.Client) error {
		self := importBot(ctx, t, api, token)
		require.True(t, self.Bot)
		require.Equal(t, botUsername, self.Username)

		return nil
	}))
}

// TestBotFatherCancel verifies a /newbot flow can be abandoned with /cancel.
func TestBotFatherCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	srv := teledtest.New(t)

	require.NoError(t, srv.Run(ctx, nil, func(api *tg.Client) error {
		signUp(ctx, t, api, "+5555555555", "Erin")
		resolved, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: "BotFather"})
		require.NoError(t, err)

		peer := inputPeer(resolved.Users[0].(*tg.User))

		var seq int

		say := func(text string) string {
			seq++
			_, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer: peer, Message: text, RandomID: int64(seq),
			})
			require.NoError(t, err)
			hist, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: peer, Limit: 1})
			require.NoError(t, err)

			return hist.(*tg.MessagesMessages).Messages[0].(*tg.Message).Message
		}

		require.Contains(t, say("/newbot"), "choose a name")
		require.Equal(t, "Canceled.", say("/cancel"))
		// After cancel, free text is no longer consumed by the flow.
		require.Contains(t, say("some name"), "I don't understand")
		// No bots were created.
		require.Contains(t, say("/mybots"), "don't have any bots")

		return nil
	}))
}
