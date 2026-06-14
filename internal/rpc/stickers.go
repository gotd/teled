package rpc

import (
	"context"

	"github.com/gotd/td/tg"
)

// Sticker and GIF catalogs. teled has no sticker store, so each method reports
// an empty/"not modified" set, which a full client (e.g. Telegram Desktop)
// requests on startup.

func (h *Handler) messagesGetAllStickers(context.Context, int64) (tg.MessagesAllStickersClass, error) {
	return &tg.MessagesAllStickersNotModified{}, nil
}

func (h *Handler) messagesGetEmojiStickers(context.Context, int64) (tg.MessagesAllStickersClass, error) {
	return &tg.MessagesAllStickersNotModified{}, nil
}

func (h *Handler) messagesGetFavedStickers(context.Context, int64) (tg.MessagesFavedStickersClass, error) {
	return &tg.MessagesFavedStickersNotModified{}, nil
}
