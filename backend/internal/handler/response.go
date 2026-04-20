package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// encodeJSON writes v as JSON and logs (but does not fail) encoding errors.
// All handler responses go through this helper so we never silently drop
// encoder failures with an errcheck bypass.
func encodeJSON(w http.ResponseWriter, r *http.Request, v any, log logger.Logger) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Error(r.Context(), "failed to encode response",
			"error", err,
			"method", r.Method,
			"path", r.URL.Path,
		)
	}
}

// respondJSON writes a JSON response with the given status code.
// The payload should be a typed API response struct (already contains Data field).
func respondJSON(w http.ResponseWriter, r *http.Request, status int, v any, log logger.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encodeJSON(w, r, v, log)
}

// respondError maps domain errors to HTTP responses.
func respondError(w http.ResponseWriter, r *http.Request, err error, log logger.Logger) {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		writeError(w, r, http.StatusUnprocessableEntity, ve.Code, ve.Message, log)
		return
	}

	var be *domain.BusinessError
	if errors.As(err, &be) {
		writeError(w, r, http.StatusConflict, be.Code, be.Message, log)
		return
	}

	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		writeError(w, r, http.StatusNotFound, domain.CodeNotFound, "Resource not found", log)
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, r, http.StatusForbidden, domain.CodeForbidden, "Access denied", log)
	case errors.Is(err, domain.ErrUnauthorized):
		writeError(w, r, http.StatusUnauthorized, domain.CodeUnauthorized, "Authentication required", log)
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, r, http.StatusConflict, domain.CodeConflict, "Resource already exists", log)
	default:
		log.Error(r.Context(), "unexpected error", "error", err, "path", r.URL.Path)
		writeError(w, r, http.StatusInternalServerError, domain.CodeInternal, "Internal server error", log)
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, log logger.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	encodeJSON(w, r, api.ErrorResponse{
		Error: api.APIError{Code: code, Message: message},
	}, log)
}
