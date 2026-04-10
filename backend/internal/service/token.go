package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenService handles JWT access tokens and opaque refresh/reset tokens.
type TokenService struct {
	secret        []byte
	accessExpiry  time.Duration
	refreshExpiry time.Duration
	resetExpiry   time.Duration
}

// NewTokenService creates a new token service.
func NewTokenService(secret string, accessExpiry, refreshExpiry, resetExpiry time.Duration) *TokenService {
	return &TokenService{
		secret:        []byte(secret),
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
		resetExpiry:   resetExpiry,
	}
}

// GenerateAccessToken creates a short-lived JWT with user_id and role.
func (s *TokenService) GenerateAccessToken(userID, role string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"iat":  now.Unix(),
		"exp":  now.Add(s.accessExpiry).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateAccessToken parses and validates a JWT, returning userID and role.
func (s *TokenService) ValidateAccessToken(tokenStr string) (string, string, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", "", fmt.Errorf("invalid token claims")
	}

	userID, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	if userID == "" || role == "" {
		return "", "", fmt.Errorf("missing claims")
	}

	return userID, role, nil
}

// GenerateRefreshToken creates a random opaque token and returns (raw, hash).
func (s *TokenService) GenerateRefreshToken() (raw string, hash string, expiresAt time.Time, err error) {
	return generateOpaqueToken(s.refreshExpiry)
}

// GenerateResetToken creates a random opaque token for password reset.
func (s *TokenService) GenerateResetToken() (raw string, hash string, expiresAt time.Time, err error) {
	return generateOpaqueToken(s.resetExpiry)
}

// HashToken returns the SHA-256 hash of a raw token.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func generateOpaqueToken(expiry time.Duration) (string, string, time.Time, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	raw := hex.EncodeToString(b)
	hash := HashToken(raw)
	return raw, hash, time.Now().Add(expiry), nil
}
