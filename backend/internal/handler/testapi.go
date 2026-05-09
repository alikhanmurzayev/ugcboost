package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	tgbot "github.com/go-telegram/bot"
	tgmodels "github.com/go-telegram/bot/models"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/trustmeport"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
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
	NewCreatorRepo(db dbutil.DB) repository.CreatorRepo
	NewCampaignRepo(db dbutil.DB) repository.CampaignRepo
}

// TestAPIHandler provides test-only endpoints that back openapi-test.yaml.
// Only registered when ENVIRONMENT != production. The cleanup endpoint
// reaches into the repository layer directly (rather than through a service)
// because the hard-delete semantics are test-only and must not leak into
// production call sites — see repository.UserRepo.DeleteForTests for details.
type TestAPIHandler struct {
	auth             TestAPIAuthService
	pool             dbutil.Pool
	repos            TestAPICleanupRepoFactory
	tokenStore       TokenStore
	tgHandler        *telegram.Handler
	telegramSpy      *telegram.SentSpyStore
	telegramBotToken string
	trustMeRunner    trustmeport.OutboxRunner
	trustMeSpy       trustmeport.SpyStore
	logger           logger.Logger
}

var _ testapi.StrictServerInterface = (*TestAPIHandler)(nil)

// NewTestAPIHandler creates a new TestAPIHandler.
func NewTestAPIHandler(
	auth TestAPIAuthService,
	pool dbutil.Pool,
	repos TestAPICleanupRepoFactory,
	tokenStore TokenStore,
	tgHandler *telegram.Handler,
	telegramSpy *telegram.SentSpyStore,
	telegramBotToken string,
	trustMeRunner trustmeport.OutboxRunner,
	trustMeSpy trustmeport.SpyStore,
	log logger.Logger,
) *TestAPIHandler {
	return &TestAPIHandler{
		auth:             auth,
		pool:             pool,
		repos:            repos,
		tokenStore:       tokenStore,
		tgHandler:        tgHandler,
		telegramSpy:      telegramSpy,
		telegramBotToken: telegramBotToken,
		trustMeRunner:    trustMeRunner,
		trustMeSpy:       trustMeSpy,
		logger:           log,
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
func (h *TestAPIHandler) SendTelegramMessage(ctx context.Context, request testapi.SendTelegramMessageRequestObject) (testapi.SendTelegramMessageResponseObject, error) {
	req := request.Body
	if req.Text == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "text is required")
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
	h.tgHandler.Handle(ctx, spy, update)

	replies := make([]testapi.TelegramReply, 0, len(spy.sent))
	for _, params := range spy.sent {
		chatID, _ := params.ChatID.(int64)
		replies = append(replies, testapi.TelegramReply{
			ChatId: chatID,
			Text:   params.Text,
		})
	}
	return testapi.SendTelegramMessage200JSONResponse{Replies: replies}, nil
}

// SeedUser handles POST /test/seed-user.
func (h *TestAPIHandler) SeedUser(ctx context.Context, request testapi.SeedUserRequestObject) (testapi.SeedUserResponseObject, error) {
	req := request.Body

	email := string(req.Email)
	if email == "" || req.Password == "" || req.Role == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "email, password, and role are required")
	}

	user, err := h.auth.SeedUser(ctx, email, req.Password, string(req.Role))
	if err != nil {
		return nil, err
	}

	return testapi.SeedUser201JSONResponse{
		Data: testapi.SeedUserData{
			Id:    user.ID,
			Email: openapi_types.Email(user.Email),
			Role:  testapi.SeedUserDataRole(user.Role),
		},
	}, nil
}

// CleanupEntity handles POST /test/cleanup-entity.
// Dispatches by req.Type: "user" hard-deletes the user and its references
// inside a transaction; "brand" forwards to the standard brand delete.
func (h *TestAPIHandler) CleanupEntity(ctx context.Context, request testapi.CleanupEntityRequestObject) (testapi.CleanupEntityResponseObject, error) {
	req := request.Body

	if req.Id == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "id is required")
	}

	var deleteErr error
	switch req.Type {
	case testapi.User:
		deleteErr = dbutil.WithTx(ctx, h.pool, func(tx dbutil.DB) error {
			return h.repos.NewUserRepo(tx).DeleteForTests(ctx, req.Id)
		})
	case testapi.Brand:
		deleteErr = h.repos.NewBrandRepo(h.pool).Delete(ctx, req.Id)
	case testapi.CreatorApplication:
		deleteErr = h.repos.NewCreatorApplicationRepo(h.pool).DeleteForTests(ctx, req.Id)
	case testapi.Creator:
		deleteErr = h.repos.NewCreatorRepo(h.pool).DeleteForTests(ctx, req.Id)
	case testapi.Campaign:
		deleteErr = h.repos.NewCampaignRepo(h.pool).DeleteForTests(ctx, req.Id)
	default:
		if !req.Type.Valid() {
			return nil, domain.NewValidationError(domain.CodeValidation,
				fmt.Sprintf("unknown type: %q", req.Type))
		}
		// Defensive: a Valid value the switch above does not handle means the
		// OpenAPI enum grew but the cleanup dispatch was not extended.
		return nil, domain.NewValidationError(domain.CodeValidation,
			fmt.Sprintf("unsupported type: %q", req.Type))
	}

	if deleteErr != nil {
		if errors.Is(deleteErr, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, deleteErr
	}

	return testapi.CleanupEntity204Response{}, nil
}

// GetResetToken handles GET /test/reset-tokens?email=...
func (h *TestAPIHandler) GetResetToken(_ context.Context, request testapi.GetResetTokenRequestObject) (testapi.GetResetTokenResponseObject, error) {
	email := string(request.Params.Email)
	token, ok := h.tokenStore.GetToken(email)
	if !ok {
		return nil, domain.ErrNotFound
	}

	return testapi.GetResetToken200JSONResponse{
		Data: testapi.ResetTokenData{Token: token},
	}, nil
}

// GetTelegramSent handles GET /test/telegram/sent. Returns SentRecord rows
// captured by the spy store, filtered by chatId (always required so parallel
// e2e suites stay isolated) and optional `since`. WebApp URL is extracted
// from the InlineKeyboardMarkup the sender attached, when present.
func (h *TestAPIHandler) GetTelegramSent(_ context.Context, request testapi.GetTelegramSentRequestObject) (testapi.GetTelegramSentResponseObject, error) {
	filter := telegram.SentFilter{ChatID: request.Params.ChatId}
	if request.Params.Since != nil {
		filter.Since = *request.Params.Since
	}
	records := h.telegramSpy.List(filter)
	out := make([]testapi.TelegramSentMessage, 0, len(records))
	for _, r := range records {
		msg := testapi.TelegramSentMessage{
			ChatId: r.ChatID,
			Text:   r.Text,
			SentAt: r.SentAt,
		}
		if url := webAppURLFrom(r.ReplyMarkup); url != "" {
			msg.WebAppUrl = &url
		}
		if r.Err != "" {
			errCopy := r.Err
			msg.Error = &errCopy
		}
		out = append(out, msg)
	}
	return testapi.GetTelegramSent200JSONResponse{
		Data: testapi.TelegramSentData{Messages: out},
	}, nil
}

// webAppURLFrom extracts the first WebApp URL from an InlineKeyboardMarkup,
// when the recorded reply markup is exactly that shape. Anything else
// returns "" — callers treat empty as "no WebApp button attached".
func webAppURLFrom(markup any) string {
	kb, ok := markup.(tgmodels.InlineKeyboardMarkup)
	if !ok {
		// Pointer-shaped markup is what NewInlineKeyboardMarkup helpers produce;
		// flat-value comes from the SendCampaignInvite path. Try both before
		// giving up.
		kbPtr, okPtr := markup.(*tgmodels.InlineKeyboardMarkup)
		if !okPtr || kbPtr == nil {
			return ""
		}
		kb = *kbPtr
	}
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.WebApp != nil {
				return btn.WebApp.URL
			}
		}
	}
	return ""
}

// TelegramSpyFailNext handles POST /test/telegram/spy/fail-next. Registers
// a one-shot synthetic Telegram failure for the given chat_id; the next
// SendMessage call returns the registered reason string (defaulting to the
// canonical "Forbidden: bot was blocked by the user" so
// MapTelegramErrorToReason classifies it as bot_blocked).
func (h *TestAPIHandler) TelegramSpyFailNext(_ context.Context, request testapi.TelegramSpyFailNextRequestObject) (testapi.TelegramSpyFailNextResponseObject, error) {
	if request.Body == nil {
		return nil, domain.NewValidationError(domain.CodeValidation, "body is required")
	}
	reason := ""
	if request.Body.Reason != nil {
		reason = *request.Body.Reason
	}
	h.telegramSpy.RegisterFailNext(request.Body.ChatId, reason)
	return testapi.TelegramSpyFailNext204Response{}, nil
}

// TelegramSpyFakeChat handles POST /test/telegram/spy/fake-chat. Marks
// chatId as test-synthetic so TeeSender bypasses the real upstream bot
// for that chat — synthetic test chat_ids cannot be reached by a live bot.
// Strict-server enforces non-nil body upstream; no defensive check here.
func (h *TestAPIHandler) TelegramSpyFakeChat(_ context.Context, request testapi.TelegramSpyFakeChatRequestObject) (testapi.TelegramSpyFakeChatResponseObject, error) {
	h.telegramSpy.RegisterFakeChat(request.Body.ChatId)
	return testapi.TelegramSpyFakeChat204Response{}, nil
}

// SignTMAInitData handles POST /test/tma/sign-init-data. Mints a valid
// init_data query string for the supplied telegram_user_id signed with the
// production bot token, so e2e suites can drive /tma/* endpoints without
// running a real Telegram WebApp client. Optional `authDate` lets callers
// rehearse expired / future-dated paths; defaults to now().
func (h *TestAPIHandler) SignTMAInitData(_ context.Context, request testapi.SignTMAInitDataRequestObject) (testapi.SignTMAInitDataResponseObject, error) {
	if request.Body == nil || request.Body.TelegramUserId <= 0 {
		return nil, domain.NewValidationError(domain.CodeValidation, "telegramUserId must be a positive integer")
	}
	authDate := time.Now()
	if request.Body.AuthDate != nil {
		authDate = time.Unix(*request.Body.AuthDate, 0)
	}
	initData := middleware.SignTMAInitDataForTests(h.telegramBotToken, request.Body.TelegramUserId, authDate)
	return testapi.SignTMAInitData200JSONResponse{
		Data: testapi.SignTMAInitDataData{InitData: initData},
	}, nil
}

// TrustMeRunOutboxOnce handles POST /test/trustme/run-outbox-once. Synchronous
// driver of one outbox tick — e2e tests do not wait for the @every 10s cron
// tick. trustMeRunner=nil → 404 (test endpoint disabled, per spec line 118).
func (h *TestAPIHandler) TrustMeRunOutboxOnce(ctx context.Context, _ testapi.TrustMeRunOutboxOnceRequestObject) (testapi.TrustMeRunOutboxOnceResponseObject, error) {
	if h.trustMeRunner == nil {
		return nil, domain.ErrNotFound
	}
	h.trustMeRunner.RunOnce(ctx)
	return testapi.TrustMeRunOutboxOnce204Response{}, nil
}

// TrustMeSpyList handles GET /test/trustme/spy-list. Returns recorded
// SendToSign attempts with PII fingerprinted (security.md hard rule —
// raw FIO/IIN/Phone are forbidden in any response body). E2E asserts on
// fingerprint stability: same input → same fingerprint.
//
// trustMeSpy=nil → 404 (real client on staging — no spy data to expose).
func (h *TestAPIHandler) TrustMeSpyList(_ context.Context, _ testapi.TrustMeSpyListRequestObject) (testapi.TrustMeSpyListResponseObject, error) {
	if h.trustMeSpy == nil {
		return nil, domain.ErrNotFound
	}
	records := h.trustMeSpy.List()
	out := make([]testapi.TrustMeSentRecord, 0, len(records))
	for _, r := range records {
		item := testapi.TrustMeSentRecord{
			AdditionalInfo:   r.AdditionalInfo,
			ContractName:     r.ContractName,
			FioFingerprint:   r.FIOFingerprint,
			IinFingerprint:   r.IINFingerprint,
			PhoneFingerprint: r.PhoneFingerprint,
			PdfSha256:        r.PDFSha256,
			SentAt:           r.SentAt,
		}
		if r.DocumentID != "" {
			docID := r.DocumentID
			item.DocumentId = &docID
		}
		if r.ShortURL != "" {
			short := r.ShortURL
			item.ShortUrl = &short
		}
		if r.Err != "" {
			errCopy := r.Err
			item.Err = &errCopy
		}
		out = append(out, item)
	}
	return testapi.TrustMeSpyList200JSONResponse{
		Data: testapi.TrustMeSpyListData{Items: out},
	}, nil
}

// TrustMeSpyClear handles POST /test/trustme/spy-clear.
func (h *TestAPIHandler) TrustMeSpyClear(_ context.Context, _ testapi.TrustMeSpyClearRequestObject) (testapi.TrustMeSpyClearResponseObject, error) {
	if h.trustMeSpy == nil {
		return nil, domain.ErrNotFound
	}
	h.trustMeSpy.Clear()
	return testapi.TrustMeSpyClear204Response{}, nil
}

// TrustMeSpyFailNext handles POST /test/trustme/spy-fail-next. Empty
// additionalInfo — wildcard, fails next `count` SendToSign'ов независимо от
// additionalInfo. Используется e2e Phase 0 recovery test где contract_id
// присваивается worker'ом и не известен заранее.
func (h *TestAPIHandler) TrustMeSpyFailNext(_ context.Context, request testapi.TrustMeSpyFailNextRequestObject) (testapi.TrustMeSpyFailNextResponseObject, error) {
	if h.trustMeSpy == nil {
		return nil, domain.ErrNotFound
	}
	if request.Body == nil {
		return nil, domain.NewValidationError(domain.CodeValidation, "body is required")
	}
	reason := ""
	if request.Body.Reason != nil {
		reason = *request.Body.Reason
	}
	count := 1
	if request.Body.Count != nil && *request.Body.Count > 0 {
		count = *request.Body.Count
	}
	h.trustMeSpy.RegisterFailNext(request.Body.AdditionalInfo, reason, count)
	return testapi.TrustMeSpyFailNext204Response{}, nil
}

// TrustMeSpyRegisterDocument handles POST /test/trustme/spy-register-document.
func (h *TestAPIHandler) TrustMeSpyRegisterDocument(_ context.Context, request testapi.TrustMeSpyRegisterDocumentRequestObject) (testapi.TrustMeSpyRegisterDocumentResponseObject, error) {
	if h.trustMeSpy == nil {
		return nil, domain.ErrNotFound
	}
	if request.Body == nil || request.Body.AdditionalInfo == "" || request.Body.DocumentId == "" {
		return nil, domain.NewValidationError(domain.CodeValidation, "additionalInfo and documentId are required")
	}
	shortURL := "https://test.trustme.kz/uploader/" + request.Body.DocumentId
	if request.Body.ShortUrl != nil && *request.Body.ShortUrl != "" {
		shortURL = *request.Body.ShortUrl
	}
	status := 0
	if request.Body.ContractStatus != nil {
		status = *request.Body.ContractStatus
	}
	h.trustMeSpy.RegisterDocument(request.Body.AdditionalInfo, request.Body.DocumentId, shortURL, status)
	return testapi.TrustMeSpyRegisterDocument204Response{}, nil
}
