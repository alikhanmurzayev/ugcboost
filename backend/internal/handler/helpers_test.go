package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

func expectHandlerUnexpectedErrorLog(log *logmocks.MockLogger, path string) {
	log.EXPECT().Error(mock.Anything, "unexpected error", mock.MatchedBy(func(args []any) bool {
		return len(args) == 4 && args[0] == "error" && args[2] == "path" && args[3] == path
	})).Once()
}

// newTestRouter mirrors prod wiring: strict adapter behind chi, RequestMeta for
// User-Agent, RefreshCookie for the /auth/refresh and /auth/logout cookie path.
func newTestRouter(t *testing.T, s *Server) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	r.Use(middleware.RequestMeta)
	r.Use(middleware.RefreshCookie)
	api.HandlerWithOptions(NewStrictAPIHandler(s), api.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: HandleParamError(logmocks.NewMockLogger(t)),
	})
	return r
}

// doJSON marshals body (if not nil), sends it through the router, unmarshals
// the response into a fresh value of Resp, and returns the raw recorder so
// callers can assert on cookies/headers.
func doJSON[Resp any](t *testing.T, router http.Handler, method, path string, body any, mutate ...func(*http.Request)) (*httptest.ResponseRecorder, Resp) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}

	r := httptest.NewRequest(method, path, reader)
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, m := range mutate {
		m(r)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)

	var resp Resp
	if w.Body.Len() > 0 && w.Body.Bytes()[0] != '\x00' {
		// Tolerate responses that omit the envelope when Resp is unused.
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w, resp
}

// withRole injects the given userID and role into the request context the
// same way the auth middleware would in production. Use it when a handler
// test needs an authenticated caller with a specific role.
func withRole(userID string, role api.UserRole) func(*http.Request) {
	return func(r *http.Request) {
		ctx := context.WithValue(r.Context(), middleware.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, role)
		*r = *r.WithContext(ctx)
	}
}

// withRefreshCookie attaches a refresh-token cookie so middleware.RefreshCookie
// surfaces it to the handler via context — only takes effect for whitelisted
// routes (/auth/refresh, /auth/logout).
func withRefreshCookie(value string) func(*http.Request) {
	return func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: middleware.CookieRefreshToken, Value: value})
	}
}

// doPDF sends a request with raw bytes as `application/pdf`. Used for the
// PUT /campaigns/{id}/contract-template tests where the body is a PDF
// rather than JSON. Decodes the JSON response body into Resp on the way out.
func doPDF[Resp any](t *testing.T, router http.Handler, method, path string, body []byte, mutate ...func(*http.Request)) (*httptest.ResponseRecorder, Resp) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	r := httptest.NewRequest(method, path, reader)
	r.Header.Set("Content-Type", "application/pdf")
	if body != nil {
		r.ContentLength = int64(len(body))
	}
	for _, m := range mutate {
		m(r)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)

	var resp Resp
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
	}
	return w, resp
}
