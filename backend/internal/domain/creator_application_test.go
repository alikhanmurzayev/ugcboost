package domain

import (
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
