package handler

import "net/http"

// HandleParamError handles parameter parsing errors from the generated wrapper.
func HandleParamError(w http.ResponseWriter, _ *http.Request, err error) {
	writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
}
