package rpc

import (
	"context"
	"encoding/hex"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
)

// requireBot resolves the caller and asserts it is a bot account, returning
// USER_BOT_REQUIRED otherwise. Bot command management is bot-only.
func (h *Handler) requireBot(ctx context.Context) (teled.User, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return teled.User{}, err
	}

	if !caller.IsBot {
		return teled.User{}, tgerr.New(400, "USER_BOT_REQUIRED")
	}

	return caller, nil
}

// scopeKey serializes a BotCommandScope to a stable, lossless key by encoding
// its MTProto representation. This keeps peer-specific scopes distinct from one
// another and from the global scopes. A nil scope keys as the default scope.
func scopeKey(scope tg.BotCommandScopeClass) string {
	if scope == nil {
		scope = &tg.BotCommandScopeDefault{}
	}

	var b bin.Buffer
	if err := scope.Encode(&b); err != nil {
		// Encoding a scope cannot realistically fail; fall back to the type id.
		return scope.TypeName()
	}

	return hex.EncodeToString(b.Buf)
}

// botsSetBotCommands stores the caller bot's command list for a scope/language.
func (h *Handler) botsSetBotCommands(ctx context.Context, req *tg.BotsSetBotCommandsRequest) (bool, error) {
	bot, err := h.requireBot(ctx)
	if err != nil {
		return false, err
	}

	commands := make([]teled.BotCommand, len(req.Commands))
	for i, c := range req.Commands {
		commands[i] = teled.BotCommand{Command: c.Command, Description: c.Description}
	}

	if err := h.db.SetBotCommands(ctx, bot.ID, scopeKey(req.Scope), req.LangCode, commands); err != nil {
		return false, h.internal(ctx, "set bot commands", err)
	}

	return true, nil
}

// botsGetBotCommands returns the caller bot's command list for a scope/language.
func (h *Handler) botsGetBotCommands(ctx context.Context, req *tg.BotsGetBotCommandsRequest) ([]tg.BotCommand, error) {
	bot, err := h.requireBot(ctx)
	if err != nil {
		return nil, err
	}

	commands, err := h.db.BotCommands(ctx, bot.ID, scopeKey(req.Scope), req.LangCode)
	if err != nil {
		return nil, h.internal(ctx, "get bot commands", err)
	}

	out := make([]tg.BotCommand, len(commands))
	for i, c := range commands {
		out[i] = tg.BotCommand{Command: c.Command, Description: c.Description}
	}

	return out, nil
}

// botsResetBotCommands clears the caller bot's command list for a scope/language.
func (h *Handler) botsResetBotCommands(ctx context.Context, req *tg.BotsResetBotCommandsRequest) (bool, error) {
	bot, err := h.requireBot(ctx)
	if err != nil {
		return false, err
	}

	if err := h.db.ResetBotCommands(ctx, bot.ID, scopeKey(req.Scope), req.LangCode); err != nil {
		return false, h.internal(ctx, "reset bot commands", err)
	}

	return true, nil
}
