package handler

import (
	"encoding/json"
	"net/http"
)

var version = "dev"

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// HealthCheck handles GET /healthz.
// This endpoint is intentionally outside the API envelope -- health checks
// are consumed by load balancers and monitoring, not by the frontend.
func (s *Server) HealthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(healthResponse{ //nolint:errcheck
		Status:  "ok",
		Version: version,
	})
}
