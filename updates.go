package teled

import "time"

// State is an account's update sequence state.
type State struct {
	Pts         int
	Qts         int
	Seq         int
	Date        time.Time
	UnreadCount int
}

// UpdateLogEntry is one durable update in an account's log, used to build
// updates.getDifference responses.
type UpdateLogEntry struct {
	Pts      int
	PtsCount int
	Type     string
	GlobalID *int64
	Extra    []byte
	Date     time.Time
}
