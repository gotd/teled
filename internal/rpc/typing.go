package rpc

import (
	"context"
	"time"

	"github.com/gotd/td/tg"
)

// messagesSetTyping relays the caller's typing action to the peer's live
// sessions. It is transient and best-effort: nothing is persisted, and the
// action is dropped if the peer has no connected session.
func (h *Handler) messagesSetTyping(ctx context.Context, req *tg.MessagesSetTypingRequest) (bool, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return false, err
	}

	peer, err := h.resolvePeerUser(ctx, caller, req.Peer)
	if err != nil {
		return false, err
	}

	// Typing in Saved Messages has no other party to notify.
	if peer.ID != caller.ID {
		h.push(ctx, peer.ID,
			[]tg.UserClass{h.tgUser(caller, false), h.tgUser(peer, true)},
			int(time.Now().Unix()),
			&tg.UpdateUserTyping{UserID: caller.ID, Action: req.Action},
		)
	}

	return true, nil
}
