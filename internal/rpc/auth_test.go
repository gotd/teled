package rpc

import "testing"

func TestPhoneCode(t *testing.T) {
	for _, tt := range []struct {
		phone string
		want  string
	}{
		// Telegram test accounts: code is the DC digit repeated five times.
		{"9996621234", "22222"},
		{"+9996621234", "22222"},
		{"9996610000", "11111"}, // 99966 + DC 1 + 0000
		{"9996631111", "33333"},
		// DC digit out of the 1-3 test range falls back to the dev code.
		{"9996641234", devPhoneCode},
		{"9996601234", devPhoneCode},
		// Non-test numbers always get the dev code.
		{"+19998887766", devPhoneCode},
		{"1337", devPhoneCode},
		{"", devPhoneCode},
	} {
		if got := phoneCode(tt.phone); got != tt.want {
			t.Errorf("phoneCode(%q) = %q, want %q", tt.phone, got, tt.want)
		}
	}
}
