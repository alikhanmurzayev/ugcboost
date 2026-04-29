package telegram_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram"
)

// TestDefaultMessages enforces invariants on the canonical reply texts: every
// message must be non-empty and start with the expected greeting/cue so that
// a copy-edit removing the greeting (or accidentally leaving an empty
// message) breaks here instead of in production.
func TestDefaultMessages(t *testing.T) {
	t.Parallel()

	m := telegram.DefaultMessages()

	cases := []struct {
		name string
		got  string
	}{
		{"LinkSuccess", m.LinkSuccess()},
		{"StartNoPayload", m.StartNoPayload()},
		{"InvalidPayload", m.InvalidPayload()},
		{"ApplicationNotFound", m.ApplicationNotFound()},
		{"ApplicationNotActive", m.ApplicationNotActive()},
		{"ApplicationAlreadyLinked", m.ApplicationAlreadyLinked()},
		{"AccountAlreadyLinked", m.AccountAlreadyLinked()},
		{"Fallback", m.Fallback()},
		{"InternalError", m.InternalError()},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			require.NotEmpty(t, c.got, "%s must be non-empty", c.name)
			require.True(t, len(strings.TrimSpace(c.got)) > 20,
				"%s must be a full sentence, got %q", c.name, c.got)
		})
	}

	// Spot-check anchor phrases so a careless copy edit cannot silently
	// remove the brand reference or the canonical opening greeting.
	require.True(t, strings.HasPrefix(m.LinkSuccess(), "Здравствуйте"))
	require.Contains(t, m.LinkSuccess(), "Заявка успешно связана")
	require.Contains(t, m.StartNoPayload(), "ugcboost.kz")
	require.Contains(t, m.Fallback(), "/start")
}
