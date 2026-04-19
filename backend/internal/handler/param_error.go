package handler

import (
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// HandleParamError handles parameter parsing errors from the generated wrapper.
func HandleParamError(w http.ResponseWriter, r *http.Request, err error) {
	writeError(w, r, http.StatusBadRequest, domain.CodeValidation, err.Error())
}
