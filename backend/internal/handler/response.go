package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := domain.APIResponse{Data: data}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

// respondError maps domain errors to HTTP responses.
func respondError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusUnprocessableEntity, ve.Code, ve.Message)
		return
	}

	var be *domain.BusinessError
	if errors.As(err, &be) {
		writeError(w, http.StatusConflict, be.Code, be.Message)
		return
	}

	switch {
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, domain.CodeNotFound, "Resource not found")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, domain.CodeForbidden, "Access denied")
	case errors.Is(err, domain.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, domain.CodeUnauthorized, "Authentication required")
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, http.StatusConflict, domain.CodeConflict, "Resource already exists")
	default:
		slog.Error("unexpected error", "error", err, "path", r.URL.Path)
		writeError(w, http.StatusInternalServerError, domain.CodeInternal, "Internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := domain.APIResponse{
		Error: &domain.APIError{Code: code, Message: message},
	}
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}
