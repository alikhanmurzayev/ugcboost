package domain

import "testing"

func TestNormalizePhoneE164(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		out  string
	}{
		{"empty", "", ""},
		{"already E.164 KZ", "+77071234567", "+77071234567"},
		{"with spaces", "+7 707 123 45 67", "+77071234567"},
		{"with parens and dashes", "+7 (707) 123-45-67", "+77071234567"},
		{"leading 8", "87071234567", "+77071234567"},
		{"leading 7 without plus", "77071234567", "+77071234567"},
		{"leading +8", "+87071234567", "+77071234567"},
		{"non-digit junk", "phone: +7 707 1234567 ext.", "+77071234567"},
		{"only digits 11 starting 8", "8 707 123 45 67", "+77071234567"},
		{"no digits", "abc", "abc"},
		{"short — pass through", "12345", "12345"},
		{"international with +", "+14155552671", "+14155552671"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := NormalizePhoneE164(tc.in)
			if got != tc.out {
				t.Fatalf("NormalizePhoneE164(%q) = %q, want %q", tc.in, got, tc.out)
			}
		})
	}
}
