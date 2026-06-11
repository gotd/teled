package rpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

// devPhoneCode is the fixed login code issued in lieu of real SMS delivery.
const devPhoneCode = "12345"

// codeTTL bounds how long an issued login code is valid.
const codeTTL = 10 * time.Minute

// authSendCode issues a login code for the phone number. No SMS is sent; the
// fixed dev code is accepted by signIn/signUp.
func (h *Handler) authSendCode(ctx context.Context, req *tg.AuthSendCodeRequest) (tg.AuthSentCodeClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, h.internal("rand", err)
	}
	codeHash := hex.EncodeToString(raw[:])

	if err := h.db.SavePhoneCode(ctx, req.PhoneNumber, codeHash, devPhoneCode, codeTTL); err != nil {
		return nil, h.internal("save code", err)
	}

	sent := &tg.AuthSentCode{
		Type:          &tg.AuthSentCodeTypeSMS{Length: len(devPhoneCode)},
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
		return h.internal("get code", err)
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
		return nil, h.internal("lookup user", err)
	}
	if !ok {
		return &tg.AuthAuthorizationSignUpRequired{}, nil
	}

	if err := h.bindCaller(ctx, u.ID); err != nil {
		return nil, h.internal("bind session", err)
	}
	return &tg.AuthAuthorization{User: toTGUser(*u, true)}, nil
}

// authSignUp creates a new account for an unoccupied phone number.
func (h *Handler) authSignUp(ctx context.Context, req *tg.AuthSignUpRequest) (tg.AuthAuthorizationClass, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}
	if err := h.checkCode(ctx, req.PhoneNumber, req.PhoneCodeHash, devPhoneCode); err != nil {
		return nil, err
	}

	if _, ok, err := h.db.UserByPhone(ctx, req.PhoneNumber); err != nil {
		return nil, h.internal("lookup user", err)
	} else if ok {
		return nil, tgerr.New(400, "PHONE_NUMBER_OCCUPIED")
	}

	u, err := h.db.CreateUser(ctx, req.PhoneNumber, req.FirstName, req.LastName)
	if err != nil {
		return nil, h.internal("create user", err)
	}
	if err := h.bindCaller(ctx, u.ID); err != nil {
		return nil, h.internal("bind session", err)
	}
	return &tg.AuthAuthorization{User: toTGUser(u, true)}, nil
}

// authLogOut unbinds the session's user.
func (h *Handler) authLogOut(ctx context.Context) (*tg.AuthLoggedOut, error) {
	if h.db != nil {
		if keyID, ok := callerKeyID(ctx); ok {
			if err := h.db.Unbind(ctx, keyID); err != nil {
				return nil, h.internal("unbind", err)
			}
		}
	}
	return &tg.AuthLoggedOut{}, nil
}
