package service

import "sync"

// ResetTokenNotifier is called when a password-reset token is generated.
// Implementations can store the raw token for test retrieval.
type ResetTokenNotifier interface {
	OnResetToken(email, rawToken string)
}

// InMemoryResetTokenStore stores the most recent raw reset token per email.
// Used only when ENABLE_TEST_ENDPOINTS=true.
type InMemoryResetTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]string // email -> raw token
}

// NewInMemoryResetTokenStore creates a new store.
func NewInMemoryResetTokenStore() *InMemoryResetTokenStore {
	return &InMemoryResetTokenStore{tokens: make(map[string]string)}
}

// OnResetToken stores the raw token for the given email.
func (s *InMemoryResetTokenStore) OnResetToken(email, rawToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[email] = rawToken
}

// GetToken returns the stored raw token for the given email.
func (s *InMemoryResetTokenStore) GetToken(email string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[email]
	return t, ok
}
