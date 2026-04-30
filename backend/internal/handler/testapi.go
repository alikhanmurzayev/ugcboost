package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/telegram"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/testapi"
)

// TokenStore retrieves raw reset tokens captured in memory.
type TokenStore interface {
	GetToken(email string) (string, bool)
}

// TestAPIAuthService is the subset of auth operations the test handler needs.
// Kept separate from AuthService so production callers never see SeedUser.
type TestAPIAuthService interface {
	SeedUser(ctx context.Context, email, password, role string) (*domain.User, error)
}

// TestAPICleanupRepoFactory is the narrow repo-factory interface the test
// cleanup endpoint uses. Kept separate from the production repo factory so
// the test handler only sees the constructors it actually needs.
type TestAPICleanupRepoFactory interface {
	NewUserRepo(db dbutil.DB) repository.UserRepo
	NewBrandRepo(db dbutil.DB) repository.BrandRepo
	NewCreatorApplicationRepo(db dbutil.DB) repository.CreatorApplicationRepo
}

// TestAPIHandler provides test-only endpoints that back openapi-test.yaml.
// Only registered when ENVIRONMENT != production. The cleanup endpoint
// reaches into the repository layer directly (rather than through a service)
// because the hard-delete semantics are test-only and must not leak into
// production call sites — see repository.UserRepo.DeleteForTests for details.
type TestAPIHandler struct {
	auth       TestAPIAuthService
	pool       dbutil.Pool
	repos      TestAPICleanupRepoFactory
	tokenStore TokenStore
	tgHandler  *telegram.Handler
	logger     logger.Logger
}

var _ testapi.ServerInterface = (*TestAPIHandler)(nil)

// NewTestAPIHandler creates a new TestAPIHandler.
func NewTestAPIHandler(
	auth TestAPIAuthService,
	pool dbutil.Pool,
	repos TestAPICleanupRepoFactory,
	tokenStore TokenStore,
	tgHandler *telegram.Handler,
	log logger.Logger,
) *TestAPIHandler {
	return &TestAPIHandler{
		auth:       auth,
		pool:       pool,
		repos:      repos,
		tokenStore: tokenStore,
		tgHandler:  tgHandler,
		logger:     log,
	}
}

// telegramSpySender captures every SendMessage call into an in-memory slice
// so SendTelegramMessage can return the bot's replies in the HTTP response.
// Lives only inside one request — fresh instance per call, no shared state.
type telegramSpySender struct {
	sent []*tgbot.SendMessageParams
}

func (s *telegramSpySender) SendMessage(_ context.Context, params *tgbot.SendMessageParams) (*tgmodels.Message, error) {
	s.sent = append(s.sent, params)
	return &tgmodels.Message{ID: len(s.sent)}, nil
}

// SendTelegramMessage handles POST /test/telegram/message.
// Builds a synthetic Update from the request body, invokes the in-process
// Telegram handler with a spy Sender, and returns whatever replies it
// produced. Bypasses long-polling and Telegram entirely.
func (h *TestAPIHandler) SendTelegramMessage(w http.ResponseWriter, r *http.Request) {
	var req testapi.SendTelegramMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), h.logger)
		return
	}
	if req.Text == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "text is required"), h.logger)
		return
	}

	from := &tgmodels.User{ID: req.ChatId}
	if req.UserId != nil {
		from.ID = *req.UserId
	}
	if req.Username != nil {
		from.Username = *req.Username
	}
	if req.FirstName != nil {
		from.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		from.LastName = *req.LastName
	}
	update := &tgmodels.Update{
		Message: &tgmodels.Message{
			// Test endpoint always simulates a private 1:1 chat with the bot —
			// the production handler drops everything else (group/channel) for
			// PII reasons, see telegram/handler.go.
			Chat: tgmodels.Chat{ID: req.ChatId, Type: "private"},
			Text: req.Text,
			From: from,
		},
	}
	spy := &telegramSpySender{}
	h.tgHandler.Handle(r.Context(), spy, update)

	replies := make([]testapi.TelegramReply, 0, len(spy.sent))
	for _, params := range spy.sent {
		chatID, _ := params.ChatID.(int64)
		replies = append(replies, testapi.TelegramReply{
			ChatId: chatID,
			Text:   params.Text,
		})
	}
	respondJSON(w, r, http.StatusOK, testapi.SendTelegramMessageResult{Replies: replies}, h.logger)
}

// SeedUser handles POST /test/seed-user.
func (h *TestAPIHandler) SeedUser(w http.ResponseWriter, r *http.Request) {
	var req testapi.SeedUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), h.logger)
		return
	}

	email := string(req.Email)
	if email == "" || req.Password == "" || req.Role == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "email, password, and role are required"), h.logger)
		return
	}

	user, err := h.auth.SeedUser(r.Context(), email, req.Password, string(req.Role))
	if err != nil {
		respondError(w, r, err, h.logger)
		return
	}

	respondJSON(w, r, http.StatusCreated, testapi.SeedUserResult{
		Data: testapi.SeedUserData{
			Id:    user.ID,
			Email: openapi_types.Email(user.Email),
			Role:  testapi.SeedUserDataRole(user.Role),
		},
	}, h.logger)
}

// CleanupEntity handles POST /test/cleanup-entity.
// Dispatches by req.Type: "user" hard-deletes the user and its references
// inside a transaction; "brand" forwards to the standard brand delete.
func (h *TestAPIHandler) CleanupEntity(w http.ResponseWriter, r *http.Request) {
	var req testapi.CleanupEntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), h.logger)
		return
	}

	if req.Id == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "id is required"), h.logger)
		return
	}

	var deleteErr error
	switch req.Type {
	case testapi.User:
		deleteErr = dbutil.WithTx(r.Context(), h.pool, func(tx dbutil.DB) error {
			return h.repos.NewUserRepo(tx).DeleteForTests(r.Context(), req.Id)
		})
	case testapi.Brand:
		deleteErr = h.repos.NewBrandRepo(h.pool).Delete(r.Context(), req.Id)
	case testapi.CreatorApplication:
		deleteErr = h.repos.NewCreatorApplicationRepo(h.pool).DeleteForTests(r.Context(), req.Id)
	default:
		if !req.Type.Valid() {
			respondError(w, r, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("unknown type: %q", req.Type)), h.logger)
			return
		}
		// Defensive: a Valid value the switch above does not handle means the
		// OpenAPI enum grew but the cleanup dispatch was not extended.
		respondError(w, r, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("unsupported type: %q", req.Type)), h.logger)
		return
	}

	if deleteErr != nil {
		if errors.Is(deleteErr, sql.ErrNoRows) {
			respondError(w, r, domain.ErrNotFound, h.logger)
			return
		}
		respondError(w, r, deleteErr, h.logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetResetToken handles GET /test/reset-tokens?email=...
func (h *TestAPIHandler) GetResetToken(w http.ResponseWriter, r *http.Request, params testapi.GetResetTokenParams) {
	email := string(params.Email)
	token, ok := h.tokenStore.GetToken(email)
	if !ok {
		respondError(w, r, domain.ErrNotFound, h.logger)
		return
	}

	respondJSON(w, r, http.StatusOK, testapi.ResetTokenResult{
		Data: testapi.ResetTokenData{Token: token},
	}, h.logger)
}
