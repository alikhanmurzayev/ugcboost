package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateVerificationCode(t *testing.T) {
	t.Parallel()

	t.Run("format matches UGC-NNNNNN", func(t *testing.T) {
		t.Parallel()

		for i := 0; i < 1000; i++ {
			code, err := GenerateVerificationCode()
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(code, VerificationCodePrefix), "missing prefix: %q", code)
			rest := code[len(VerificationCodePrefix):]
			require.Len(t, rest, VerificationCodeDigits, "wrong digit count: %q", code)
			for _, c := range rest {
				require.True(t, c >= '0' && c <= '9', "non-digit char in %q", code)
			}
		}
	})

	t.Run("varies across calls", func(t *testing.T) {
		t.Parallel()
		// 1000 draws over a 1M-element space — collision probability ~50%, so we
		// only assert that not every value collapsed onto the same code (which
		// would be a sign of a broken random source).
		seen := make(map[string]struct{}, 1000)
		for i := 0; i < 1000; i++ {
			code, err := GenerateVerificationCode()
			require.NoError(t, err)
			seen[code] = struct{}{}
		}
		require.Greater(t, len(seen), 100, "verification codes do not vary enough — random source likely broken")
	})
}

func TestParseVerificationCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		input  string
		want   string
		wantOk bool
	}{
		{"upper-case match", "Hi UGC-123456 thanks", "UGC-123456", true},
		{"lower-case normalised to upper", "code: ugc-987654 done", "UGC-987654", true},
		{"mixed case", "Code Ugc-111222 here", "UGC-111222", true},
		{"first match wins on multiple codes", "UGC-111111 then UGC-222222", "UGC-111111", true},
		{"surrounded by punctuation", "(UGC-555555).", "UGC-555555", true},
		{"empty input", "", "", false},
		{"no UGC token", "Hello world!", "", false},
		{"too few digits", "UGC-12345 nope", "", false},
		// Word-boundary regex rejects "UGC-1234567" — a 7-digit typo must
		// not silently extract some other 6-digit code from the prefix and
		// trigger a wrong verification.
		{"trailing digit beyond 6 rejects the match", "UGC-1234567 nope", "", false},
		{"letters in digits", "UGC-12A456 nope", "", false},
		{"missing dash", "UGC123456 nope", "", false},
		{"prefix only", "UGC- nope", "", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseVerificationCode(tc.input)
			require.Equal(t, tc.wantOk, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeInstagramHandle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already canonical", "aidana", "aidana"},
		{"upper-case", "AIDANA", "aidana"},
		{"mixed case", "AiDana", "aidana"},
		{"single leading at", "@aidana", "aidana"},
		{"multiple leading ats", "@@@aidana", "aidana"},
		{"trailing at stripped to mirror SQL backfill", "aidana@", "aidana"},
		{"both sides at", "@aidana@", "aidana"},
		{"surrounding whitespace", "   @Aidana   ", "aidana"},
		{"trailing whitespace only", "aidana  ", "aidana"},
		{"empty", "", ""},
		{"whitespace only", "    ", ""},
		{"only ats", "@@@", ""},
		{"with underscore and digits", "@User_42", "user_42"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, NormalizeInstagramHandle(tc.in))
		})
	}
}

func TestIsCreatorApplicationTransitionAllowed(t *testing.T) {
	t.Parallel()

	t.Run("verification → moderation allowed", func(t *testing.T) {
		t.Parallel()
		require.True(t, IsCreatorApplicationTransitionAllowed(
			CreatorApplicationStatusVerification,
			CreatorApplicationStatusModeration,
		))
	})

	// Identity transitions are explicitly disallowed — no row should map onto
	// itself in the state machine.
	t.Run("verification → verification disallowed", func(t *testing.T) {
		t.Parallel()
		require.False(t, IsCreatorApplicationTransitionAllowed(
			CreatorApplicationStatusVerification,
			CreatorApplicationStatusVerification,
		))
	})

	t.Run("moderation → verification disallowed", func(t *testing.T) {
		t.Parallel()
		require.False(t, IsCreatorApplicationTransitionAllowed(
			CreatorApplicationStatusModeration,
			CreatorApplicationStatusVerification,
		))
	})

	t.Run("moderation → approved allowed", func(t *testing.T) {
		t.Parallel()
		require.True(t, IsCreatorApplicationTransitionAllowed(
			CreatorApplicationStatusModeration,
			CreatorApplicationStatusApproved,
		))
	})

	t.Run("approved is terminal — outbound edges rejected", func(t *testing.T) {
		t.Parallel()
		require.False(t, IsCreatorApplicationTransitionAllowed(
			CreatorApplicationStatusApproved,
			CreatorApplicationStatusModeration,
		))
		require.False(t, IsCreatorApplicationTransitionAllowed(
			CreatorApplicationStatusApproved,
			CreatorApplicationStatusRejected,
		))
	})

	t.Run("verification → approved disallowed (must go through moderation)", func(t *testing.T) {
		t.Parallel()
		require.False(t, IsCreatorApplicationTransitionAllowed(
			CreatorApplicationStatusVerification,
			CreatorApplicationStatusApproved,
		))
	})

	t.Run("unknown statuses rejected", func(t *testing.T) {
		t.Parallel()
		require.False(t, IsCreatorApplicationTransitionAllowed("not_a_status", CreatorApplicationStatusModeration))
		require.False(t, IsCreatorApplicationTransitionAllowed(CreatorApplicationStatusVerification, "not_a_status"))
	})
}

func TestErrInvalidStatusTransition(t *testing.T) {
	t.Parallel()
	// Sentinel must be reachable via errors.Is — the service wraps it with
	// the from/to context but downstream code keys off the sentinel itself.
	require.Error(t, ErrInvalidStatusTransition)
	require.True(t, errors.Is(ErrInvalidStatusTransition, ErrInvalidStatusTransition))
}
