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
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
)

// newTestRouter registers the given Server behind the generated chi wrapper.
// The returned router accepts requests exactly like the production router
// (same ServerInterfaceWrapper, same param parsing), so unit tests exercise
// the full handler contract instead of calling methods directly.
func newTestRouter(t *testing.T, s *Server) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	api.HandlerWithOptions(s, api.ChiServerOptions{
		BaseRouter:       r,
		ErrorHandlerFunc: HandleParamError,
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
