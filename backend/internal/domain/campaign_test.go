package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCampaignTmaURL(t *testing.T) {
	t.Parallel()

	t.Run("empty input ok (legacy/draft)", func(t *testing.T) {
		t.Parallel()
		got, err := ValidateCampaignTmaURL("")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("whitespace-only input trims to empty", func(t *testing.T) {
		t.Parallel()
		got, err := ValidateCampaignTmaURL("   ")
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("too long returns CodeCampaignTmaURLTooLong", func(t *testing.T) {
		t.Parallel()
		raw := "https://x.kz/tz/" + strings.Repeat("a", 2049)
		_, err := ValidateCampaignTmaURL(raw)
		var ve *ValidationError
		require.ErrorAs(t, err, &ve)
		require.Equal(t, CodeCampaignTmaURLTooLong, ve.Code)
	})

	t.Run("missing scheme/host returns CodeInvalidTmaURL", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateCampaignTmaURL("/tz/abc_padding_secrettokenxx")
		require.ErrorIs(t, err, ErrInvalidTmaURL)
	})

	t.Run("bare token (not a URL) returns CodeInvalidTmaURL", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateCampaignTmaURL("abc_padding_secrettokenxx")
		require.ErrorIs(t, err, ErrInvalidTmaURL)
	})

	t.Run("last segment <16 returns CodeInvalidTmaURL", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateCampaignTmaURL("https://tma.ugcboost.kz/tz/short")
		require.ErrorIs(t, err, ErrInvalidTmaURL)
	})

	t.Run("last segment with non-URL-safe chars returns CodeInvalidTmaURL", func(t *testing.T) {
		t.Parallel()
		_, err := ValidateCampaignTmaURL("https://tma.ugcboost.kz/tz/abc_padding/secrettoken!@#")
		require.ErrorIs(t, err, ErrInvalidTmaURL)
	})

	t.Run("happy path returns trimmed value", func(t *testing.T) {
		t.Parallel()
		got, err := ValidateCampaignTmaURL("  https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx  ")
		require.NoError(t, err)
		require.Equal(t, "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx", got)
	})
}

func TestExtractSecretToken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty input → empty", "", ""},
		{"whitespace-only → empty", "   ", ""},
		{"trailing slash trimmed", "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx/", "abc_padding_secrettokenxx"},
		{"happy URL", "https://tma.ugcboost.kz/tz/abc_padding_secrettokenxx", "abc_padding_secrettokenxx"},
		{"unparseable input → empty", "://", ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, ExtractSecretToken(tc.in))
		})
	}
}

func TestSecretTokenRegex(t *testing.T) {
	t.Parallel()

	require.True(t, SecretTokenRegex().MatchString("abc_padding_secrettokenxx"))
	require.False(t, SecretTokenRegex().MatchString("short"))
	require.False(t, SecretTokenRegex().MatchString(""))
	require.False(t, SecretTokenRegex().MatchString("abc!def_padding_secret"))
}

func TestErrInvalidTmaURL_IsValidationError(t *testing.T) {
	t.Parallel()

	var ve *ValidationError
	require.True(t, errors.As(ErrInvalidTmaURL, &ve))
	require.Equal(t, CodeInvalidTmaURL, ve.Code)
}
