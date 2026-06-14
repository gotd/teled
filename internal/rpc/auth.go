package rpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// devPhoneCode is the fixed login code issued in lieu of real SMS delivery.
const devPhoneCode = "12345"

// codeTTL bounds how long an issued login code is valid.
const codeTTL = 10 * time.Minute

// testPhoneRe matches Telegram test account numbers of the form 99966XYYYY,
// where X is the datacenter ID (1-3) and Y is any digit. An optional leading
// "+" is tolerated.
var testPhoneRe = regexp.MustCompile(`^\+?99966([1-3])\d{4}$`)

// phoneCode returns the login code that the test server expects for a phone
// number. For Telegram test accounts (99966XYYYY) the code is the datacenter
// digit X repeated five times (e.g. 9996621234 -> "22222"); every other number
// gets the fixed dev code.
func phoneCode(phone string) string {
	if m := testPhoneRe.FindStringSubmatch(phone); m != nil {
		return strings.Repeat(m[1], 5)
	}
	return devPhoneCode
}

// authSendCode issues a login code for the phone number. No SMS is sent; the
// fixed dev code is accepted by signIn/signUp.
func (h *Handler) authSendCode(ctx context.Context, req *tg.AuthSendCodeRequest) (tg.AuthSentCodeClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, h.internal(ctx, "rand", err)
	}
	codeHash := hex.EncodeToString(raw[:])

	code := phoneCode(req.PhoneNumber)
	if err := h.db.SavePhoneCode(ctx, req.PhoneNumber, codeHash, code, codeTTL); err != nil {
		return nil, h.internal(ctx, "save code", err)
	}

	sent := &tg.AuthSentCode{
		Type:          &tg.AuthSentCodeTypeSMS{Length: len(code)},
		PhoneCodeHash: codeHash,
		Timeout:       int(codeTTL.Seconds()),
	}
	sent.SetFlags()
	return sent, nil
}

// checkCode validates the (phone, hash, code) triple, returning a typed RPC
// error on mismatch.
func (h *Handler) checkCode(ctx context.Context, phone, codeHash, code string) error {
	want, ok, err := h.db.PhoneCode(ctx, phone, codeHash)
	if err != nil {
		return h.internal(ctx, "get code", err)
	}
	if !ok {
		return tgerr.New(400, "PHONE_CODE_EXPIRED")
	}
	if code != want {
		return tgerr.New(400, "PHONE_CODE_INVALID")
	}
	return nil
}

// authSignIn signs in an existing account, or signals that signup is required.
func (h *Handler) authSignIn(ctx context.Context, req *tg.AuthSignInRequest) (tg.AuthAuthorizationClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}
	if err := h.checkCode(ctx, req.PhoneNumber, req.PhoneCodeHash, req.PhoneCode); err != nil {
		return nil, err
	}

	u, ok, err := h.db.UserByPhone(ctx, req.PhoneNumber)
	if err != nil {
		return nil, h.internal(ctx, "lookup user", err)
	}
	if !ok {
		return &tg.AuthAuthorizationSignUpRequired{}, nil
	}

	if err := h.bindCaller(ctx, u.ID); err != nil {
		return nil, h.internal(ctx, "bind session", err)
	}
	return &tg.AuthAuthorization{User: toTGUser(*u, true)}, nil
}

// authSignUp creates a new account for an unoccupied phone number.
func (h *Handler) authSignUp(ctx context.Context, req *tg.AuthSignUpRequest) (tg.AuthAuthorizationClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}
	if err := h.checkCode(ctx, req.PhoneNumber, req.PhoneCodeHash, phoneCode(req.PhoneNumber)); err != nil {
		return nil, err
	}

	if _, ok, err := h.db.UserByPhone(ctx, req.PhoneNumber); err != nil {
		return nil, h.internal(ctx, "lookup user", err)
	} else if ok {
		return nil, tgerr.New(400, "PHONE_NUMBER_OCCUPIED")
	}

	u, err := h.db.CreateUser(ctx, req.PhoneNumber, req.FirstName, req.LastName)
	if err != nil {
		return nil, h.internal(ctx, "create user", err)
	}
	if err := h.bindCaller(ctx, u.ID); err != nil {
		return nil, h.internal(ctx, "bind session", err)
	}
	return &tg.AuthAuthorization{User: toTGUser(u, true)}, nil
}

// authImportBotAuthorization logs in a bot by token. Tokens are not minted by a
// BotFather here: the first login with a well-formed token auto-provisions the
// bot account, and subsequent logins reuse it. This mirrors how authSignUp
// auto-creates user accounts for the test server.
func (h *Handler) authImportBotAuthorization(
	ctx context.Context, req *tg.AuthImportBotAuthorizationRequest,
) (tg.AuthAuthorizationClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	// A valid bot token is "<bot_id>:<secret>"; reject anything else.
	id, secret, ok := strings.Cut(req.BotAuthToken, ":")
	if !ok || id == "" || secret == "" {
		return nil, tgerr.New(400, "ACCESS_TOKEN_INVALID")
	}

	bot, found, err := h.db.BotByToken(ctx, req.BotAuthToken)
	if err != nil {
		return nil, h.internal(ctx, "lookup bot", err)
	}
	if !found {
		created, err := h.db.CreateBot(ctx, req.BotAuthToken, "", "Bot "+id)
		if err != nil {
			return nil, h.internal(ctx, "create bot", err)
		}
		bot = &created
	}

	if err := h.bindCaller(ctx, bot.ID); err != nil {
		return nil, h.internal(ctx, "bind session", err)
	}
	return &tg.AuthAuthorization{User: toTGUser(*bot, true)}, nil
}

// authLogOut unbinds the session's user.
func (h *Handler) authLogOut(ctx context.Context) (*tg.AuthLoggedOut, error) {
	if h.db != nil {
		if keyID, ok := callerKeyID(ctx); ok {
			if err := h.db.Unbind(ctx, keyID); err != nil {
				return nil, h.internal(ctx, "unbind", err)
			}
		}
	}
	return &tg.AuthLoggedOut{}, nil
}
