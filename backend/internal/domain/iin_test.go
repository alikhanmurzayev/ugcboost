package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateIIN(t *testing.T) {
	t.Parallel()

	// Reference vectors below use the published two-pass RK IIN algorithm.
	// "950515312348" — born 1995-05-15, single-pass checksum (mod=8).
	// "990101310105" — born 1999-01-01, two-pass checksum (first pass yields
	// mod=10, second pass yields mod=5).

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateIIN("")
		require.ErrorIs(t, err, ErrIINFormat)
	})

	t.Run("too short", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateIIN("123")
		require.ErrorIs(t, err, ErrIINFormat)
	})

	t.Run("too long", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateIIN("9505153123480")
		require.ErrorIs(t, err, ErrIINFormat)
	})

	t.Run("non digit characters", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateIIN("95051531234A")
		require.ErrorIs(t, err, ErrIINFormat)
	})

	t.Run("invalid checksum single pass", func(t *testing.T) {
		t.Parallel()
		// Same as the valid vector but with the control digit flipped.
		_, err := ValidateIIN("950515312349")
		require.ErrorIs(t, err, ErrIINChecksum)
	})

	t.Run("invalid checksum two pass", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateIIN("990101310106")
		require.ErrorIs(t, err, ErrIINChecksum)
	})

	t.Run("invalid century byte", func(t *testing.T) {
		t.Parallel()
		// Take the valid vector and replace century byte 3 with 9 — that
		// breaks the checksum but the century check fires first only if
		// checksum already matches; pick a digit that keeps checksum legit.
		// Simpler: run iinYear directly via ValidateIIN with a malformed
		// century — we craft one where checksum still validates.
		// d[0..5]=950515, d[6]=9, d[7..10] must satisfy first-pass mod
		// at the right value. Use brute-search alternative: assert via
		// the helper in isolation when full vector is impractical.
		_, err := iinYear(95, 9)
		require.ErrorIs(t, err, ErrIINCentury)
	})

	t.Run("invalid birth date february 30", func(t *testing.T) {
		t.Parallel()
		// 950230 (Feb 30) with century 3 — pick checksum-valid version.
		// d[0..6] = 9,5,0,2,3,0,3
		// First-pass partial: 9+10+0+8+15+0+21 = 63
		// Want overall mod with d[7..10]=0,0,0,0 → 63%11 = 8.
		// So control digit = 8 → "950230300008".
		_, err := ValidateIIN("950230300008")
		require.ErrorIs(t, err, ErrIINBirthDate)
	})

	t.Run("valid single pass checksum", func(t *testing.T) {
		t.Parallel()
		birth, err := ValidateIIN("950515312348")
		require.NoError(t, err)
		require.Equal(t, time.Date(1995, time.May, 15, 0, 0, 0, 0, time.UTC), birth)
	})

	t.Run("valid two pass checksum", func(t *testing.T) {
		t.Parallel()
		birth, err := ValidateIIN("990101310105")
		require.NoError(t, err)
		require.Equal(t, time.Date(1999, time.January, 1, 0, 0, 0, 0, time.UTC), birth)
	})

	t.Run("century 5 maps to 2000s", func(t *testing.T) {
		t.Parallel()
		// 050101 5 0000 X — compute checksum.
		// Sum = 0*1+5*2+0*3+1*4+0*5+1*6+5*7+0*8+0*9+0*10+0*11
		//     = 0+10+0+4+0+6+35+0+0+0+0 = 55, mod=0 → control=0.
		birth, err := ValidateIIN("050101500000")
		require.NoError(t, err)
		require.Equal(t, 2005, birth.Year())
	})
}

func TestAgeYearsOn(t *testing.T) {
	t.Parallel()

	birth := time.Date(2000, time.June, 15, 0, 0, 0, 0, time.UTC)

	t.Run("anniversary not yet reached", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, time.June, 14, 12, 0, 0, 0, time.UTC)
		require.Equal(t, 25, AgeYearsOn(birth, now))
	})

	t.Run("anniversary reached", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
		require.Equal(t, 26, AgeYearsOn(birth, now))
	})

	t.Run("anniversary passed", func(t *testing.T) {
		t.Parallel()
		now := time.Date(2026, time.June, 16, 0, 0, 0, 0, time.UTC)
		require.Equal(t, 26, AgeYearsOn(birth, now))
	})
}

func TestEnsureAdult(t *testing.T) {
	t.Parallel()

	t.Run("under MinCreatorAge by one day", func(t *testing.T) {
		t.Parallel()
		// Anchored on MinCreatorAge so the test stays correct if the constant
		// changes: birthday is one day after `now`, so the applicant is one
		// day shy of MinCreatorAge.
		birth := time.Date(2005, time.April, 21, 0, 0, 0, 0, time.UTC)
		now := time.Date(2005+MinCreatorAge, time.April, 20, 0, 0, 0, 0, time.UTC)
		err := EnsureAdult(birth, now)
		require.ErrorIs(t, err, ErrIINUnderAge)
	})

	t.Run("exactly MinCreatorAge today", func(t *testing.T) {
		t.Parallel()
		// Anchored on MinCreatorAge — anniversary lands on `now`, so the
		// applicant turns MinCreatorAge exactly today.
		birth := time.Date(2005, time.April, 20, 0, 0, 0, 0, time.UTC)
		now := time.Date(2005+MinCreatorAge, time.April, 20, 0, 0, 0, 0, time.UTC)
		require.NoError(t, EnsureAdult(birth, now))
	})

	t.Run("comfortably adult", func(t *testing.T) {
		t.Parallel()
		birth := time.Date(1990, time.January, 1, 0, 0, 0, 0, time.UTC)
		now := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC)
		require.NoError(t, EnsureAdult(birth, now))
	})
}

func TestSentinels(t *testing.T) {
	t.Parallel()
	// Guard against accidental sentinel collapsing — each sentinel must
	// remain distinct so callers can branch on errors.Is reliably.
	require.False(t, errors.Is(ErrIINFormat, ErrIINChecksum))
	require.False(t, errors.Is(ErrIINChecksum, ErrIINBirthDate))
	require.False(t, errors.Is(ErrIINBirthDate, ErrIINCentury))
	require.False(t, errors.Is(ErrIINCentury, ErrIINUnderAge))
}
