package handler

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
)

// HealthCheck handles GET /healthz.
func (s *Server) HealthCheck(_ context.Context, _ api.HealthCheckRequestObject) (api.HealthCheckResponseObject, error) {
	return api.HealthCheck200JSONResponse{
		Status:  "ok",
		Version: s.version,
	}, nil
}
