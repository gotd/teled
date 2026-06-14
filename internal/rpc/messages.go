package rpc

import (
	"context"

	"github.com/gotd/log"
	"github.com/gotd/td/proto"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
)

const (
	historyDefaultLimit = 100
	historyMaxLimit     = 100
)

// messagesSendMessage persists a DM and returns the sender's updates
// (UpdateMessageID mapping random_id -> id, then UpdateNewMessage).
func (h *Handler) messagesSendMessage(ctx context.Context, req *tg.MessagesSendMessageRequest) (tg.UpdatesClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	peer, err := h.resolvePeerUser(ctx, caller, req.Peer)
	if err != nil {
		return nil, err
	}

	sent, err := h.db.SendMessage(ctx, caller.ID, peer.ID, req.Message, req.RandomID, 0)
	if err != nil {
		return nil, h.internal(ctx, "send message", err)
	}

	out := dmMessage(teled.Message{
		LocalID:    sent.SenderLocalID,
		FromUserID: caller.ID,
		PeerUserID: peer.ID,
		Out:        true,
		Text:       req.Message,
		Date:       sent.Date,
	})

	h.deliver(ctx, caller, peer, sent, req.Message)

	// BotFather answers DMs to it inline: its replies are persisted and pushed
	// before this RPC returns, so they are immediately visible in history.
	if peer.ID == teled.BotFatherID {
		h.handleBotFather(ctx, caller, peer, req.Message)
	}

	return &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: int(sent.SenderLocalID), RandomID: req.RandomID},
			&tg.UpdateNewMessage{Message: out, Pts: sent.SenderPts, PtsCount: 1},
		},
		Users: []tg.UserClass{toTGUser(caller, true), toTGUser(peer, false)},
		Date:  int(sent.Date.Unix()),
	}, nil
}

// push best-effort delivers updates to a user's live sessions.
func (h *Handler) push(ctx context.Context, userID int64, users []tg.UserClass, date int, updates ...tg.UpdateClass) {
	server := serverFrom(ctx)
	sessions := h.sessions.get(userID)

	if server == nil || len(sessions) == 0 {
		return
	}

	wrap := &tg.Updates{Updates: updates, Users: users, Date: date}
	for _, s := range sessions {
		if err := server.Send(ctx, s, proto.MessageFromServer, wrap); err != nil {
			log.For(h.lg).Debug(ctx, "push failed", log.Error(err))
		}
	}
}

// deliver best-effort pushes the new message to the recipient's live sessions.
func (h *Handler) deliver(ctx context.Context, from, peer teled.User, sent teled.SentMessage, text string) {
	if sent.SelfChat {
		return
	}

	incoming := dmMessage(teled.Message{
		LocalID:    sent.RecipientLocalID,
		FromUserID: from.ID,
		PeerUserID: from.ID,
		Out:        false,
		Text:       text,
		Date:       sent.Date,
	})
	h.push(ctx, peer.ID,
		[]tg.UserClass{toTGUser(from, false), toTGUser(peer, true)},
		int(sent.Date.Unix()),
		&tg.UpdateNewMessage{Message: incoming, Pts: sent.RecipientPts, PtsCount: 1},
	)
}

// messagesGetHistory returns the conversation with a peer, newest first.
func (h *Handler) messagesGetHistory(ctx context.Context, req *tg.MessagesGetHistoryRequest) (tg.MessagesMessagesClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	peer, err := h.resolvePeerUser(ctx, caller, req.Peer)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 || limit > historyMaxLimit {
		limit = historyDefaultLimit
	}

	msgs, err := h.db.GetHistory(ctx, caller.ID, peer.ID, int64(req.OffsetID), limit)
	if err != nil {
		return nil, h.internal(ctx, "get history", err)
	}

	out := make([]tg.MessageClass, 0, len(msgs))
	for i := range msgs {
		out = append(out, dmMessage(msgs[i]))
	}

	return &tg.MessagesMessages{
		Messages: out,
		Users:    []tg.UserClass{toTGUser(caller, true), toTGUser(peer, false)},
	}, nil
}

// messagesGetSavedHistory returns the caller's Saved Messages (self-chat).
// Modern clients (e.g. Telegram Desktop) load Saved Messages through this RPC
// rather than messages.getHistory, so without it Saved Messages appears empty
// even though the messages are persisted.
func (h *Handler) messagesGetSavedHistory(ctx context.Context, req *tg.MessagesGetSavedHistoryRequest) (tg.MessagesMessagesClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 || limit > historyMaxLimit {
		limit = historyDefaultLimit
	}

	msgs, err := h.db.GetHistory(ctx, caller.ID, caller.ID, int64(req.OffsetID), limit)
	if err != nil {
		return nil, h.internal(ctx, "get saved history", err)
	}

	out := make([]tg.MessageClass, 0, len(msgs))
	for i := range msgs {
		out = append(out, dmMessage(msgs[i]))
	}

	return &tg.MessagesMessages{
		Messages: out,
		Users:    []tg.UserClass{toTGUser(caller, true)},
	}, nil
}

// contactsResolveUsername resolves a username to a user peer.
func (h *Handler) contactsResolveUsername(ctx context.Context, req *tg.ContactsResolveUsernameRequest) (*tg.ContactsResolvedPeer, error) {
	if err := h.requireDB(); err != nil {
		return nil, err
	}

	u, ok, err := h.db.UserByUsername(ctx, req.Username)
	if err != nil {
		return nil, h.internal(ctx, "resolve username", err)
	}

	if !ok {
		return nil, tgerr.New(400, "USERNAME_NOT_OCCUPIED")
	}

	return &tg.ContactsResolvedPeer{
		Peer:  &tg.PeerUser{UserID: u.ID},
		Users: []tg.UserClass{toTGUser(*u, false)},
	}, nil
}
