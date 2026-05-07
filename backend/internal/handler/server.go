package handler

import (
	"context"
	"net/http"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/logger"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/middleware"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/service"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/testapi"
)

// AuthService is the interface Server needs from the auth service.
type AuthService interface {
	Login(ctx context.Context, email, password string) (*service.LoginResult, error)
	Refresh(ctx context.Context, rawRefreshToken string) (*service.RefreshResult, error)
	LogoutByRefresh(ctx context.Context, rawRefreshToken string) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, rawToken, newPassword string) (string, error)
	GetUser(ctx context.Context, userID string) (*domain.User, error)
}

// BrandService is the interface Server needs from the brand service.
type BrandService interface {
	CreateBrand(ctx context.Context, name string, logoURL *string) (*domain.Brand, error)
	GetBrand(ctx context.Context, id string) (*domain.Brand, error)
	ListBrands(ctx context.Context, managerID *string) ([]*domain.BrandListItem, error)
	UpdateBrand(ctx context.Context, id, name string, logoURL *string) (*domain.Brand, error)
	DeleteBrand(ctx context.Context, id string) error
	ListManagers(ctx context.Context, brandID string) ([]*domain.BrandManager, error)
	AssignManager(ctx context.Context, brandID, email string) (*domain.User, string, error)
	RemoveManager(ctx context.Context, brandID, userID string) error
}

// AuthzService is the interface Server needs from the authorisation service.
type AuthzService interface {
	CanCreateBrand(ctx context.Context) error
	CanListBrands(ctx context.Context) (canViewAll bool, userID string, err error)
	CanViewBrand(ctx context.Context, brandID string) error
	CanUpdateBrand(ctx context.Context, brandID string) error
	CanDeleteBrand(ctx context.Context, brandID string) error
	CanAssignManager(ctx context.Context, brandID string) error
	CanRemoveManager(ctx context.Context, brandID, userID string) error
	CanListAuditLogs(ctx context.Context) error
	CanViewCreatorApplication(ctx context.Context) error
	CanListCreatorApplications(ctx context.Context) error
	CanGetCreatorApplicationsCounts(ctx context.Context) error
	CanVerifyCreatorApplicationSocialManually(ctx context.Context) error
	CanRejectCreatorApplication(ctx context.Context) error
	CanApproveCreatorApplication(ctx context.Context) error
	CanViewCreator(ctx context.Context) error
	CanViewCreators(ctx context.Context) error
	CanCreateCampaign(ctx context.Context) error
	CanGetCampaign(ctx context.Context) error
	CanUpdateCampaign(ctx context.Context) error
	CanListCampaigns(ctx context.Context) error
	CanAddCampaignCreators(ctx context.Context) error
	CanRemoveCampaignCreator(ctx context.Context) error
	CanListCampaignCreators(ctx context.Context) error
}

// AuditLogService is the interface Server needs from the audit service.
type AuditLogService interface {
	List(ctx context.Context, f domain.AuditFilter, page, perPage int) ([]*domain.AuditLog, int64, error)
}

// CreatorApplicationService is the interface Server needs from the creator
// application service (public submission flow from the landing page plus the
// admin-only read aggregate used by the moderation UI). Verification flows
// from the SendPulse webhook also live here so the handler stays thin.
type CreatorApplicationService interface {
	Submit(ctx context.Context, in domain.CreatorApplicationInput) (*domain.CreatorApplicationSubmission, error)
	GetByID(ctx context.Context, id string) (*domain.CreatorApplicationDetail, error)
	List(ctx context.Context, in domain.CreatorApplicationListInput) (*domain.CreatorApplicationListPage, error)
	Counts(ctx context.Context) (map[string]int64, error)
	VerifyInstagramByCode(ctx context.Context, code, igHandle string) (domain.VerifyInstagramStatus, error)
	VerifyApplicationSocialManually(ctx context.Context, applicationID, socialID, actorUserID string) error
	RejectApplication(ctx context.Context, applicationID, actorUserID string) error
	ApproveApplication(ctx context.Context, applicationID, actorUserID string) (string, error)
}

// DictionaryService is the interface Server needs to serve public dictionaries
// (categories, cities) on the landing page.
type DictionaryService interface {
	List(ctx context.Context, t domain.DictionaryType) ([]domain.DictionaryEntry, error)
}

// CreatorService is the interface Server needs from the creator-side service:
// the GET /creators/{id} aggregate read used by the admin moderation UI plus
// the POST /creators/list paginated list backing the campaign-side catalog.
type CreatorService interface {
	GetByID(ctx context.Context, creatorID string) (*domain.CreatorAggregate, error)
	List(ctx context.Context, in domain.CreatorListInput) (*domain.CreatorListPage, error)
}

// CampaignService is the interface Server needs from the campaign service —
// admin-only POST /campaigns plus per-id read and patch; subsequent chunks
// (#6–#7) extend it with list / soft-delete.
type CampaignService interface {
	CreateCampaign(ctx context.Context, in domain.CampaignInput) (*domain.Campaign, error)
	GetByID(ctx context.Context, id string) (*domain.Campaign, error)
	UpdateCampaign(ctx context.Context, id string, in domain.CampaignInput) error
	List(ctx context.Context, in domain.CampaignListInput) (*domain.CampaignListPage, error)
}

// CampaignCreatorService is the interface Server needs from the campaign-
// creator service — admin-only batch add (POST), single remove (DELETE) and
// no-pagination list (GET) on /campaigns/{id}/creators.
type CampaignCreatorService interface {
	Add(ctx context.Context, campaignID string, creatorIDs []string) ([]*domain.CampaignCreator, error)
	Remove(ctx context.Context, campaignID, creatorID string) error
	List(ctx context.Context, campaignID string) ([]*domain.CampaignCreator, error)
}

// ServerConfig bundles configuration values the handler layer needs. Keeping
// them in a struct lets NewServer grow without a long positional signature.
type ServerConfig struct {
	Version               string
	CookieSecure          bool
	TelegramBotUsername   string
	LegalAgreementVersion string
	LegalPrivacyVersion   string
}

// Server implements api.StrictServerInterface.
type Server struct {
	authService               AuthService
	brandService              BrandService
	authzService              AuthzService
	auditService              AuditLogService
	creatorApplicationService CreatorApplicationService
	creatorService            CreatorService
	campaignService           CampaignService
	campaignCreatorService    CampaignCreatorService
	dictionaryService         DictionaryService
	version                   string
	cookieSecure              bool
	telegramBotUsername       string
	legalAgreementVersion     string
	legalPrivacyVersion       string
	logger                    logger.Logger
}

var _ api.StrictServerInterface = (*Server)(nil)

// NewServer creates a new Server.
func NewServer(
	auth AuthService,
	brands BrandService,
	authz AuthzService,
	audit AuditLogService,
	creatorApps CreatorApplicationService,
	creators CreatorService,
	campaigns CampaignService,
	campaignCreators CampaignCreatorService,
	dict DictionaryService,
	cfg ServerConfig,
	log logger.Logger,
) *Server {
	return &Server{
		authService:               auth,
		brandService:              brands,
		authzService:              authz,
		auditService:              audit,
		creatorApplicationService: creatorApps,
		creatorService:            creators,
		campaignService:           campaigns,
		campaignCreatorService:    campaignCreators,
		dictionaryService:         dict,
		version:                   cfg.Version,
		cookieSecure:              cfg.CookieSecure,
		telegramBotUsername:       cfg.TelegramBotUsername,
		legalAgreementVersion:     cfg.LegalAgreementVersion,
		legalPrivacyVersion:       cfg.LegalPrivacyVersion,
		logger:                    log,
	}
}

// strictErrorHandlerFunc matches both api.* and testapi.* RequestErrorHandlerFunc /
// ResponseErrorHandlerFunc signatures — they are identical net/http handlers.
type strictErrorHandlerFunc = func(w http.ResponseWriter, r *http.Request, err error)

// newStrictErrorHandlers binds respondError to a logger and returns the pair of
// handlers strict-server expects. Body-decode errors always become 422 +
// CodeValidation; runtime errors keep their domain-driven mapping. The
// SendPulse webhook is the single exception: per its anti-fingerprinting
// contract every authenticated request must respond 200 `{}` regardless
// of whether the handler succeeded, no-op'd or hit an infra failure (the
// 401 path is owned upstream by SendPulseAuth middleware). Routing both
// request- and response-error hooks through suppressSendPulseError keeps
// the contract uniform.
func newStrictErrorHandlers(log logger.Logger) (request, response strictErrorHandlerFunc) {
	request = func(w http.ResponseWriter, r *http.Request, _ error) {
		if suppressSendPulseError(w, r, log) {
			return
		}
		respondError(w, r, domain.NewValidationError(domain.CodeValidation, "Invalid request body"), log)
	}
	response = func(w http.ResponseWriter, r *http.Request, err error) {
		if suppressSendPulseError(w, r, log) {
			log.Error(r.Context(), "sendpulse webhook: suppressed downstream error",
				"error", err.Error(), "path", r.URL.Path)
			return
		}
		respondError(w, r, err, log)
	}
	return
}

// suppressSendPulseError detects the SendPulse webhook path and writes the
// canonical 200 `{}` response so neither validation nor downstream errors
// leak past the auth boundary. Returns true when it handled the response.
func suppressSendPulseError(w http.ResponseWriter, r *http.Request, log logger.Logger) bool {
	if r.URL.Path != middleware.SendPulseWebhookPath {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("{}\n")); err != nil {
		log.Error(r.Context(), "sendpulse webhook 200 encode failed", "error", err)
	}
	return true
}

// NewStrictAPIHandler wraps a Server with the strict-server adapter, plugging
// respondError as the source of truth for both decode-time and runtime errors.
// The same factory is shared by main.go and helpers_test.go to keep production
// and test wiring identical.
func NewStrictAPIHandler(s *Server) api.ServerInterface {
	requestErr, responseErr := newStrictErrorHandlers(s.logger)
	return api.NewStrictHandlerWithOptions(s, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErr,
		ResponseErrorHandlerFunc: responseErr,
	})
}

// NewStrictTestAPIHandler mirrors NewStrictAPIHandler for the test API.
func NewStrictTestAPIHandler(h *TestAPIHandler) testapi.ServerInterface {
	requestErr, responseErr := newStrictErrorHandlers(h.logger)
	return testapi.NewStrictHandlerWithOptions(h, nil, testapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErr,
		ResponseErrorHandlerFunc: responseErr,
	})
}
