package domain

import (
	"testing"
	"time"
)

func TestFormatIssuedDate(t *testing.T) {
	t.Parallel()

	almaty, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		t.Fatalf("load Asia/Almaty: %v", err)
	}

	tests := []struct {
		name string
		in   time.Time
		loc  *time.Location
		want string
	}{
		{
			name: "Asia/Almaty 9 May 2026",
			in:   time.Date(2026, time.May, 9, 0, 0, 0, 0, almaty),
			loc:  almaty,
			want: "«9» мая 2026 г.",
		},
		{
			name: "January 1",
			in:   time.Date(2026, time.January, 1, 12, 0, 0, 0, almaty),
			loc:  almaty,
			want: "«1» января 2026 г.",
		},
		{
			name: "December 31",
			in:   time.Date(2027, time.December, 31, 23, 59, 0, 0, almaty),
			loc:  almaty,
			want: "«31» декабря 2027 г.",
		},
		{
			name: "UTC midnight 8 May converts to 9 May Almaty",
			in:   time.Date(2026, time.May, 8, 19, 0, 0, 0, time.UTC),
			loc:  almaty,
			want: "«9» мая 2026 г.",
		},
		{
			name: "nil loc falls back to UTC",
			in:   time.Date(2026, time.July, 4, 12, 0, 0, 0, time.UTC),
			loc:  nil,
			want: "«4» июля 2026 г.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FormatIssuedDate(tc.in, tc.loc)
			if got != tc.want {
				t.Fatalf("FormatIssuedDate = %q, want %q", got, tc.want)
			}
		})
	}
}
