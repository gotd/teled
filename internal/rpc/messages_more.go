package rpc

import (
	"context"
	"errors"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
	"github.com/gotd/teled/internal/db"
)

const dialogsDefaultLimit = 100

// messagesGetDialogs returns the caller's dialog list with top messages.
func (h *Handler) messagesGetDialogs(ctx context.Context, req *tg.MessagesGetDialogsRequest) (tg.MessagesDialogsClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 || limit > dialogsDefaultLimit {
		limit = dialogsDefaultLimit
	}

	dialogs, err := h.db.GetDialogs(ctx, caller.ID, limit)
	if err != nil {
		return nil, h.internal("get dialogs", err)
	}

	users := []tg.UserClass{toTGUser(caller, true)}
	outDialogs := make([]tg.DialogClass, 0, len(dialogs))
	messages := make([]tg.MessageClass, 0, len(dialogs))

	for _, dl := range dialogs {
		peer, ok, err := h.db.UserByID(ctx, dl.PeerUserID)
		if err != nil {
			return nil, h.internal("dialog peer", err)
		}
		if !ok {
			continue
		}
		users = append(users, toTGUser(*peer, false))

		top, err := h.db.GetHistory(ctx, caller.ID, dl.PeerUserID, 0, 1)
		if err != nil {
			return nil, h.internal("dialog top", err)
		}
		if len(top) > 0 {
			messages = append(messages, dmMessage(top[0]))
		}

		d := &tg.Dialog{
			Peer:           &tg.PeerUser{UserID: dl.PeerUserID},
			TopMessage:     int(dl.TopMessageID),
			ReadInboxMaxID: int(dl.ReadInboxMaxID),
			UnreadCount:    dl.UnreadCount,
		}
		d.SetFlags()
		outDialogs = append(outDialogs, d)
	}

	return &tg.MessagesDialogs{Dialogs: outDialogs, Messages: messages, Users: users}, nil
}

// messagesReadHistory marks incoming messages from a peer as read.
func (h *Handler) messagesReadHistory(ctx context.Context, req *tg.MessagesReadHistoryRequest) (*tg.MessagesAffectedMessages, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := h.resolvePeerUser(ctx, caller, req.Peer)
	if err != nil {
		return nil, err
	}

	pts, err := h.db.ReadHistory(ctx, caller.ID, peer.ID, int64(req.MaxID))
	if err != nil {
		return nil, h.internal("read history", err)
	}
	return &tg.MessagesAffectedMessages{Pts: pts, PtsCount: 1}, nil
}

// messagesEditMessage edits a message the caller sent.
func (h *Handler) messagesEditMessage(ctx context.Context, req *tg.MessagesEditMessageRequest) (tg.UpdatesClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := h.resolvePeerUser(ctx, caller, req.Peer)
	if err != nil {
		return nil, err
	}

	res, err := h.db.EditMessage(ctx, caller.ID, int64(req.ID), req.Message)
	if err != nil {
		if errors.Is(err, db.ErrMessageID) {
			return nil, tgerr.New(400, "MESSAGE_ID_INVALID")
		}
		return nil, h.internal("edit message", err)
	}

	edited := dmMessage(teled.Message{
		LocalID: res.SelfLocalID, FromUserID: caller.ID, PeerUserID: peer.ID,
		Out: true, Text: req.Message, Date: res.Date, EditDate: res.EditDate,
	})

	if res.PeerLocalID != 0 {
		peerView := dmMessage(teled.Message{
			LocalID: res.PeerLocalID, FromUserID: caller.ID, PeerUserID: caller.ID,
			Out: false, Text: req.Message, Date: res.Date, EditDate: res.EditDate,
		})
		h.push(ctx, peer.ID,
			[]tg.UserClass{toTGUser(caller, false), toTGUser(peer, true)},
			int(res.EditDate.Unix()),
			&tg.UpdateEditMessage{Message: peerView, Pts: res.PeerPts, PtsCount: 1},
		)
	}

	return &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateEditMessage{Message: edited, Pts: res.SelfPts, PtsCount: 1}},
		Users:   []tg.UserClass{toTGUser(caller, true), toTGUser(peer, false)},
		Date:    int(res.EditDate.Unix()),
	}, nil
}

// messagesDeleteMessages deletes messages by the caller's local ids.
func (h *Handler) messagesDeleteMessages(ctx context.Context, req *tg.MessagesDeleteMessagesRequest) (*tg.MessagesAffectedMessages, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, len(req.ID))
	for i, id := range req.ID {
		ids[i] = int64(id)
	}

	res, err := h.db.DeleteMessages(ctx, caller.ID, ids)
	if err != nil {
		return nil, h.internal("delete messages", err)
	}
	return &tg.MessagesAffectedMessages{Pts: res.Pts, PtsCount: res.PtsCount}, nil
}
