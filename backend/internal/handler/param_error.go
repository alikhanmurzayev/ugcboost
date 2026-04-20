package handler

import (
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
)

// HandleParamError returns a chi ErrorHandlerFunc that reports parameter
// parsing errors from the generated wrapper as HTTP 400.
func HandleParamError(log logger.Logger) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		writeError(w, r, http.StatusBadRequest, domain.CodeValidation, err.Error(), log)
	}
}
