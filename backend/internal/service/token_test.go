package service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func newTestTokenService() *TokenService {
	return NewTokenService("test-secret-key", 15*time.Minute, 168*time.Hour, 1*time.Hour)
}

func TestTokenService_GenerateAccessToken(t *testing.T) {
	t.Parallel()

	t.Run("correct claims", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()
		token, err := svc.GenerateAccessToken("user-1", "admin")
		require.NoError(t, err)
		require.NotEmpty(t, token)

		userID, role, err := svc.ValidateAccessToken(token)
		require.NoError(t, err)
		require.Equal(t, "user-1", userID)
		require.Equal(t, "admin", role)
	})

	t.Run("different roles", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()

		for _, role := range []string{"admin", "brand_manager"} {
			token, err := svc.GenerateAccessToken("u-1", role)
			require.NoError(t, err)

			_, gotRole, err := svc.ValidateAccessToken(token)
			require.NoError(t, err)
			require.Equal(t, role, gotRole)
		}
	})

	t.Run("claims content parses sub, role and exp independently", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()
		before := time.Now()
		token, err := svc.GenerateAccessToken("user-42", "admin")
		require.NoError(t, err)

		claims := jwt.MapClaims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(_ *jwt.Token) (any, error) {
			return []byte("test-secret-key"), nil
		})
		require.NoError(t, err)
		require.True(t, parsed.Valid)

		sub, err := claims.GetSubject()
		require.NoError(t, err)
		require.Equal(t, "user-42", sub)

		require.Equal(t, "admin", claims["role"])

		expFloat, ok := claims["exp"].(float64)
		require.True(t, ok, "exp claim should be numeric")
		exp := time.Unix(int64(expFloat), 0)
		require.True(t, exp.After(before.Add(14*time.Minute)), "exp should be ~15m in the future")
		require.True(t, exp.Before(before.Add(16*time.Minute)), "exp should be ~15m in the future")
	})
}

func TestTokenService_ValidateAccessToken(t *testing.T) {
	t.Parallel()

	t.Run("expired token", func(t *testing.T) {
		t.Parallel()
		svc := NewTokenService("secret", -1*time.Second, 168*time.Hour, 1*time.Hour) // negative expiry = already expired
		token, err := svc.GenerateAccessToken("u-1", "admin")
		require.NoError(t, err)

		_, _, err = svc.ValidateAccessToken(token)
		require.Error(t, err)
	})

	t.Run("wrong secret", func(t *testing.T) {
		t.Parallel()
		svc1 := NewTokenService("secret-A", 15*time.Minute, 168*time.Hour, 1*time.Hour)
		svc2 := NewTokenService("secret-B", 15*time.Minute, 168*time.Hour, 1*time.Hour)

		token, err := svc1.GenerateAccessToken("u-1", "admin")
		require.NoError(t, err)

		_, _, err = svc2.ValidateAccessToken(token)
		require.Error(t, err)
	})

	t.Run("garbage", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()
		_, _, err := svc.ValidateAccessToken("not.a.jwt")
		require.Error(t, err)
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()
		_, _, err := svc.ValidateAccessToken("")
		require.Error(t, err)
	})

	t.Run("token missing sub claim returns missing claims error", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()
		claims := jwt.MapClaims{
			"role": "admin",
			"exp":  time.Now().Add(time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		str, err := token.SignedString([]byte("test-secret-key"))
		require.NoError(t, err)

		_, _, err = svc.ValidateAccessToken(str)
		require.ErrorContains(t, err, "missing claims")
	})

	t.Run("token missing role claim returns missing claims error", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()
		claims := jwt.MapClaims{
			"sub": "u-1",
			"exp": time.Now().Add(time.Hour).Unix(),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		str, err := token.SignedString([]byte("test-secret-key"))
		require.NoError(t, err)

		_, _, err = svc.ValidateAccessToken(str)
		require.ErrorContains(t, err, "missing claims")
	})
}

func TestTokenService_GenerateRefreshToken(t *testing.T) {
	t.Parallel()

	t.Run("unique and hashed", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()

		raw1, hash1, exp1, err := svc.GenerateRefreshToken()
		require.NoError(t, err)
		raw2, hash2, _, err := svc.GenerateRefreshToken()
		require.NoError(t, err)

		require.NotEqual(t, raw1, raw2, "tokens should be unique")
		require.NotEqual(t, hash1, hash2, "hashes should be unique")
		require.NotEqual(t, raw1, hash1, "raw should differ from hash")
		require.True(t, exp1.After(time.Now()), "expiry should be in the future")
	})
}

func TestTokenService_GenerateResetToken(t *testing.T) {
	t.Parallel()

	t.Run("unique and hashed", func(t *testing.T) {
		t.Parallel()
		svc := newTestTokenService()

		raw, hash, exp, err := svc.GenerateResetToken()
		require.NoError(t, err)

		require.Len(t, raw, 64, "raw token should be 32 bytes hex-encoded")
		require.NotEqual(t, raw, hash)
		require.True(t, exp.After(time.Now()))
		require.True(t, exp.Before(time.Now().Add(2*time.Hour)), "reset token should expire within 2 hours")
	})
}

func TestTokenService_HashToken(t *testing.T) {
	t.Parallel()

	t.Run("deterministic", func(t *testing.T) {
		t.Parallel()
		h1 := HashToken("abc")
		h2 := HashToken("abc")
		require.Equal(t, h1, h2)
	})

	t.Run("different inputs", func(t *testing.T) {
		t.Parallel()
		h1 := HashToken("abc")
		h2 := HashToken("def")
		require.NotEqual(t, h1, h2)
	})
}
