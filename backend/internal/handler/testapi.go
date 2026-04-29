package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/integration/telegram"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/repository"
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

// TelegramDispatcher is the narrow interface the test handler needs to push
// a synthetic update through the production dispatcher. Defined here so the
// handler stays decoupled from the integration package's other surface.
type TelegramDispatcher interface {
	Dispatch(ctx context.Context, update telegram.IncomingUpdate)
}

// TelegramSpy is the test-side draining surface used to capture the bot's
// replies after Dispatch returns. nil in production (the handler is not
// registered when EnableTestEndpoints is false).
type TelegramSpy interface {
	Drain(chatID int64) []telegram.SentMessage
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
	dispatcher TelegramDispatcher
	spy        TelegramSpy
	logger     logger.Logger
}

var _ testapi.ServerInterface = (*TestAPIHandler)(nil)

// NewTestAPIHandler creates a new TestAPIHandler.
func NewTestAPIHandler(
	auth TestAPIAuthService,
	pool dbutil.Pool,
	repos TestAPICleanupRepoFactory,
	tokenStore TokenStore,
	dispatcher TelegramDispatcher,
	spy TelegramSpy,
	log logger.Logger,
) *TestAPIHandler {
	return &TestAPIHandler{
		auth:       auth,
		pool:       pool,
		repos:      repos,
		tokenStore: tokenStore,
		dispatcher: dispatcher,
		spy:        spy,
		logger:     log,
	}
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

// SendTelegramUpdate handles POST /test/telegram/send-update.
//
// Body is decoded into the generated testapi.SendTelegramUpdateRequest, then
// shaped into a domain telegram.IncomingUpdate and dispatched synchronously
// through the production dispatcher. After Dispatch returns, the spy client
// is drained for the chat under test and every captured reply is returned
// in the response. The whole call is one DB-less synchronous operation —
// e2e tests use it to drive the bot without a live Telegram connection.
func (h *TestAPIHandler) SendTelegramUpdate(w http.ResponseWriter, r *http.Request) {
	if h.dispatcher == nil || h.spy == nil {
		// Plain error → respondError default branch → 500 INTERNAL_ERROR
		// (NOT a BusinessError, which would map to 409 Conflict).
		respondError(w, r, fmt.Errorf("telegram test endpoint not wired"), h.logger)
		return
	}
	var req testapi.SendTelegramUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), h.logger)
		return
	}
	// Reject zero/negative ids — Telegram never sends them, so an
	// accidental {chatId:0} from a misconfigured test must surface as 422,
	// not silently dispatch a phantom update and drain chat 0.
	if req.UpdateId <= 0 || req.ChatId <= 0 || req.UserId <= 0 {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation,
			"updateId, chatId and userId must be positive"), h.logger)
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		respondError(w, r, domain.NewValidationError(domain.CodeValidation,
			"text must be non-empty"), h.logger)
		return
	}

	update := telegram.IncomingUpdate{
		UpdateID:  req.UpdateId,
		ChatID:    req.ChatId,
		UserID:    req.UserId,
		Text:      req.Text,
		Username:  req.Username,
		FirstName: req.FirstName,
		LastName:  req.LastName,
	}
	h.dispatcher.Dispatch(r.Context(), update)

	captured := h.spy.Drain(req.ChatId)
	replies := make([]testapi.TelegramReply, len(captured))
	for i, m := range captured {
		replies[i] = testapi.TelegramReply{ChatId: m.ChatID, Text: m.Text}
	}

	respondJSON(w, r, http.StatusOK, testapi.SendTelegramUpdateResult{
		Data: testapi.SendTelegramUpdateData{Replies: replies},
	}, h.logger)
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
