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
// All error responses go through this helper so we never silently drop
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

// respondError maps domain errors to HTTP responses. It is bound to the
// Server's logger and plugged into strict-server's ResponseErrorHandlerFunc /
// RequestErrorHandlerFunc — keeping domain → HTTP translation in one place.
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
	case errors.Is(err, domain.ErrCreatorApplicationNotFound):
		writeError(w, r, http.StatusNotFound, domain.CodeNotFound, "Заявка не найдена", log)
	case errors.Is(err, domain.ErrCreatorApplicationSocialNotFound):
		writeError(w, r, http.StatusNotFound, domain.CodeCreatorApplicationSocialNotFound,
			"Соцсеть не найдена в этой заявке", log)
	case errors.Is(err, domain.ErrCreatorApplicationSocialAlreadyVerified):
		writeError(w, r, http.StatusConflict, domain.CodeCreatorApplicationSocialAlreadyVerified,
			"Эта соцсеть уже верифицирована", log)
	case errors.Is(err, domain.ErrCreatorApplicationNotInVerification):
		writeError(w, r, http.StatusUnprocessableEntity, domain.CodeCreatorApplicationNotInVerification,
			"Заявка уже не на этапе верификации", log)
	case errors.Is(err, domain.ErrCreatorApplicationTelegramNotLinked):
		writeError(w, r, http.StatusUnprocessableEntity, domain.CodeCreatorApplicationTelegramNotLinked,
			"Креатор не привязал Telegram-бота — попросите его открыть бот по deep-link и повторите", log)
	case errors.Is(err, domain.ErrCreatorApplicationNotRejectable):
		writeError(w, r, http.StatusUnprocessableEntity, domain.CodeCreatorApplicationNotRejectable,
			"Заявку нельзя отклонить в текущем статусе. Допустимые статусы для отклонения — verification или moderation.", log)
	case errors.Is(err, domain.ErrCreatorApplicationNotApprovable):
		writeError(w, r, http.StatusUnprocessableEntity, domain.CodeCreatorApplicationNotApprovable,
			"Заявку нельзя одобрить в текущем статусе. Допустимый статус для одобрения — moderation.", log)
	case errors.Is(err, domain.ErrCreatorAlreadyExists):
		writeError(w, r, http.StatusUnprocessableEntity, domain.CodeCreatorAlreadyExists,
			"Креатор с таким ИИН уже существует. Сверьте данные с реестром или удалите дубль креатора.", log)
	case errors.Is(err, domain.ErrCreatorTelegramAlreadyTaken):
		writeError(w, r, http.StatusUnprocessableEntity, domain.CodeCreatorTelegramAlreadyTaken,
			"Этот Telegram-аккаунт уже привязан к другому креатору. Освободите его или попросите креатора сменить аккаунт.", log)
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
