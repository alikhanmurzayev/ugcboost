package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTokenService() *TokenService {
	return NewTokenService("test-secret-key", 15*time.Minute, 168*time.Hour, 1*time.Hour)
}

// --- GenerateAccessToken + ValidateAccessToken ---

func TestAccessToken_CorrectClaims(t *testing.T) {
	t.Parallel()
	svc := newTestTokenService()
	token, err := svc.GenerateAccessToken("user-1", "admin")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	userID, role, err := svc.ValidateAccessToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-1", userID)
	assert.Equal(t, "admin", role)
}

func TestAccessToken_DifferentRoles(t *testing.T) {
	t.Parallel()
	svc := newTestTokenService()

	for _, role := range []string{"admin", "brand_manager"} {
		token, err := svc.GenerateAccessToken("u-1", role)
		require.NoError(t, err)

		_, gotRole, err := svc.ValidateAccessToken(token)
		require.NoError(t, err)
		assert.Equal(t, role, gotRole)
	}
}

func TestAccessToken_ExpiredToken(t *testing.T) {
	t.Parallel()
	svc := NewTokenService("secret", -1*time.Second, 168*time.Hour, 1*time.Hour) // negative expiry = already expired
	token, err := svc.GenerateAccessToken("u-1", "admin")
	require.NoError(t, err)

	_, _, err = svc.ValidateAccessToken(token)
	assert.Error(t, err)
}

func TestAccessToken_WrongSecret(t *testing.T) {
	t.Parallel()
	svc1 := NewTokenService("secret-A", 15*time.Minute, 168*time.Hour, 1*time.Hour)
	svc2 := NewTokenService("secret-B", 15*time.Minute, 168*time.Hour, 1*time.Hour)

	token, err := svc1.GenerateAccessToken("u-1", "admin")
	require.NoError(t, err)

	_, _, err = svc2.ValidateAccessToken(token)
	assert.Error(t, err)
}

func TestAccessToken_Garbage(t *testing.T) {
	t.Parallel()
	svc := newTestTokenService()
	_, _, err := svc.ValidateAccessToken("not.a.jwt")
	assert.Error(t, err)
}

func TestAccessToken_EmptyString(t *testing.T) {
	t.Parallel()
	svc := newTestTokenService()
	_, _, err := svc.ValidateAccessToken("")
	assert.Error(t, err)
}

// --- GenerateRefreshToken ---

func TestRefreshToken_UniqueAndHashed(t *testing.T) {
	t.Parallel()
	svc := newTestTokenService()

	raw1, hash1, exp1, err := svc.GenerateRefreshToken()
	require.NoError(t, err)
	raw2, hash2, _, err := svc.GenerateRefreshToken()
	require.NoError(t, err)

	assert.NotEqual(t, raw1, raw2, "tokens should be unique")
	assert.NotEqual(t, hash1, hash2, "hashes should be unique")
	assert.NotEqual(t, raw1, hash1, "raw should differ from hash")
	assert.True(t, exp1.After(time.Now()), "expiry should be in the future")
}

// --- GenerateResetToken ---

func TestResetToken_UniqueAndHashed(t *testing.T) {
	t.Parallel()
	svc := newTestTokenService()

	raw, hash, exp, err := svc.GenerateResetToken()
	require.NoError(t, err)

	assert.Len(t, raw, 64, "raw token should be 32 bytes hex-encoded")
	assert.NotEqual(t, raw, hash)
	assert.True(t, exp.After(time.Now()))
	assert.True(t, exp.Before(time.Now().Add(2*time.Hour)), "reset token should expire within 2 hours")
}

// --- HashToken ---

func TestHashToken_Deterministic(t *testing.T) {
	t.Parallel()
	h1 := HashToken("abc")
	h2 := HashToken("abc")
	assert.Equal(t, h1, h2)
}

func TestHashToken_DifferentInputs(t *testing.T) {
	t.Parallel()
	h1 := HashToken("abc")
	h2 := HashToken("def")
	assert.NotEqual(t, h1, h2)
}
