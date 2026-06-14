// Package e2e exercises teled end-to-end through a real gotd client connected
// to an in-process server started with teledtest — which doubles as a worked
// example of embedding the server in tests.
package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

// signUp registers a fresh account via the dev auth flow and returns self.
func signUp(ctx context.Context, t *testing.T, api *tg.Client, phone, first string) *tg.User {
	t.Helper()
	sent, err := api.AuthSendCode(ctx, &tg.AuthSendCodeRequest{
		PhoneNumber: phone, APIID: telegram.TestAppID, APIHash: telegram.TestAppHash, Settings: tg.CodeSettings{},
	})
	require.NoError(t, err)
	code := sent.(*tg.AuthSentCode)
	authResp, err := api.AuthSignUp(ctx, &tg.AuthSignUpRequest{
		PhoneNumber: phone, PhoneCodeHash: code.PhoneCodeHash, FirstName: first,
	})
	require.NoError(t, err)
	return authResp.(*tg.AuthAuthorization).User.(*tg.User)
}

// importBot logs in a bot by token and returns the resolved self user.
func importBot(ctx context.Context, t *testing.T, api *tg.Client, token string) *tg.User {
	t.Helper()
	authResp, err := api.AuthImportBotAuthorization(ctx, &tg.AuthImportBotAuthorizationRequest{
		APIID: telegram.TestAppID, APIHash: telegram.TestAppHash, BotAuthToken: token,
	})
	require.NoError(t, err)
	return authResp.(*tg.AuthAuthorization).User.(*tg.User)
}

func inputPeer(u *tg.User) *tg.InputPeerUser {
	return &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
}

// extractToken pulls the token line out of BotFather's success message.
func extractToken(t *testing.T, success string) string {
	t.Helper()
	lines := strings.Split(success, "\n")
	for i, l := range lines {
		if strings.Contains(l, "Use this token") && i+1 < len(lines) {
			return strings.TrimSpace(lines[i+1])
		}
	}
	t.Fatalf("no token in message: %q", success)
	return ""
}
