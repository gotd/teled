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
		{"9996641234", "44444"},
		{"9996656617", "55555"}, // DC 5 test account
		{"9996691234", "99999"},
		// DC digit 0 is not a test account; falls back to the dev code.
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

func TestNormalizePhone(t *testing.T) {
	for _, tt := range []struct {
		phone string
		want  string
	}{
		// Display-formatted strings collapse to bare digits.
		{"+ 9 996610000", "9996610000"},
		{"+9996610000", "9996610000"},
		{"9996610000", "9996610000"},
		{"+99966 1 0000", "9996610000"},
		{"+1 (337) 55-69", "13375569"}, // punctuation and spaces stripped
		{"", ""},
	} {
		if got := normalizePhone(tt.phone); got != tt.want {
			t.Errorf("normalizePhone(%q) = %q, want %q", tt.phone, got, tt.want)
		}
	}

	// A formatted test number must normalize so that test-account detection and
	// code derivation see the raw digits (regression for PHONE_CODE_EXPIRED).
	if got := phoneCode(normalizePhone("+ 9 996610000")); got != "11111" {
		t.Errorf("phoneCode(normalizePhone(...)) = %q, want %q", got, "11111")
	}
}
