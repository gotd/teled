package rpc

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"github.com/gotd/log"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// devPhoneCode is the fixed login code issued in lieu of real SMS delivery.
const devPhoneCode = "12345"

// codeTTL bounds how long an issued login code is valid.
const codeTTL = 10 * time.Minute

// testPhoneRe matches Telegram test account numbers of the form 99966XYYYY,
// where X is the datacenter ID (1-9) and Y is any digit. An optional leading
// "+" is tolerated.
var testPhoneRe = regexp.MustCompile(`^\+?99966([1-9])\d{4}$`)

// normalizePhone reduces a phone number to its canonical digits-only form.
// Clients may submit the same number formatted differently across the login
// flow — e.g. the display string "+ 9 996610000" to auth.sendCode but the bare
// "9996610000" to auth.signIn. Keying code storage and user lookup on the
// normalized value makes those map to a single number, and lets test-account
// detection (testPhoneRe) see the raw digits.
func normalizePhone(phone string) string {
	var b strings.Builder

	b.Grow(len(phone))

	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}

	return b.String()
}

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

// authKeyTag returns a short hex identifier for the calling MTProto session's
// auth key, so login RPCs that span multiple requests (sendCode → signIn) can
// be correlated and a cross-session/cross-process mismatch made visible.
func authKeyTag(ctx context.Context) string {
	id, ok := callerKeyID(ctx)
	if !ok {
		return ""
	}

	return hex.EncodeToString(id[:])
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

	phone := normalizePhone(req.PhoneNumber)
	code := phoneCode(phone)
	isTest := testPhoneRe.MatchString(phone)

	if err := h.db.SavePhoneCode(ctx, phone, codeHash, code, codeTTL); err != nil {
		return nil, h.internal(ctx, "save code", err)
	}

	log.For(h.lg).Debug(ctx, "auth.sendCode issued login code",
		log.String("phone", phone),
		log.String("phone_raw", req.PhoneNumber),
		log.String("code_hash", codeHash),
		log.String("code", code),
		log.Bool("test_account", isTest),
		log.Int("code_len", len(code)),
		log.Duration("ttl", codeTTL),
		log.String("auth_key", authKeyTag(ctx)),
	)

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
	lg := log.For(h.lg)

	want, ok, err := h.db.PhoneCode(ctx, phone, codeHash)
	if err != nil {
		return h.internal(ctx, "get code", err)
	}

	lg.Debug(ctx, "auth.checkCode lookup",
		log.String("phone", phone),
		log.String("code_hash", codeHash),
		log.String("got_code", code),
		log.String("want_code", want),
		log.Bool("found", ok),
		log.String("auth_key", authKeyTag(ctx)),
	)

	if !ok {
		// No unexpired (phone, code_hash) row. Either the code TTL elapsed, or
		// the phone / code_hash sent at sign-in differs from the one used at
		// sendCode (e.g. a stray "+" prefix or normalization mismatch).
		lg.Warn(ctx, "auth.checkCode rejected: no matching code (PHONE_CODE_EXPIRED)",
			log.String("phone", phone),
			log.String("code_hash", codeHash),
		)

		return tgerr.New(400, "PHONE_CODE_EXPIRED")
	}

	if code != want {
		lg.Warn(ctx, "auth.checkCode rejected: wrong code (PHONE_CODE_INVALID)",
			log.String("phone", phone),
			log.String("got_code", code),
			log.String("want_code", want),
		)

		return tgerr.New(400, "PHONE_CODE_INVALID")
	}

	return nil
}

// authSignIn signs in an existing account, or signals that signup is required.
func (h *Handler) authSignIn(ctx context.Context, req *tg.AuthSignInRequest) (tg.AuthAuthorizationClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	phone := normalizePhone(req.PhoneNumber)
	log.For(h.lg).Debug(ctx, "auth.signIn request",
		log.String("phone", phone),
		log.String("phone_raw", req.PhoneNumber),
		log.String("code_hash", req.PhoneCodeHash),
		log.String("code", req.PhoneCode),
	)

	if err := h.checkCode(ctx, phone, req.PhoneCodeHash, req.PhoneCode); err != nil {
		return nil, err
	}

	u, ok, err := h.db.UserByPhone(ctx, phone)
	if err != nil {
		return nil, h.internal(ctx, "lookup user", err)
	}

	if !ok {
		log.For(h.lg).Debug(ctx, "auth.signIn: no account, signup required",
			log.String("phone", phone))

		return &tg.AuthAuthorizationSignUpRequired{}, nil
	}

	if err := h.bindCaller(ctx, u.ID); err != nil {
		return nil, h.internal(ctx, "bind session", err)
	}

	log.For(h.lg).Debug(ctx, "auth.signIn succeeded",
		log.String("phone", phone), log.Int64("user_id", u.ID))

	return &tg.AuthAuthorization{User: toTGUser(*u, true)}, nil
}

// authSignUp creates a new account for an unoccupied phone number.
func (h *Handler) authSignUp(ctx context.Context, req *tg.AuthSignUpRequest) (tg.AuthAuthorizationClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	phone := normalizePhone(req.PhoneNumber)
	// signUp does not carry the entered code; it must match the one issued at
	// sendCode, which for the test server is deterministic from the phone.
	log.For(h.lg).Debug(ctx, "auth.signUp request",
		log.String("phone", phone),
		log.String("phone_raw", req.PhoneNumber),
		log.String("code_hash", req.PhoneCodeHash),
		log.String("expected_code", phoneCode(phone)),
	)

	if err := h.checkCode(ctx, phone, req.PhoneCodeHash, phoneCode(phone)); err != nil {
		return nil, err
	}

	if _, ok, err := h.db.UserByPhone(ctx, phone); err != nil {
		return nil, h.internal(ctx, "lookup user", err)
	} else if ok {
		return nil, tgerr.New(400, "PHONE_NUMBER_OCCUPIED")
	}

	u, err := h.db.CreateUser(ctx, phone, req.FirstName, req.LastName)
	if err != nil {
		return nil, h.internal(ctx, "create user", err)
	}

	if err := h.bindCaller(ctx, u.ID); err != nil {
		return nil, h.internal(ctx, "bind session", err)
	}

	log.For(h.lg).Debug(ctx, "auth.signUp succeeded",
		log.String("phone", phone), log.Int64("user_id", u.ID))

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

// authBindTempAuthKey records the binding of a temporary (PFS) auth key — the
// one this request arrives on — to its permanent key, so requests on the temp
// key resolve to the permanent key's logged-in user. Clients that use temporary
// keys (e.g. Telegram Desktop) rotate them and re-bind on reconnect; without
// this the user binding would be lost on every rotation and after restart.
func (h *Handler) authBindTempAuthKey(ctx context.Context, req *tg.AuthBindTempAuthKeyRequest) (bool, error) {
	if err := h.requireDB(); err != nil {
		return false, err
	}

	tempID, ok := callerKeyID(ctx)
	if !ok {
		return false, tgerr.New(400, "ENCRYPTED_MESSAGE_INVALID")
	}

	var permID [8]byte

	binary.LittleEndian.PutUint64(permID[:], uint64(req.PermAuthKeyID)) // #nosec G115 -- bit reinterpretation.

	expires := time.Unix(int64(req.ExpiresAt), 0)
	if err := h.db.BindTempAuthKey(ctx, tempID, permID, expires); err != nil {
		return false, h.internal(ctx, "bind temp key", err)
	}

	log.For(h.lg).Debug(ctx, "auth.bindTempAuthKey",
		log.String("temp_key", hex.EncodeToString(tempID[:])),
		log.String("perm_key", hex.EncodeToString(permID[:])),
	)

	return true, nil
}

// authLogOut unbinds the session's user.
func (h *Handler) authLogOut(ctx context.Context) (*tg.AuthLoggedOut, error) {
	if h.db != nil {
		if keyID, ok := callerKeyID(ctx); ok {
			// Unbind the permanent key that actually holds the authorization.
			if eff, err := h.effectiveKeyID(ctx, keyID); err == nil {
				keyID = eff
			}

			if err := h.db.Unbind(ctx, keyID); err != nil {
				return nil, h.internal(ctx, "unbind", err)
			}
		}
	}

	return &tg.AuthLoggedOut{}, nil
}
