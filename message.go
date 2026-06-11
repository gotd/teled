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
