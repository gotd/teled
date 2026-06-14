package rpc

import (
	"context"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"github.com/gotd/teled"
)

// draftMessage converts a stored draft to its wire form, auto-detecting link
// and mention entities like a sent message.
func draftMessage(d teled.Draft) *tg.DraftMessage {
	m := &tg.DraftMessage{Message: d.Text, Date: int(d.Date.Unix())}
	if ents := messageEntities(d.Text); len(ents) > 0 {
		m.Entities = ents
	}

	m.SetFlags()

	return m
}

// draftUpdate builds an UpdateDraftMessage for a peer.
func draftUpdate(peerID int64, draft tg.DraftMessageClass) *tg.UpdateDraftMessage {
	upd := &tg.UpdateDraftMessage{Peer: &tg.PeerUser{UserID: peerID}, Draft: draft}
	upd.SetFlags()

	return upd
}

// messagesSaveDraft stores (or clears) the caller's draft for a peer and syncs
// it to the caller's other sessions via updateDraftMessage.
func (h *Handler) messagesSaveDraft(ctx context.Context, req *tg.MessagesSaveDraftRequest) (bool, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return false, err
	}

	peer, err := h.resolvePeerUser(ctx, caller, req.Peer)
	if err != nil {
		return false, err
	}

	date, err := h.db.SaveDraft(ctx, caller.ID, peer.ID, req.Message)
	if err != nil {
		return false, h.internal(ctx, "save draft", err)
	}

	var draft tg.DraftMessageClass = &tg.DraftMessageEmpty{}
	if strings.TrimSpace(req.Message) != "" {
		draft = draftMessage(teled.Draft{PeerUserID: peer.ID, Text: req.Message, Date: date})
	}

	h.push(ctx, caller.ID,
		[]tg.UserClass{h.tgUser(caller, true), h.tgUser(peer, false)},
		int(time.Now().Unix()),
		draftUpdate(peer.ID, draft),
	)

	return true, nil
}

// messagesGetAllDrafts returns all of the caller's saved drafts as a batch of
// updateDraftMessage updates.
func (h *Handler) messagesGetAllDrafts(ctx context.Context) (tg.UpdatesClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	drafts, err := h.db.Drafts(ctx, caller.ID)
	if err != nil {
		return nil, h.internal(ctx, "get drafts", err)
	}

	updates := make([]tg.UpdateClass, 0, len(drafts))
	users := []tg.UserClass{h.tgUser(caller, true)}
	seen := map[int64]bool{caller.ID: true}

	for _, d := range drafts {
		updates = append(updates, draftUpdate(d.PeerUserID, draftMessage(d)))

		if seen[d.PeerUserID] {
			continue
		}

		if u, ok, err := h.db.UserByID(ctx, d.PeerUserID); err == nil && ok {
			users = append(users, h.tgUser(*u, false))
			seen[d.PeerUserID] = true
		}
	}

	return &tg.Updates{Updates: updates, Users: users, Date: int(time.Now().Unix())}, nil
}
