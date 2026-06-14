package rpc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/tg"

	"github.com/gotd/teled"
)

func TestToTGUserStatus(t *testing.T) {
	// Self is reported online.
	self := toTGUser(teled.User{ID: 1, FirstName: "Ada"}, true)
	require.IsType(t, &tg.UserStatusOnline{}, self.Status)

	// Others are "recently", not UserStatusEmpty (which renders as
	// "last seen a long time ago").
	other := toTGUser(teled.User{ID: 2, FirstName: "Bob"}, false)
	require.IsType(t, &tg.UserStatusRecently{}, other.Status)
}
