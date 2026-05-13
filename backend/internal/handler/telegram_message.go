package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// ListTelegramMessages handles GET /telegram-messages — admin-only read of the
// chronological inbound/outbound feed for one chat. Keyset cursor pagination;
// cursor is opaque base64-JSON. The handler decodes it, validates limit
// bounds, then delegates to the service.
func (s *Server) ListTelegramMessages(ctx context.Context, request api.ListTelegramMessagesRequestObject) (api.ListTelegramMessagesResponseObject, error) {
	if err := s.authzService.CanReadTelegramMessages(ctx); err != nil {
		return nil, err
	}

	params := request.Params
	if params.Limit < 1 || params.Limit > 100 {
		return nil, domain.NewValidationError(domain.CodeValidation, "limit must be between 1 and 100")
	}

	var cursor *domain.TelegramMessagesCursor
	if params.Cursor != nil && *params.Cursor != "" {
		c, err := domain.DecodeTelegramMessagesCursor(*params.Cursor)
		if err != nil {
			return nil, domain.NewValidationError(domain.CodeValidation, "invalid cursor")
		}
		cursor = c
	}

	rows, nextCursor, err := s.telegramMessageService.ListByChat(ctx, params.ChatId, cursor, params.Limit)
	if err != nil {
		return nil, err
	}

	items := make([]api.TelegramMessage, len(rows))
	for i, row := range rows {
		item, err := telegramMessageRowToAPI(row)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}

	var nextCursorStr *string
	if nextCursor != nil {
		encoded, err := domain.EncodeTelegramMessagesCursor(*nextCursor)
		if err != nil {
			return nil, err
		}
		nextCursorStr = &encoded
	}

	return api.ListTelegramMessages200JSONResponse{
		Data: api.TelegramMessagesData{
			Items:      items,
			NextCursor: nextCursorStr,
		},
	}, nil
}

func telegramMessageRowToAPI(row *repository.TelegramMessageRow) (api.TelegramMessage, error) {
	id, err := uuid.Parse(row.ID)
	if err != nil {
		return api.TelegramMessage{}, fmt.Errorf("parse telegram_messages.id %q: %w", row.ID, err)
	}
	out := api.TelegramMessage{
		Id:                id,
		ChatId:            row.ChatID,
		Direction:         api.TelegramMessageDirection(row.Direction),
		Text:              row.Text,
		TelegramMessageId: row.TelegramMessageID,
		TelegramUsername:  row.TelegramUsername,
		Error:             row.Error,
		CreatedAt:         row.CreatedAt,
	}
	if row.Status != nil {
		s := api.TelegramMessageStatus(*row.Status)
		out.Status = &s
	}
	return out, nil
}
