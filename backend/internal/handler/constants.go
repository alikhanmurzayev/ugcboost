package handler

import "github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"

// Cookie names. CookieRefreshToken lives in middleware (which handler depends
// on) to keep a single source of truth — RequestMeta extracts the cookie from
// the request, and handler builds the corresponding Set-Cookie value.
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
