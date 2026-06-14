package rpc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/gotd/td/tg"

	"github.com/gotd/teled"
)

// BotFather reply texts. Kept close to the real BotFather wording so client
// code and tests that match on it feel familiar.
const (
	bfIntro = "I can help you create and manage Telegram bots. " +
		"Use /newbot to create a new bot.\n\n" +
		"Commands:\n" +
		"/newbot - create a new bot\n" +
		"/mybots - list your bots\n" +
		"/token - show a bot's token\n" +
		"/revoke - revoke a bot's token\n" +
		"/cancel - cancel the current operation"
	bfAskName     = "Alright, a new bot. How are we going to call it? Please choose a name for your bot."
	bfAskUsername = "Good. Now let's choose a username for your bot. It must end in `bot`. " +
		"Like this, for example: TetrisBot or tetris_bot."
	bfAskRevoke     = "Choose a bot to revoke the token. Send its username, for example @tetris_bot."
	bfCancelled     = "Canceled."
	bfNothingToDo   = "There is no active command to cancel."
	bfNoBots        = "You don't have any bots yet. Use /newbot to create one."
	bfBadName       = "Sorry, this name is invalid. Please choose a name for your bot."
	bfNotUnderstood = "I don't understand. Please use /help to see the list of commands."
)

// handleBotFather runs the BotFather engine for an incoming DM and delivers its
// replies as messages from BotFather to the caller. It is best-effort: failures
// are logged, never surfaced to the sending client.
func (h *Handler) handleBotFather(ctx context.Context, caller, botFather teled.User, text string) {
	replies, err := h.botFatherEngine(ctx, caller, text)
	if err != nil {
		h.lg.Error("botfather engine", zap.Error(err))
		return
	}
	for _, reply := range replies {
		sent, err := h.db.SendMessage(ctx, botFather.ID, caller.ID, reply, 0, 0)
		if err != nil {
			h.lg.Error("botfather reply", zap.Error(err))
			return
		}
		incoming := dmMessage(teled.Message{
			LocalID:    sent.RecipientLocalID,
			FromUserID: botFather.ID,
			PeerUserID: botFather.ID,
			Out:        false,
			Text:       reply,
			Date:       sent.Date,
		})
		h.push(ctx, caller.ID,
			[]tg.UserClass{toTGUser(botFather, false), toTGUser(caller, true)},
			int(sent.Date.Unix()),
			&tg.UpdateNewMessage{Message: incoming, Pts: sent.RecipientPts, PtsCount: 1},
		)
	}
}

// botFatherEngine maps an incoming message (plus any pending flow state) to
// BotFather's replies, applying state changes and bot creation as side effects.
func (h *Handler) botFatherEngine(ctx context.Context, caller teled.User, text string) ([]string, error) {
	text = strings.TrimSpace(text)

	if cmd, ok := botCommand(text); ok {
		return h.botFatherCommand(ctx, caller, cmd)
	}

	state, err := h.db.BotFatherState(ctx, caller.ID)
	if err != nil {
		return nil, err
	}
	switch state.Step {
	case teled.BotFatherStepNewBotName:
		return h.botFatherSetName(ctx, caller, text)
	case teled.BotFatherStepNewBotUsername:
		return h.botFatherCreate(ctx, caller, state.DraftName, text)
	case teled.BotFatherStepRevokeSelect:
		return h.botFatherRevoke(ctx, caller, text)
	default:
		return []string{bfNotUnderstood}, nil
	}
}

// botCommand extracts a lowercased "/command" head, dropping any "@botname"
// suffix and arguments. ok is false when text is not a command.
func botCommand(text string) (string, bool) {
	if !strings.HasPrefix(text, "/") {
		return "", false
	}
	head := strings.Fields(text)[0]
	if at := strings.IndexByte(head, '@'); at >= 0 {
		head = head[:at]
	}
	return strings.ToLower(head), true
}

func (h *Handler) botFatherCommand(ctx context.Context, caller teled.User, cmd string) ([]string, error) {
	switch cmd {
	case "/start", "/help":
		return []string{bfIntro}, nil
	case "/newbot":
		if err := h.db.SetBotFatherState(ctx, caller.ID, teled.BotFatherState{Step: teled.BotFatherStepNewBotName}); err != nil {
			return nil, err
		}
		return []string{bfAskName}, nil
	case "/mybots", "/token":
		return h.botFatherListBots(ctx, caller)
	case "/revoke":
		bots, err := h.db.BotsByOwner(ctx, caller.ID)
		if err != nil {
			return nil, err
		}
		if len(bots) == 0 {
			return []string{bfNoBots}, nil
		}
		if err := h.db.SetBotFatherState(ctx, caller.ID, teled.BotFatherState{Step: teled.BotFatherStepRevokeSelect}); err != nil {
			return nil, err
		}
		return []string{bfAskRevoke}, nil
	case "/cancel":
		state, err := h.db.BotFatherState(ctx, caller.ID)
		if err != nil {
			return nil, err
		}
		if state.Step == "" {
			return []string{bfNothingToDo}, nil
		}
		if err := h.db.ClearBotFatherState(ctx, caller.ID); err != nil {
			return nil, err
		}
		return []string{bfCancelled}, nil
	default:
		return []string{bfNotUnderstood}, nil
	}
}

func (h *Handler) botFatherListBots(ctx context.Context, caller teled.User) ([]string, error) {
	bots, err := h.db.BotsByOwner(ctx, caller.ID)
	if err != nil {
		return nil, err
	}
	if len(bots) == 0 {
		return []string{bfNoBots}, nil
	}
	var b strings.Builder
	b.WriteString("Here are your bots:\n")
	for _, bot := range bots {
		fmt.Fprintf(&b, "\n@%s\n%s", bot.Username, bot.BotToken)
	}
	return []string{b.String()}, nil
}

func (h *Handler) botFatherSetName(ctx context.Context, caller teled.User, name string) ([]string, error) {
	if !validBotName(name) {
		return []string{bfBadName}, nil
	}
	if err := h.db.SetBotFatherState(ctx, caller.ID, teled.BotFatherState{
		Step:      teled.BotFatherStepNewBotUsername,
		DraftName: name,
	}); err != nil {
		return nil, err
	}
	return []string{bfAskUsername}, nil
}

func (h *Handler) botFatherCreate(ctx context.Context, caller teled.User, name, username string) ([]string, error) {
	username = strings.TrimPrefix(username, "@")
	if msg, ok := validBotUsername(username); !ok {
		return []string{msg}, nil
	}

	if _, taken, err := h.db.UserByUsername(ctx, username); err != nil {
		return nil, err
	} else if taken {
		return []string{"Sorry, this username is already taken. Please try something different."}, nil
	}

	secret, err := botTokenSecret()
	if err != nil {
		return nil, err
	}
	bot, err := h.db.CreateOwnedBot(ctx, username, name, caller.ID, secret)
	if err != nil {
		return nil, err
	}
	if err := h.db.ClearBotFatherState(ctx, caller.ID); err != nil {
		return nil, err
	}

	return []string{fmt.Sprintf(
		"Done! Congratulations on your new bot. You will find it at t.me/%s.\n\n"+
			"Use this token to access the HTTP API:\n%s\n\n"+
			"Keep your token secure and store it safely, it can be used by anyone to control your bot.",
		bot.Username, bot.BotToken,
	)}, nil
}

func (h *Handler) botFatherRevoke(ctx context.Context, caller teled.User, username string) ([]string, error) {
	username = strings.TrimPrefix(username, "@")
	bots, err := h.db.BotsByOwner(ctx, caller.ID)
	if err != nil {
		return nil, err
	}
	var target *teled.User
	for i := range bots {
		if strings.EqualFold(bots[i].Username, username) {
			target = &bots[i]
			break
		}
	}
	if target == nil {
		return []string{"You don't own a bot with that username. Send the username of one of your bots."}, nil
	}

	secret, err := botTokenSecret()
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("%d:%s", target.ID, secret)
	if err := h.db.SetBotToken(ctx, target.ID, token); err != nil {
		return nil, err
	}
	if err := h.db.ClearBotFatherState(ctx, caller.ID); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf(
		"Token revoked. The previous token has stopped working.\n\n"+
			"New token for @%s:\n%s", target.Username, token,
	)}, nil
}

// validBotName accepts any non-empty, reasonably short, non-command name.
func validBotName(name string) bool {
	return name != "" && len(name) <= 64 && !strings.HasPrefix(name, "/")
}

// validBotUsername enforces BotFather's username rules, returning the
// user-facing error when invalid.
func validBotUsername(u string) (string, bool) {
	const help = "Sorry, this username is invalid. A username must be 5-32 characters long, " +
		"start with a letter, use only letters, digits and underscores, and end in `bot`."
	if len(u) < 5 || len(u) > 32 {
		return help, false
	}
	if !isASCIILetter(u[0]) {
		return help, false
	}
	for i := 0; i < len(u); i++ {
		if !isUsernameChar(u[i]) {
			return help, false
		}
	}
	if !strings.HasSuffix(strings.ToLower(u), "bot") {
		return help, false
	}
	return "", true
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isUsernameChar reports whether c is allowed in a bot username.
func isUsernameChar(c byte) bool {
	return isASCIILetter(c) || (c >= '0' && c <= '9') || c == '_'
}

// botTokenSecret returns the part of a bot token after the colon: 35 URL-safe
// characters, matching the shape of a real BotFather token secret.
func botTokenSecret() (string, error) {
	var raw [26]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
