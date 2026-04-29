package telegram_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram"
)

func TestDefaultMessages(t *testing.T) {
	t.Parallel()

	m := telegram.DefaultMessages()
	require.Equal(t, telegram.MessageLinkSuccess, m.LinkSuccess())
	require.Equal(t, telegram.MessageStartNoPayload, m.StartNoPayload())
	require.Equal(t, telegram.MessageInvalidPayload, m.InvalidPayload())
	require.Equal(t, telegram.MessageApplicationNotFound, m.ApplicationNotFound())
	require.Equal(t, telegram.MessageApplicationNotActive, m.ApplicationNotActive())
	require.Equal(t, telegram.MessageApplicationAlreadyLinked, m.ApplicationAlreadyLinked())
	require.Equal(t, telegram.MessageAccountAlreadyLinked, m.AccountAlreadyLinked())
	require.Equal(t, telegram.MessageFallback, m.Fallback())
	require.Equal(t, telegram.MessageInternalError, m.InternalError())
}
