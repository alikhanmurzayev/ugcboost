package service

import (
	"context"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
)

// TelegramMessageRepoFactory exposes only the constructors TelegramMessageService
// needs (accept interfaces, return structs — see backend-design).
type TelegramMessageRepoFactory interface {
	NewTelegramMessageRepo(db dbutil.DB) repository.TelegramMessageRepo
}

// TelegramMessageService backs GET /telegram-messages. Read-only by design;
// inserts happen via the recorder + RecordingSender pair on the telegram side.
type TelegramMessageService struct {
	pool        dbutil.Pool
	repoFactory TelegramMessageRepoFactory
}

// NewTelegramMessageService wires the service. pool is used directly (no
// transaction); ListByChat does a single read.
func NewTelegramMessageService(pool dbutil.Pool, repoFactory TelegramMessageRepoFactory) *TelegramMessageService {
	return &TelegramMessageService{pool: pool, repoFactory: repoFactory}
}

// ListByChat returns up to `limit` rows for the chat plus an optional next
// cursor. The repo is asked for limit+1; if it returns more than `limit`, the
// extra row is dropped from the page and its predecessor becomes the cursor
// origin. Handler validates limit ∈ [1,100] before calling.
func (s *TelegramMessageService) ListByChat(
	ctx context.Context,
	chatID int64,
	cursor *domain.TelegramMessagesCursor,
	limit int,
) ([]*repository.TelegramMessageRow, *domain.TelegramMessagesCursor, error) {
	repo := s.repoFactory.NewTelegramMessageRepo(s.pool)
	rows, err := repo.ListByChat(ctx, chatID, cursor, limit+1)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) <= limit {
		return rows, nil, nil
	}
	page := rows[:limit]
	last := page[len(page)-1]
	next := &domain.TelegramMessagesCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	return page, next, nil
}
