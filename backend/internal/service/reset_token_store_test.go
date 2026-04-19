package service

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInMemoryResetTokenStore(t *testing.T) {
	t.Parallel()

	t.Run("empty store returns false", func(t *testing.T) {
		t.Parallel()
		store := NewInMemoryResetTokenStore()
		token, ok := store.GetToken("nobody@example.com")
		require.False(t, ok)
		require.Empty(t, token)
	})

	t.Run("set then get returns stored token", func(t *testing.T) {
		t.Parallel()
		store := NewInMemoryResetTokenStore()
		store.OnResetToken("alice@example.com", "raw-token-1")

		token, ok := store.GetToken("alice@example.com")
		require.True(t, ok)
		require.Equal(t, "raw-token-1", token)
	})

	t.Run("set overwrites previous token for same email", func(t *testing.T) {
		t.Parallel()
		store := NewInMemoryResetTokenStore()
		store.OnResetToken("alice@example.com", "first")
		store.OnResetToken("alice@example.com", "second")

		token, ok := store.GetToken("alice@example.com")
		require.True(t, ok)
		require.Equal(t, "second", token)
	})

	t.Run("tokens are isolated per email", func(t *testing.T) {
		t.Parallel()
		store := NewInMemoryResetTokenStore()
		store.OnResetToken("alice@example.com", "alice-token")
		store.OnResetToken("bob@example.com", "bob-token")

		alice, ok := store.GetToken("alice@example.com")
		require.True(t, ok)
		require.Equal(t, "alice-token", alice)

		bob, ok := store.GetToken("bob@example.com")
		require.True(t, ok)
		require.Equal(t, "bob-token", bob)
	})
}

func TestInMemoryResetTokenStore_Concurrency(t *testing.T) {
	t.Parallel()
	// Race detector must not flag any concurrent access.
	store := NewInMemoryResetTokenStore()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := range goroutines {
		i := i
		go func() {
			defer wg.Done()
			store.OnResetToken(fmt.Sprintf("u%d@example.com", i), fmt.Sprintf("token-%d", i))
		}()
		go func() {
			defer wg.Done()
			_, _ = store.GetToken(fmt.Sprintf("u%d@example.com", i))
		}()
	}
	wg.Wait()
}
