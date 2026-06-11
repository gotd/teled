package teled

import "time"

// Message is a DM message from the perspective of one account (the viewer).
type Message struct {
	GlobalID   int64     // canonical id, shared by both participants
	LocalID    int64     // viewer's per-account message id
	FromUserID int64     // sender
	PeerUserID int64     // the other party in the conversation
	Out        bool      // true if the viewer is the sender
	Text       string    // message text
	Date       time.Time // sent time
	EditDate   time.Time // zero if never edited
	RandomID   int64     // sender-provided dedup id
}

// Dialog is one conversation in an account's dialog list.
type Dialog struct {
	PeerUserID     int64
	TopMessageID   int64
	ReadInboxMaxID int64
	UnreadCount    int
}

// EditResult is the outcome of editing a message.
type EditResult struct {
	SelfLocalID int64
	SelfPts     int
	PeerUserID  int64
	PeerLocalID int64 // 0 when the peer has no copy (self-chat)
	PeerPts     int
	Date        time.Time
	EditDate    time.Time
}

// DeleteResult is the outcome of deleting messages.
type DeleteResult struct {
	Pts      int
	PtsCount int
	LocalIDs []int64 // the caller's local ids actually deleted
}

// SentMessage is the result of persisting a DM: the per-account local ids and
// pts allocated for each participant.
type SentMessage struct {
	GlobalID         int64
	Date             time.Time
	SenderLocalID    int64
	SenderPts        int
	RecipientLocalID int64
	RecipientPts     int
	SelfChat         bool // true when sender == peer (no separate recipient copy)
}
