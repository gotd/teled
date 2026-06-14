package rpc

import (
	"context"
	"errors"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/gotd/teled"
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
		return nil, h.internal(ctx, "get dialogs", err)
	}

	drafts, err := h.db.Drafts(ctx, caller.ID)
	if err != nil {
		return nil, h.internal(ctx, "dialog drafts", err)
	}

	draftByPeer := make(map[int64]teled.Draft, len(drafts))
	for _, d := range drafts {
		draftByPeer[d.PeerUserID] = d
	}

	users := []tg.UserClass{h.tgUser(caller, true)}
	outDialogs := make([]tg.DialogClass, 0, len(dialogs))
	messages := make([]tg.MessageClass, 0, len(dialogs))
	seen := make(map[int64]bool, len(dialogs))

	for _, dl := range dialogs {
		peer, ok, err := h.db.UserByID(ctx, dl.PeerUserID)
		if err != nil {
			return nil, h.internal(ctx, "dialog peer", err)
		}

		if !ok {
			continue
		}

		seen[dl.PeerUserID] = true

		users = append(users, h.tgUser(*peer, false))

		top, err := h.db.GetHistory(ctx, caller.ID, dl.PeerUserID, 0, 1)
		if err != nil {
			return nil, h.internal(ctx, "dialog top", err)
		}

		if len(top) > 0 {
			messages = append(messages, dmMessage(top[0]))
		}

		d := &tg.Dialog{
			Peer:            &tg.PeerUser{UserID: dl.PeerUserID},
			TopMessage:      int(dl.TopMessageID),
			ReadInboxMaxID:  int(dl.ReadInboxMaxID),
			ReadOutboxMaxID: int(dl.ReadOutboxMaxID),
			UnreadCount:     dl.UnreadCount,
		}

		if dr, ok := draftByPeer[dl.PeerUserID]; ok {
			d.Draft = draftMessage(dr)
		}

		d.SetFlags()
		outDialogs = append(outDialogs, d)
	}

	// A draft alone forms a dialog (no messages exchanged yet). Prepend those
	// not already covered, newest draft first.
	draftOnly := make([]tg.DialogClass, 0)

	for _, dr := range drafts {
		if seen[dr.PeerUserID] {
			continue
		}

		peer, ok, err := h.db.UserByID(ctx, dr.PeerUserID)
		if err != nil {
			return nil, h.internal(ctx, "draft peer", err)
		}

		if !ok {
			continue
		}

		users = append(users, h.tgUser(*peer, false))

		d := &tg.Dialog{Peer: &tg.PeerUser{UserID: dr.PeerUserID}, Draft: draftMessage(dr)}
		d.SetFlags()
		draftOnly = append(draftOnly, d)
	}

	outDialogs = append(draftOnly, outDialogs...)

	return &tg.MessagesDialogs{Dialogs: outDialogs, Messages: messages, Users: users}, nil
}

// messagesGetPeerDialogs returns dialog entries (read state, top message,
// draft) for specific peers. Clients call this to refresh the unread count
// after reading a chat (tdesktop's requestDialogEntry), so it must report the
// real read state — an empty reply leaves the unread badge stuck.
func (h *Handler) messagesGetPeerDialogs(ctx context.Context, peers []tg.InputDialogPeerClass) (*tg.MessagesPeerDialogs, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	dialogs, err := h.db.GetDialogs(ctx, caller.ID, dialogsDefaultLimit)
	if err != nil {
		return nil, h.internal(ctx, "get dialogs", err)
	}

	byPeer := make(map[int64]teled.Dialog, len(dialogs))
	for _, d := range dialogs {
		byPeer[d.PeerUserID] = d
	}

	drafts, err := h.db.Drafts(ctx, caller.ID)
	if err != nil {
		return nil, h.internal(ctx, "dialog drafts", err)
	}

	draftByPeer := make(map[int64]teled.Draft, len(drafts))
	for _, d := range drafts {
		draftByPeer[d.PeerUserID] = d
	}

	st, err := h.db.GetState(ctx, caller.ID)
	if err != nil {
		return nil, h.internal(ctx, "get state", err)
	}

	out := &tg.MessagesPeerDialogs{State: *tgState(st)}
	users := []tg.UserClass{h.tgUser(caller, true)}
	seen := map[int64]bool{caller.ID: true}

	for _, p := range peers {
		dp, ok := p.(*tg.InputDialogPeer)
		if !ok {
			continue
		}

		peer, err := h.resolvePeerUser(ctx, caller, dp.Peer)
		if err != nil {
			continue
		}

		dl := byPeer[peer.ID] // zero-valued when the conversation has no messages

		d := &tg.Dialog{
			Peer:            &tg.PeerUser{UserID: peer.ID},
			TopMessage:      int(dl.TopMessageID),
			ReadInboxMaxID:  int(dl.ReadInboxMaxID),
			ReadOutboxMaxID: int(dl.ReadOutboxMaxID),
			UnreadCount:     dl.UnreadCount,
		}

		if dr, ok := draftByPeer[peer.ID]; ok {
			d.Draft = draftMessage(dr)
		}

		d.SetFlags()
		out.Dialogs = append(out.Dialogs, d)

		top, err := h.db.GetHistory(ctx, caller.ID, peer.ID, 0, 1)
		if err != nil {
			return nil, h.internal(ctx, "dialog top", err)
		}

		if len(top) > 0 {
			out.Messages = append(out.Messages, dmMessage(top[0]))
		}

		if !seen[peer.ID] {
			users = append(users, h.tgUser(peer, false))
			seen[peer.ID] = true
		}
	}

	out.Users = users

	return out, nil
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

	res, err := h.db.ReadHistory(ctx, caller.ID, peer.ID, int64(req.MaxID))
	if err != nil {
		return nil, h.internal(ctx, "read history", err)
	}

	now := int(time.Now().Unix())

	// Sync the read to the caller's other sessions so the unread badge clears
	// everywhere.
	inbox := &tg.UpdateReadHistoryInbox{
		Peer: &tg.PeerUser{UserID: peer.ID}, MaxID: req.MaxID,
		StillUnreadCount: res.UnreadCount, Pts: res.InboxPts, PtsCount: 1,
	}
	inbox.SetFlags()
	h.push(ctx, caller.ID, []tg.UserClass{h.tgUser(caller, true), h.tgUser(peer, false)}, now, inbox)

	// Read-receipt: tell the peer their messages were read (their "sent" ticks
	// turn to "read").
	if res.OutboxMaxID > 0 {
		h.push(ctx, peer.ID,
			[]tg.UserClass{h.tgUser(caller, false), h.tgUser(peer, true)}, now,
			&tg.UpdateReadHistoryOutbox{
				Peer: &tg.PeerUser{UserID: caller.ID}, MaxID: int(res.OutboxMaxID),
				Pts: res.OutboxPts, PtsCount: 1,
			},
		)
	}

	return &tg.MessagesAffectedMessages{Pts: res.InboxPts, PtsCount: 1}, nil
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
		if errors.Is(err, teled.ErrMessageID) {
			return nil, tgerr.New(400, "MESSAGE_ID_INVALID")
		}

		return nil, h.internal(ctx, "edit message", err)
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
			[]tg.UserClass{h.tgUser(caller, false), h.tgUser(peer, true)},
			int(res.EditDate.Unix()),
			&tg.UpdateEditMessage{Message: peerView, Pts: res.PeerPts, PtsCount: 1},
		)
	}

	return &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateEditMessage{Message: edited, Pts: res.SelfPts, PtsCount: 1}},
		Users:   []tg.UserClass{h.tgUser(caller, true), h.tgUser(peer, false)},
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
		return nil, h.internal(ctx, "delete messages", err)
	}

	return &tg.MessagesAffectedMessages{Pts: res.Pts, PtsCount: res.PtsCount}, nil
}
