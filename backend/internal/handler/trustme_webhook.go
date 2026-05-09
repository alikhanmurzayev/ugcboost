package handler

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

// TrustMeWebhook принимает state-change webhook от TrustMe. Auth проверяется
// upstream через middleware.TrustMeWebhookAuth (401 `{}` на missing/wrong).
// Тело payload'а уже распарсено strict-server'ом в TrustMeWebhookRequest.
//
// Маппит api-type → domain.TrustMeWebhookEvent, дальше WebhookService:
// один Tx (lookup + UPDATE contracts с двойным guard'ом + cc.status flips +
// audit) и fire-and-forget notify после COMMIT.
//
// Sentinel-ошибки → respondError → 404/422 (см. response.go).
func (s *Server) TrustMeWebhook(ctx context.Context, request api.TrustMeWebhookRequestObject) (api.TrustMeWebhookResponseObject, error) {
	if request.Body == nil {
		return nil, domain.ErrContractWebhookInvalidStatus
	}
	ev, err := domain.NewTrustMeWebhookEvent(request.Body.ContractId, request.Body.Status)
	if err != nil {
		return nil, err
	}
	if err := s.trustMeWebhookService.HandleEvent(ctx, ev); err != nil {
		return nil, err
	}
	return api.TrustMeWebhook200JSONResponse{}, nil
}
