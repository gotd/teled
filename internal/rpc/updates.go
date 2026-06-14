package rpc

import (
	"context"
	"time"

	"github.com/gotd/td/tg"

	"github.com/gotd/teled"
)

const differenceLimit = 1000

// updatesGetState returns the caller's current update state.
func (h *Handler) updatesGetState(ctx context.Context) (*tg.UpdatesState, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	st, err := h.db.GetState(ctx, caller.ID)
	if err != nil {
		return nil, h.internal(ctx, "get state", err)
	}
	return tgState(st), nil
}

// updatesGetDifference returns updates newer than the client's pts.
func (h *Handler) updatesGetDifference(ctx context.Context, req *tg.UpdatesGetDifferenceRequest) (tg.UpdatesDifferenceClass, error) {
	caller, err := h.requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	entries, currentPts, err := h.db.GetDifference(ctx, caller.ID, req.Pts, differenceLimit)
	if err != nil {
		return nil, h.internal(ctx, "get difference", err)
	}
	if len(entries) == 0 {
		return &tg.UpdatesDifferenceEmpty{Date: int(time.Now().Unix()), Seq: 0}, nil
	}

	var (
		newMessages  []tg.MessageClass
		otherUpdates []tg.UpdateClass
		userIDs      = map[int64]struct{}{caller.ID: {}}
	)

	for _, e := range entries {
		switch e.Type {
		case teled.UpdateNew:
			if msg, ok := h.messageByGlobal(ctx, caller.ID, e); ok {
				newMessages = append(newMessages, dmMessage(msg))
				userIDs[msg.FromUserID] = struct{}{}
				userIDs[msg.PeerUserID] = struct{}{}
			}
		case teled.UpdateEdit:
			if msg, ok := h.messageByGlobal(ctx, caller.ID, e); ok {
				otherUpdates = append(otherUpdates, &tg.UpdateEditMessage{
					Message: dmMessage(msg), Pts: e.Pts, PtsCount: e.PtsCount,
				})
				userIDs[msg.FromUserID] = struct{}{}
				userIDs[msg.PeerUserID] = struct{}{}
			}
		case teled.UpdateDelete:
			ids := teled.DecodeDeleted(e.Extra)
			msgIDs := make([]int, len(ids))
			for i, id := range ids {
				msgIDs[i] = int(id)
			}
			otherUpdates = append(otherUpdates, &tg.UpdateDeleteMessages{
				Messages: msgIDs, Pts: e.Pts, PtsCount: e.PtsCount,
			})
		case teled.UpdateReadInbox:
			peer, maxID := teled.DecodeRead(e.Extra)
			userIDs[peer] = struct{}{}
			otherUpdates = append(otherUpdates, &tg.UpdateReadHistoryInbox{
				Peer: &tg.PeerUser{UserID: peer}, MaxID: int(maxID), Pts: e.Pts, PtsCount: e.PtsCount,
			})
		}
	}

	users, err := h.usersByIDs(ctx, caller.ID, userIDs)
	if err != nil {
		return nil, h.internal(ctx, "difference users", err)
	}

	return &tg.UpdatesDifference{
		NewMessages:  newMessages,
		OtherUpdates: otherUpdates,
		Users:        users,
		State:        *tgState(teled.State{Pts: currentPts, Date: time.Now()}),
	}, nil
}

func (h *Handler) messageByGlobal(ctx context.Context, self int64, e teled.UpdateLogEntry) (teled.Message, bool) {
	if e.GlobalID == nil {
		return teled.Message{}, false
	}
	msg, ok, err := h.db.MessageByGlobal(ctx, self, *e.GlobalID)
	if err != nil || !ok {
		return teled.Message{}, false
	}
	return msg, true
}

// usersByIDs loads users and marks the caller as self.
func (h *Handler) usersByIDs(ctx context.Context, self int64, ids map[int64]struct{}) ([]tg.UserClass, error) {
	list := make([]int64, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	users, err := h.db.UsersByIDs(ctx, list)
	if err != nil {
		return nil, err
	}
	out := make([]tg.UserClass, 0, len(users))
	for i := range users {
		out = append(out, toTGUser(users[i], users[i].ID == self))
	}
	return out, nil
}

func tgState(st teled.State) *tg.UpdatesState {
	return &tg.UpdatesState{
		Pts:         st.Pts,
		Qts:         st.Qts,
		Seq:         st.Seq,
		Date:        int(st.Date.Unix()),
		UnreadCount: st.UnreadCount,
	}
}
