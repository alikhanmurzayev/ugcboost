// Package creator_application — E2E тесты HTTP-поверхности
// POST /creators/applications (публичная ручка, лендинг Айданы) и
// GET /creators/applications/{id} (admin-only ручка модерации).
//
// TestSubmitCreatorApplication покрывает happy path подачи заявки. Клиент
// отправляет полный валидный payload (ФИО + ИИН + соцсети + согласия) и
// ожидает 201 с application_id (UUID) и ссылкой на Telegram-бот вида
// https://t.me/{bot}?start={id}. Application_id сохраняется для cleanup
// через /test/cleanup-entity: удаление родительской записи каскадно сносит
// соцсети, категории и согласия (DELETE CASCADE в миграциях), так что после
// теста в базе остаётся только audit-запись — её трогать не надо, аудит
// специально переживает очистку. После 201 тест поднимает админ-клиента
// и через GET /creators/applications/{id} сверяет, что записанный aggregate
// действительно содержит все отправленные поля — это превращает happy path
// в полноценный round-trip и подтверждает, что данные легли во все четыре
// таблицы.
//
// TestSubmitCreatorApplicationDuplicate закрывает инвариант из FR17: по ИИН
// с активной заявкой (verification) повторная подача отвергается 409
// CREATOR_APPLICATION_DUPLICATE, и в базе остаётся только первая заявка.
// Второй запрос не создаёт новых строк ни в одной из пяти таблиц —
// rollback всей транзакции при конфликте гарантирован partial unique index
// плюс явной проверкой в сервисе.
//
// TestSubmitCreatorApplicationValidation проходит по всем валидационным
// сценариям I/O-матрицы спеки. Некорректный формат ИИН, нарушенная
// контрольная сумма, несовершеннолетний возраст, отсутствующее согласие и
// неизвестная категория — каждая ошибка возвращает 422 с своим машинным
// кодом. Неподдерживаемая соцсеть блокируется на уровне OpenAPI enum —
// сгенерированный клиент не принимает такое значение в тип, поэтому мы
// отправляем запрос сырым HTTP через PostRaw, чтобы обойти клиентскую
// валидацию и дойти до серверной.
//
// TestSubmitCreatorApplicationOther и TestSubmitCreatorApplicationThreads
// проверяют, что специфичные ветки (категория «other» с обязательным
// categoryOtherText и платформа threads) проходят полный путь записи и
// потом честно возвращаются админу через GET /creators/applications/{id} —
// та же сверка aggregate подтверждает, что эти граничные случаи дошли до
// всех связанных таблиц.
//
// TestSubmitCreatorApplicationCFIP закрывает регрессию GH-39: за цепочкой
// Cloudflare → Dokploy → Docker backend раньше писал в audit_logs.ip_address
// IP edge-узла или Dokploy-прокси, а не реального клиента. Тест шлёт сырой
// POST с заголовком CF-Connecting-IP и через ListAuditLogs API убеждается,
// что IP из заголовка дошёл до audit-строки — это покрывает полную цепочку
// middleware → service → repo → DB → API, которую unit-тесты middleware
// изолированно поймать не могут.
//
// TestGetCreatorApplicationForbidden закрывает security-границу: brand_manager,
// хоть и аутентифицирован, не может прочитать заявку по id (403). Тест
// специально создаёт реальную заявку, чтобы убедиться: 403 возвращается
// именно из-за роли, а не из-за отсутствия записи. TestGetCreatorApplicationNotFound
// — обратный полюс: админ с валидным, но несуществующим UUID получает 404
// NOT_FOUND. Невалидный формат UUID не проверяем — это ветка
// HandleParamError, не ветка handler'а; и невалидный формат не покрывается
// этим слайсом.
//
// Все тесты параллельны и используют UniqueIIN для генерации валидного
// казахстанского ИИН — это защищает partial unique index от коллизий между
// параллельными прогонами. RegisterCreatorApplicationCleanup опирается на
// POST /test/cleanup-entity с type=creator_application; cleanup работает
// при E2E_CLEANUP=true (дефолт), при false — данные остаются для ручного
// инспекта. SetupAdminClient в новых GET-тестах сам регистрирует cleanup
// для созданного admin'а через user-cleanup helper.
package creator_application_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// validRequest builds a request body that satisfies every server-side check
// except for the uniqueness of the IIN, which the caller decides. City is a
// dictionary code ("almaty") rather than the human-readable label so the GET
// aggregate's dictionary resolution returns a populated name; sending a raw
// label would still pass the submit validation but would force the read path
// onto the deactivated-code fallback in every successful test.
//
// Address is intentionally left nil — the landing form does not collect a
// legal address, the column is nullable, and the GET aggregate must echo nil
// back. Tests that need to verify non-nil address round-trip set it
// explicitly.
func validRequest(iin string) apiclient.CreatorApplicationSubmitRequest {
	middle := "Ивановна"
	return apiclient.CreatorApplicationSubmitRequest{
		LastName:   "Муратова",
		FirstName:  "Айдана",
		MiddleName: &middle,
		Iin:        iin,
		Phone:      "+77001234567",
		City:       "almaty",
		Categories: []string{"beauty", "fashion"},
		Socials: []apiclient.SocialAccountInput{
			{Platform: apiclient.Instagram, Handle: "@aidana_" + iin[7:]},
			{Platform: apiclient.Tiktok, Handle: "aidana_tt_" + iin[7:]},
		},
		AcceptedAll: true,
	}
}

func TestSubmitCreatorApplication(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	iin := testutil.UniqueIIN()

	req := validRequest(iin)
	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)

	data := resp.JSON201.Data
	require.NotZero(t, data.ApplicationId)
	require.True(t, strings.HasPrefix(data.TelegramBotUrl, "https://t.me/"),
		"telegram bot url should start with https://t.me/, got %q", data.TelegramBotUrl)
	require.Contains(t, data.TelegramBotUrl, "?start="+data.ApplicationId.String(),
		"telegram bot url must carry the application id as start parameter")

	testutil.RegisterCreatorApplicationCleanup(t, data.ApplicationId.String())

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	verifyCreatorApplicationByID(t, adminClient, adminToken, data.ApplicationId.String(), req, "")

	// Audit-entry sanity: the same admin client can read the audit log filtered
	// by entity, and the creator_application_submit action must be present.
	// A direct submit without UTM markers must leave audit details compact —
	// no utm_* keys leak in when the landing did not carry any.
	entry := testutil.FindAuditEntry(t, adminClient, adminToken,
		"creator_application", data.ApplicationId.String(), "creator_application_submit")
	for _, k := range []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content"} {
		_, present := auditDetailsMap(t, entry)[k]
		require.False(t, present, "audit details unexpectedly carried %q for direct submit", k)
	}
}

func TestSubmitCreatorApplicationDuplicate(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	iin := testutil.UniqueIIN()
	req := validRequest(iin)

	first, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, first.StatusCode())
	require.NotNil(t, first.JSON201)
	testutil.RegisterCreatorApplicationCleanup(t, first.JSON201.Data.ApplicationId.String())

	// Mutate the second request so nothing accidentally passes because of
	// identical content — only the IIN must be the same.
	req.City = "astana"
	second, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, second.StatusCode())
	require.NotNil(t, second.JSON409)
	require.Equal(t, "CREATOR_APPLICATION_DUPLICATE", second.JSON409.Error.Code)
}

func TestSubmitCreatorApplicationValidation(t *testing.T) {
	t.Parallel()

	t.Run("invalid iin format", func(t *testing.T) {
		t.Parallel()
		// Raw HTTP bypasses the generated client's pattern validation so the
		// server's domain check fires instead. The error body should still be
		// a valid ErrorResponse with INVALID_IIN — same contract as every
		// other 422 in the I/O matrix.
		body := validRequestMap(testutil.UniqueIIN())
		body["iin"] = "bad"
		resp := testutil.PostRaw(t, "/creators/applications", body)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

		var envelope apiclient.ErrorResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
		require.Equal(t, "INVALID_IIN", envelope.Error.Code)
	})

	t.Run("invalid iin checksum", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		iin := testutil.UniqueIIN()
		// Flip the last digit to break the checksum while keeping the format.
		broken := iin[:11] + flipDigit(iin[11])
		resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), validRequest(broken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "INVALID_IIN", resp.JSON422.Error.Code)
	})

	t.Run("under MinCreatorAge rejected with UNDER_AGE", func(t *testing.T) {
		t.Parallel()
		// buildUnderageIIN picks a birth (MinCreatorAge-2) years before now, so
		// this test stays green regardless of when it runs (a hardcoded year
		// would break the moment real-world time caught up).
		iin := buildUnderageIIN()
		c := testutil.NewAPIClient(t)
		resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), validRequest(iin))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "UNDER_AGE", resp.JSON422.Error.Code)
	})

	t.Run("missing consent", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		req := validRequest(testutil.UniqueIIN())
		req.AcceptedAll = false
		resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "MISSING_CONSENT", resp.JSON422.Error.Code)
	})

	t.Run("too many categories rejected with VALIDATION_ERROR", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		req := validRequest(testutil.UniqueIIN())
		req.Categories = []string{"beauty", "fashion", "food", "fitness"}
		resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "VALIDATION_ERROR", resp.JSON422.Error.Code)
	})

	t.Run("other category without text rejected", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		req := validRequest(testutil.UniqueIIN())
		req.Categories = []string{"beauty", "other"}
		// CategoryOtherText is left nil — the server must answer 422.
		resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "VALIDATION_ERROR", resp.JSON422.Error.Code)
	})

	t.Run("unknown category", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		req := validRequest(testutil.UniqueIIN())
		req.Categories = []string{"beauty", "wizardry"}
		resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "UNKNOWN_CATEGORY", resp.JSON422.Error.Code)
	})

	t.Run("unsupported social platform via raw http", func(t *testing.T) {
		t.Parallel()
		// Handler decodes the body with plain json.NewDecoder (not through
		// HandleParamError, which only runs on query/path params), so unknown
		// enum values land in the typed struct and are rejected by the service
		// in normaliseSocials — deterministically 422 VALIDATION_ERROR.
		body := validRequestMap(testutil.UniqueIIN())
		body["socials"] = []map[string]string{
			{"platform": "facebook", "handle": "aidana"},
		}
		resp := testutil.PostRaw(t, "/creators/applications", body)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)

		var envelope apiclient.ErrorResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
		require.Equal(t, "VALIDATION_ERROR", envelope.Error.Code)
	})
}

// TestSubmitCreatorApplicationThreads sanity-checks that threads is now an
// accepted social platform end-to-end (migration + enum + service registry)
// and that the GET aggregate honours the new platform too.
func TestSubmitCreatorApplicationThreads(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	iin := testutil.UniqueIIN()
	req := validRequest(iin)
	req.Socials = []apiclient.SocialAccountInput{
		{Platform: apiclient.Threads, Handle: "aidana_th_" + iin[7:]},
	}

	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	testutil.RegisterCreatorApplicationCleanup(t, resp.JSON201.Data.ApplicationId.String())

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	verifyCreatorApplicationByID(t, adminClient, adminToken, resp.JSON201.Data.ApplicationId.String(), req, "")
}

// TestSubmitCreatorApplicationOther covers the "other" category branch:
// categoryOtherText is required and must be persisted alongside the
// application — the GET aggregate sees the trimmed value and the lone
// "other" code.
func TestSubmitCreatorApplicationOther(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	iin := testutil.UniqueIIN()
	req := validRequest(iin)
	req.Categories = []string{"other"}
	other := "Авторские ASMR-видео про винтажные велосипеды"
	req.CategoryOtherText = &other

	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	testutil.RegisterCreatorApplicationCleanup(t, resp.JSON201.Data.ApplicationId.String())

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	verifyCreatorApplicationByID(t, adminClient, adminToken, resp.JSON201.Data.ApplicationId.String(), req, other)
}

// TestSubmitCreatorApplicationWithUTM закрывает end-to-end UTM-цепочку:
// лендинг отдаёт пять `utm_*` маркеров в submit, бэк сохраняет их в новые
// колонки и возвращает в админский Detail-эндпоинт; audit-строка submit-а
// получает эти же значения в `details` плоскими ключами `utm_source` и т.д.
// Тест отправляет полный набор маркеров вместе с обычной заявкой и проверяет
// (1) detail-API echo — пять полей с теми же значениями; (2) audit `details`
// содержит ровно пять utm-ключей и совпадает с input. Базовый
// TestSubmitCreatorApplication уже закрывает обратную ветку (без UTM →
// audit-details без utm-ключей), так что split на две точки даёт обе
// границы инварианта.
func TestSubmitCreatorApplicationWithUTM(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	iin := testutil.UniqueIIN()
	req := validRequest(iin)
	req.UtmSource = pointer.ToString("telegram_chat")
	req.UtmMedium = pointer.ToString("tg")
	req.UtmCampaign = pointer.ToString("spring2026")
	req.UtmTerm = pointer.ToString("ugc")
	req.UtmContent = pointer.ToString("banner")

	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	applicationID := resp.JSON201.Data.ApplicationId.String()
	testutil.RegisterCreatorApplicationCleanup(t, applicationID)

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)

	// Detail echo: every UTM marker round-trips through DB to the admin GET.
	appUUID, err := uuid.Parse(applicationID)
	require.NoError(t, err)
	detail, err := adminClient.GetCreatorApplicationWithResponse(context.Background(), appUUID, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, detail.StatusCode())
	require.NotNil(t, detail.JSON200)
	got := detail.JSON200.Data
	// Each pointer is asserted non-nil before dereferencing so a missing
	// column or omitted JSON field surfaces as a clean test failure
	// instead of a nil-deref panic.
	require.NotNil(t, got.UtmSource)
	require.Equal(t, "telegram_chat", *got.UtmSource)
	require.NotNil(t, got.UtmMedium)
	require.Equal(t, "tg", *got.UtmMedium)
	require.NotNil(t, got.UtmCampaign)
	require.Equal(t, "spring2026", *got.UtmCampaign)
	require.NotNil(t, got.UtmTerm)
	require.Equal(t, "ugc", *got.UtmTerm)
	require.NotNil(t, got.UtmContent)
	require.Equal(t, "banner", *got.UtmContent)

	// Audit details echo: same five flat keys mirror the input markers; nothing
	// else UTM-shaped sneaks in.
	entry := testutil.FindAuditEntry(t, adminClient, adminToken,
		"creator_application", applicationID, "creator_application_submit")
	details := auditDetailsMap(t, entry)
	require.Equal(t, "telegram_chat", details["utm_source"])
	require.Equal(t, "tg", details["utm_medium"])
	require.Equal(t, "spring2026", details["utm_campaign"])
	require.Equal(t, "ugc", details["utm_term"])
	require.Equal(t, "banner", details["utm_content"])
}

// auditDetailsMap decodes AuditLogEntry.NewValue (interface{} from the
// generated client) into a string-keyed map for ad-hoc assertions on the
// payload. Fails the test if NewValue is not a JSON object — every audit row
// in this surface is supposed to carry one.
func auditDetailsMap(t *testing.T, entry *apiclient.AuditLogEntry) map[string]any {
	t.Helper()
	require.NotNil(t, entry)
	require.NotNil(t, entry.NewValue, "audit entry NewValue must not be nil")
	m, ok := entry.NewValue.(map[string]any)
	require.True(t, ok, "audit entry NewValue must decode as map, got %T", entry.NewValue)
	return m
}

// TestSubmitCreatorApplicationCFIP closes the GH-39 regression: behind the
// Cloudflare → Dokploy → Docker chain the backend used to log the edge or
// proxy IP into audit_logs.ip_address instead of the real client IP. The
// test sends a raw POST with CF-Connecting-IP set to a known fake address,
// then reads the audit row via the admin ListAuditLogs API and asserts the
// header value made it all the way through middleware → service → repo → DB.
//
// Skipped when E2E_BASE_URL points to a Cloudflare-fronted host: CF rewrites
// CF-Connecting-IP at the edge, so a client-supplied value never reaches the
// backend. The chain we want to validate is only observable against direct-
// to-origin (local docker compose) targets — staging E2E in CI hits the
// public Cloudflare hostname and would assert against the runner's IP.
func TestSubmitCreatorApplicationCFIP(t *testing.T) {
	t.Parallel()

	if !strings.Contains(testutil.BaseURL, "localhost") &&
		!strings.Contains(testutil.BaseURL, "127.0.0.1") {
		t.Skipf("CF-Connecting-IP test requires direct-to-origin target; BaseURL=%q is fronted by Cloudflare", testutil.BaseURL)
	}

	const fakeClientIP = "203.0.113.7"

	body := validRequestMap(testutil.UniqueIIN())
	// Raw HTTP — typed client cannot attach arbitrary proxy-chain headers.
	resp := testutil.PostRaw(t, "/creators/applications", body,
		testutil.WithHeader(testutil.HeaderCFConnectingIP, fakeClientIP))
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var envelope struct {
		Data struct {
			ApplicationID uuid.UUID `json:"applicationId"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&envelope))
	applicationID := envelope.Data.ApplicationID.String()
	testutil.RegisterCreatorApplicationCleanup(t, applicationID)

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	entry := testutil.FindAuditEntry(t, adminClient, adminToken,
		"creator_application", applicationID, "creator_application_submit")
	require.Equal(t, fakeClientIP, entry.IpAddress)
}

// TestGetCreatorApplicationForbidden verifies the security boundary: a
// brand_manager (legitimately authenticated) cannot read a creator
// application by id. Application is created via the public POST so the
// 403 is unambiguously about the caller's role — not a missing record.
func TestGetCreatorApplicationForbidden(t *testing.T) {
	t.Parallel()

	publicClient := testutil.NewAPIClient(t)
	iin := testutil.UniqueIIN()
	submit, err := publicClient.SubmitCreatorApplicationWithResponse(context.Background(), validRequest(iin))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, submit.StatusCode())
	require.NotNil(t, submit.JSON201)
	applicationID := submit.JSON201.Data.ApplicationId
	testutil.RegisterCreatorApplicationCleanup(t, applicationID.String())

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)
	brandID := testutil.SetupBrand(t, adminClient, adminToken, "Forbidden Brand "+iin)
	_, managerToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

	resp, err := adminClient.GetCreatorApplicationWithResponse(context.Background(), applicationID, testutil.WithAuth(managerToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode())
	require.NotNil(t, resp.JSON403)
	require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
}

// TestGetCreatorApplicationNotFound asserts that a syntactically valid UUID
// that does not match any application returns 404 NOT_FOUND. We deliberately
// use uuid.New() — pgx would surface a different error class for invalid
// UUID syntax, which is HandleParamError territory and out of scope.
func TestGetCreatorApplicationNotFound(t *testing.T) {
	t.Parallel()

	adminClient, adminToken, _ := testutil.SetupAdminClient(t)

	resp, err := adminClient.GetCreatorApplicationWithResponse(context.Background(), uuid.New(), testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode())
	require.NotNil(t, resp.JSON404)
	require.Equal(t, "NOT_FOUND", resp.JSON404.Error.Code)
}

// verifyCreatorApplicationByID reads the aggregate via admin GET and asserts
// that it matches the request, dictionary-resolved City/Categories, and the
// canonical four-consent layout. Dynamic fields (id, timestamps, ipAddress,
// userAgent, documentVersion) are validated independently, then normalised
// onto the expected canonical values so the final require.Equal compares the
// whole apiclient.CreatorApplicationDetailData struct in one shot — exactly
// the pattern docs/standards/backend-testing-e2e.md prescribes.
func verifyCreatorApplicationByID(t *testing.T, c *apiclient.ClientWithResponses, adminToken, applicationID string, req apiclient.CreatorApplicationSubmitRequest, otherText string) {
	t.Helper()
	appUUID, err := uuid.Parse(applicationID)
	require.NoError(t, err)

	resp, err := c.GetCreatorApplicationWithResponse(context.Background(), appUUID, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	got := resp.JSON200.Data

	// Dynamic-field assertions: id is the one we created, timestamps land
	// inside a sane window, every consent carries non-empty server-derived
	// fields. Failure here points at a real bug; if everything passes we
	// neutralise these fields and let require.Equal do the structural check.
	require.Equal(t, applicationID, got.Id.String())
	require.WithinDuration(t, time.Now().UTC(), got.CreatedAt, 5*time.Minute)
	require.WithinDuration(t, time.Now().UTC(), got.UpdatedAt, 5*time.Minute)
	require.Regexp(t, `^UGC-\d{6}$`, got.VerificationCode,
		"verificationCode must be UGC-NNNNNN, got %q", got.VerificationCode)
	require.True(t, strings.HasPrefix(got.TelegramBotUrl, "https://t.me/"),
		"telegram bot url should start with https://t.me/, got %q", got.TelegramBotUrl)
	require.Contains(t, got.TelegramBotUrl, "?start="+applicationID,
		"telegram bot url must carry the application id as start parameter")
	require.Len(t, got.Consents, 4)
	for i := range got.Consents {
		require.WithinDuration(t, time.Now().UTC(), got.Consents[i].AcceptedAt, 5*time.Minute,
			"consent %d acceptedAt out of range", i)
		require.NotEmpty(t, got.Consents[i].IpAddress, "consent %d ipAddress empty", i)
		require.NotEmpty(t, got.Consents[i].UserAgent, "consent %d userAgent empty", i)
		require.NotEmpty(t, got.Consents[i].DocumentVersion, "consent %d documentVersion empty", i)
	}
	// Newly-submitted application: every social row must come back unverified
	// with the three companion fields (method/by/at) set to null.
	for i, soc := range got.Socials {
		require.False(t, soc.Verified, "social[%d].verified must default to false", i)
		require.Nil(t, soc.Method, "social[%d].method must be nil for an unverified row", i)
		require.Nil(t, soc.VerifiedByUserId, "social[%d].verifiedByUserId must be nil for an unverified row", i)
		require.Nil(t, soc.VerifiedAt, "social[%d].verifiedAt must be nil for an unverified row", i)
	}

	// Build the full expected aggregate with City/Categories resolved against
	// the live public dictionaries — the same source the read-side handler
	// queries — so name/sortOrder line up without us hardcoding seed values.
	expected := buildExpectedDetail(t, req, otherText, got)

	require.Equal(t, expected, got)
}

// buildExpectedDetail constructs the canonical apiclient.CreatorApplicationDetailData
// the GET handler must return for the given submission. Dynamic fields (id,
// timestamps, consent server-stamped values) are copied from `got` after the
// caller has validated them so equality holds for the rest of the structure.
func buildExpectedDetail(t *testing.T, req apiclient.CreatorApplicationSubmitRequest, otherText string, got apiclient.CreatorApplicationDetailData) apiclient.CreatorApplicationDetailData {
	t.Helper()
	publicClient := testutil.NewAPIClient(t)
	cityRef := resolveCityRef(t, publicClient, req.City)
	catRefs := resolveCategoryRefs(t, publicClient, req.Categories)

	socs := make([]apiclient.CreatorApplicationDetailSocial, len(req.Socials))
	for i, s := range req.Socials {
		handle := strings.ToLower(strings.TrimLeft(strings.TrimSpace(s.Handle), "@"))
		socs[i] = apiclient.CreatorApplicationDetailSocial{
			Platform: apiclient.SocialPlatform(s.Platform),
			Handle:   handle,
		}
	}
	sortSocials(socs)

	// Social IDs are server-assigned UUIDs. They are not derivable from the
	// submission request, so copy them from `got` after the per-row sorts
	// align — same pattern as ApplicationId / CreatedAt above.
	require.Len(t, got.Socials, len(socs), "got/expected social row count must match")
	for i := range socs {
		socs[i].Id = got.Socials[i].Id
	}

	var otherPtr *string
	if otherText != "" {
		s := otherText
		otherPtr = &s
	}

	expected := apiclient.CreatorApplicationDetailData{
		Id:                got.Id,
		LastName:          req.LastName,
		FirstName:         req.FirstName,
		MiddleName:        req.MiddleName,
		Iin:               req.Iin,
		BirthDate:         got.BirthDate, // verified below to match the IIN's YYMMDD prefix; trust the parsed value
		Phone:             req.Phone,
		City:              cityRef,
		Address:           req.Address,
		CategoryOtherText: otherPtr,
		Status:            apiclient.Verification,
		// VerificationCode is generated server-side per submission; shape is
		// asserted in verifyCreatorApplicationByID — copy through here so the
		// equality check stays exhaustive without freezing the random suffix.
		VerificationCode: got.VerificationCode,
		CreatedAt:        got.CreatedAt,
		UpdatedAt:        got.UpdatedAt,
		Categories:       catRefs,
		Socials:          socs,
		Consents:         buildExpectedConsents(got.Consents),
		// telegramBotUrl shape (https://t.me/{bot}?start={id}) is asserted in
		// verifyCreatorApplicationByID against the live TELEGRAM_BOT_USERNAME;
		// the username itself is environment-specific so we copy through here.
		TelegramBotUrl: got.TelegramBotUrl,
	}

	// Verify birth_date derives from the IIN's YYMMDD prefix so a regression
	// in the IIN→date conversion path surfaces here, not silently in the
	// require.Equal copy from `got`. UniqueIIN draws year/month/day at random,
	// so we read the expected date out of the IIN itself.
	require.Equal(t, iinBirthDate(t, req.Iin), got.BirthDate.Format("2006-01-02"))

	return expected
}

// iinBirthDate decodes the YYMMDD prefix and century byte (positions 0..6) of
// a Kazakhstani IIN into the canonical YYYY-MM-DD string. Century byte 3..4
// flags 1900s, 5..6 flags 2000s — matching the format produced by
// testutil.UniqueIIN.
func iinBirthDate(t *testing.T, iin string) string {
	t.Helper()
	require.Len(t, iin, 12)
	yy, err := strconv.Atoi(iin[0:2])
	require.NoError(t, err)
	mm, err := strconv.Atoi(iin[2:4])
	require.NoError(t, err)
	dd, err := strconv.Atoi(iin[4:6])
	require.NoError(t, err)
	century := iin[6]
	year := 1900 + yy
	if century == '5' || century == '6' {
		year = 2000 + yy
	}
	return fmt.Sprintf("%04d-%02d-%02d", year, mm, dd)
}

// resolveCityRef looks the city up in the public cities dictionary and
// returns the same struct the GET handler would emit. Uses the same data
// source as the server, so name/sortOrder match exactly.
func resolveCityRef(t *testing.T, c *apiclient.ClientWithResponses, code string) apiclient.DictionaryItem {
	t.Helper()
	resp, err := c.ListDictionaryWithResponse(context.Background(), apiclient.Cities)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	for _, item := range resp.JSON200.Data.Items {
		if item.Code == code {
			return apiclient.DictionaryItem{Code: item.Code, Name: item.Name, SortOrder: item.SortOrder}
		}
	}
	require.Failf(t, "city not found in dictionary", "code=%q", code)
	return apiclient.DictionaryItem{}
}

// resolveCategoryRefs maps the requested category codes to their dictionary
// entries and returns them in the same (sortOrder, code) order the GET
// handler does. Failures here mean the test is asking for codes that do not
// (yet) exist in the seed.
func resolveCategoryRefs(t *testing.T, c *apiclient.ClientWithResponses, codes []string) []apiclient.DictionaryItem {
	t.Helper()
	resp, err := c.ListDictionaryWithResponse(context.Background(), apiclient.Categories)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	byCode := make(map[string]apiclient.DictionaryItem, len(resp.JSON200.Data.Items))
	for _, item := range resp.JSON200.Data.Items {
		byCode[item.Code] = item
	}
	out := make([]apiclient.DictionaryItem, 0, len(codes))
	for _, code := range codes {
		entry, ok := byCode[code]
		require.Truef(t, ok, "category %q not found in dictionary", code)
		out = append(out, apiclient.DictionaryItem{
			Code:      entry.Code,
			Name:      entry.Name,
			SortOrder: entry.SortOrder,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// buildExpectedConsents returns four consent rows in canonical order, copying
// server-stamped fields (acceptedAt, ipAddress, userAgent, documentVersion)
// from `got` after the caller has validated them — the read-side reorders to
// canonical sequence regardless of how Postgres stored them.
func buildExpectedConsents(got []apiclient.CreatorApplicationDetailConsent) []apiclient.CreatorApplicationDetailConsent {
	canonical := []apiclient.ConsentType{
		apiclient.Processing,
		apiclient.ThirdParty,
		apiclient.CrossBorder,
		apiclient.Terms,
	}
	out := make([]apiclient.CreatorApplicationDetailConsent, len(canonical))
	for i, ct := range canonical {
		out[i] = apiclient.CreatorApplicationDetailConsent{
			ConsentType:     ct,
			AcceptedAt:      got[i].AcceptedAt,
			DocumentVersion: got[i].DocumentVersion,
			IpAddress:       got[i].IpAddress,
			UserAgent:       got[i].UserAgent,
		}
	}
	return out
}

// sortSocials orders socials by (platform, handle) — the same key the server
// uses when reading the aggregate.
func sortSocials(s []apiclient.CreatorApplicationDetailSocial) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Platform != s[j].Platform {
			return string(s[i].Platform) < string(s[j].Platform)
		}
		return s[i].Handle < s[j].Handle
	})
}

// validRequestMap is a raw-map variant of the valid request used to drive
// PostRaw-based tests that need to send fields the typed client refuses to
// serialise (empty strings, malformed enums, etc.).
func validRequestMap(iin string) map[string]any {
	return map[string]any{
		"lastName":   "Муратова",
		"firstName":  "Айдана",
		"middleName": "Ивановна",
		"iin":        iin,
		"phone":      "+77001234567",
		"city":       "almaty",
		"categories": []string{"beauty", "fashion"},
		"socials": []map[string]string{
			{"platform": "instagram", "handle": "@aidana_" + iin[7:]},
			{"platform": "tiktok", "handle": "aidana_tt_" + iin[7:]},
		},
		"acceptedAll": true,
	}
}

// flipDigit returns a single-digit string different from r. Trivial helper
// that keeps the checksum-breaking test readable.
func flipDigit(r byte) string {
	if r == '0' {
		return "1"
	}
	return string(r - 1)
}

// buildUnderageIIN produces a checksum-valid IIN for a creator who will
// always be a couple of years short of MinCreatorAge against the backend's
// real-time clock — regardless of when the test runs. Clock-independent:
// a hardcoded year would stop reproducing under-age the moment real time
// caught up.
func buildUnderageIIN() string {
	const minAge = 18 // mirrors domain.MinCreatorAge
	birth := time.Now().UTC().AddDate(-(minAge - 2), 0, 0)
	yy := fmt.Sprintf("%02d", birth.Year()%100)
	mm := fmt.Sprintf("%02d", int(birth.Month()))
	dd := fmt.Sprintf("%02d", birth.Day())
	// Century byte 5/6 → 2000s; pick whichever fits a valid checksum.
	for _, century := range []string{"5", "6"} {
		for {
			serial := testutil.UniqueIIN()[7:11]
			prefix := yy + mm + dd + century + serial
			if last, ok := iinControlForTests(prefix); ok {
				return fmt.Sprintf("%s%d", prefix, last)
			}
		}
	}
	panic("buildUnderageIIN: failed to find a valid checksum")
}

// iinControlForTests duplicates the algorithm from testutil.iinControl because
// that symbol is unexported. Small copy is fine — the logic is stable and
// covered by the domain unit tests.
func iinControlForTests(first11 string) (int, bool) {
	digits := make([]int, 11)
	for i, r := range first11 {
		digits[i] = int(r - '0')
	}
	w1 := [11]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	sum := 0
	for i := 0; i < 11; i++ {
		sum += digits[i] * w1[i]
	}
	mod := sum % 11
	if mod == 10 {
		w2 := [11]int{3, 4, 5, 6, 7, 8, 9, 10, 11, 1, 2}
		sum2 := 0
		for i := 0; i < 11; i++ {
			sum2 += digits[i] * w2[i]
		}
		mod = sum2 % 11
		if mod == 10 {
			return 0, false
		}
	}
	return mod, true
}
