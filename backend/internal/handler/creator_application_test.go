package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/api"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
	"github.com/alikhanmurzayev/ugcboost/backend/internal/handler/mocks"
	logmocks "github.com/alikhanmurzayev/ugcboost/backend/internal/logger/mocks"
)

// validRequest returns a payload that the handler will accept end-to-end so
// scenarios can mutate one field to hit a specific branch.
func validRequest() api.CreatorApplicationSubmitRequest {
	middle := "Ивановна"
	return api.CreatorApplicationSubmitRequest{
		LastName:   "Муратова",
		FirstName:  "Айдана",
		MiddleName: &middle,
		Iin:        "950515312348",
		Phone:      "+77001234567",
		City:       "Алматы",
		Address:    "ул. Абая 1",
		Categories: []string{"beauty", "fashion"},
		Socials: []api.SocialAccountInput{
			{Platform: api.Instagram, Handle: "@aidana"},
			{Platform: api.Tiktok, Handle: "aidana_tt"},
		},
		Consents: api.ConsentsInput{
			Processing:  true,
			ThirdParty:  true,
			CrossBorder: true,
			Terms:       true,
		},
	}
}

func serverWithCreator(t *testing.T, creator CreatorApplicationService, log *logmocks.MockLogger) *Server {
	t.Helper()
	return NewServer(nil, nil, nil, nil, creator, ServerConfig{
		Version:               "test-version",
		TelegramBotUsername:   "ugcboost_test_bot",
		LegalAgreementVersion: "2026-04-20",
		LegalPrivacyVersion:   "2026-04-20",
	}, log)
}

func TestServer_SubmitCreatorApplication(t *testing.T) {
	t.Parallel()

	t.Run("invalid json body returns 422", func(t *testing.T) {
		t.Parallel()
		router := newTestRouter(t, serverWithCreator(t, nil, logmocks.NewMockLogger(t)))

		req := httptest.NewRequest(http.MethodPost, "/creators/applications", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("service validation error passes through as 422", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Return(nil, domain.NewValidationError(domain.CodeInvalidIIN, "Некорректный ИИН"))

		router := newTestRouter(t, serverWithCreator(t, creator, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/applications", validRequest())

		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Equal(t, domain.CodeInvalidIIN, resp.Error.Code)
	})

	t.Run("duplicate returns 409", func(t *testing.T) {
		t.Parallel()
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Submit(mock.Anything, mock.Anything).
			Return(nil, domain.NewBusinessError(domain.CodeCreatorApplicationDuplicate, "already exists"))

		router := newTestRouter(t, serverWithCreator(t, creator, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.ErrorResponse](t, router, http.MethodPost, "/creators/applications", validRequest())

		require.Equal(t, http.StatusConflict, w.Code)
		require.Equal(t, domain.CodeCreatorApplicationDuplicate, resp.Error.Code)
	})

	t.Run("success returns 201 with deep link and forwards request metadata", func(t *testing.T) {
		t.Parallel()
		appID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
		creator := mocks.NewMockCreatorApplicationService(t)
		creator.EXPECT().Submit(mock.Anything, mock.MatchedBy(func(in domain.CreatorApplicationInput) bool {
			// Handler must forward all important fields and metadata verbatim.
			// IP is empty in unit tests because the ClientIP middleware is not
			// wired in newTestRouter — the forwarding path itself is covered by
			// middleware_test.go and by the E2E suite.
			return in.IIN == "950515312348" &&
				in.LastName == "Муратова" &&
				in.Phone == "+77001234567" &&
				in.UserAgent == "go-test/1" &&
				in.AgreementVersion == "2026-04-20" &&
				in.PrivacyVersion == "2026-04-20" &&
				len(in.Socials) == 2 &&
				in.Consents.Processing && in.Consents.ThirdParty && in.Consents.CrossBorder && in.Consents.Terms
		})).Return(&domain.CreatorApplicationSubmission{
			ApplicationID: appID.String(),
			BirthDate:     time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC),
		}, nil)

		router := newTestRouter(t, serverWithCreator(t, creator, logmocks.NewMockLogger(t)))
		w, resp := doJSON[api.CreatorApplicationSubmitResult](t, router, http.MethodPost, "/creators/applications", validRequest(),
			func(r *http.Request) {
				r.Header.Set("User-Agent", "go-test/1")
				r.RemoteAddr = "203.0.113.7:4567"
			})

		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, api.CreatorApplicationSubmitResult{
			Data: api.CreatorApplicationSubmitData{
				ApplicationId:  appID,
				TelegramBotUrl: "https://t.me/ugcboost_test_bot?start=" + appID.String(),
			},
		}, resp)
	})
}
