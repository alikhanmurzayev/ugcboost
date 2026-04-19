package handler

import (
	"net/http"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// HealthCheck handles GET /healthz.
// This endpoint is intentionally outside the API envelope -- health checks
// are consumed by load balancers and monitoring, not by the frontend.
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	encodeJSON(w, r, healthResponse{
		Status:  "ok",
		Version: s.version,
	})
}
