package rpc

import (
	"context"
	"time"

	"github.com/gotd/td/tg"
)

// timezoneIDs is a curated set of IANA zones surfaced to clients (e.g. for the
// "send scheduled" / birthday timezone pickers). UTC offsets are computed live
// so they track DST.
const zoneUTC = "UTC"

var timezoneIDs = []struct{ id, name string }{
	{zoneUTC, zoneUTC},
	{"Pacific/Honolulu", "Honolulu"},
	{"America/Anchorage", "Anchorage"},
	{"America/Los_Angeles", "Los Angeles"},
	{"America/Denver", "Denver"},
	{"America/Chicago", "Chicago"},
	{"America/New_York", "New York"},
	{"America/Sao_Paulo", "São Paulo"},
	{"Atlantic/Reykjavik", "Reykjavik"},
	{"Europe/London", "London"},
	{"Europe/Paris", "Paris"},
	{"Europe/Berlin", "Berlin"},
	{"Europe/Madrid", "Madrid"},
	{"Europe/Rome", "Rome"},
	{"Europe/Kyiv", "Kyiv"},
	{"Europe/Istanbul", "Istanbul"},
	{"Europe/Moscow", "Moscow"},
	{"Asia/Dubai", "Dubai"},
	{"Asia/Karachi", "Karachi"},
	{"Asia/Kolkata", "Kolkata"},
	{"Asia/Dhaka", "Dhaka"},
	{"Asia/Bangkok", "Bangkok"},
	{"Asia/Shanghai", "Shanghai"},
	{"Asia/Singapore", "Singapore"},
	{"Asia/Tokyo", "Tokyo"},
	{"Asia/Seoul", "Seoul"},
	{"Australia/Sydney", "Sydney"},
	{"Pacific/Auckland", "Auckland"},
}

// helpGetTimezonesList returns the supported timezones with live UTC offsets.
// The hash lets clients cache: a matching request hash yields "not modified".
func (h *Handler) helpGetTimezonesList(_ context.Context, hash int) (tg.HelpTimezonesListClass, error) {
	now := time.Now()
	zones := make([]tg.Timezone, 0, len(timezoneIDs))

	for _, z := range timezoneIDs {
		offset := 0
		if loc, err := time.LoadLocation(z.id); err == nil {
			_, offset = now.In(loc).Zone()
		}

		zones = append(zones, tg.Timezone{ID: z.id, Name: z.name, UtcOffset: offset})
	}

	listHash := timezonesHash(zones)
	if hash == listHash {
		return &tg.HelpTimezonesListNotModified{}, nil
	}

	return &tg.HelpTimezonesList{Timezones: zones, Hash: listHash}, nil
}

// timezonesHash is a stable hash over the timezone set so clients can detect
// changes (including DST offset shifts).
func timezonesHash(zones []tg.Timezone) int {
	var h uint32

	for _, z := range zones {
		for _, b := range []byte(z.ID) {
			h = h*31 + uint32(b)
		}

		h = h*31 + uint32(z.UtcOffset) // #nosec G115 -- hash mixing.
	}

	return int(h & 0x7fffffff)
}
