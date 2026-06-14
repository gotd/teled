package rpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gotd/td/tg"
)

func TestHelpGetTimezonesList(t *testing.T) {
	h := &Handler{}

	res, err := h.helpGetTimezonesList(context.Background(), 0)
	require.NoError(t, err)

	list, ok := res.(*tg.HelpTimezonesList)
	require.True(t, ok)
	require.NotEmpty(t, list.Timezones)

	// UTC is present with a zero offset.
	var utc *tg.Timezone

	for i := range list.Timezones {
		if list.Timezones[i].ID == "UTC" {
			utc = &list.Timezones[i]
		}
	}

	require.NotNil(t, utc)
	require.Equal(t, 0, utc.UtcOffset)

	// Echoing the returned hash yields "not modified".
	res2, err := h.helpGetTimezonesList(context.Background(), list.Hash)
	require.NoError(t, err)
	require.IsType(t, &tg.HelpTimezonesListNotModified{}, res2)
}
