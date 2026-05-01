package handler

import "github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"

const CookieRefreshToken = middleware.CookieRefreshToken

// HTTP header names.
const (
	HeaderAuthorization = "Authorization"
	HeaderXForwardedFor = "X-Forwarded-For"
	HeaderXRealIP       = "X-Real-IP"
)

// Auth scheme constants.
const (
	AuthSchemeBearer = "bearer"
)

// Input validation constants.
const (
	// minPasswordLength mirrors the `minLength: 6` constraint on password/newPassword
	// fields in the OpenAPI contract. Enforced in handler because the generated
	// chi wrapper does not validate request bodies against the OpenAPI schema.
	minPasswordLength = 6
)
