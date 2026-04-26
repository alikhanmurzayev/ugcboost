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
// с активной заявкой (pending) повторная подача отвергается 409
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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// validRequest builds a request body that satisfies every server-side check
// except for the uniqueness of the IIN, which the caller decides.
func validRequest(iin string) apiclient.CreatorApplicationSubmitRequest {
	middle := "Ивановна"
	return apiclient.CreatorApplicationSubmitRequest{
		LastName:   "Муратова",
		FirstName:  "Айдана",
		MiddleName: &middle,
		Iin:        iin,
		Phone:      "+77001234567",
		City:       "Алматы",
		Address:    "ул. Абая 1",
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
	verifyCreatorApplicationByID(t, adminClient, adminToken, data.ApplicationId.String(), expectedFromRequest(req, ""))
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
	req.City = "Астана"
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
	verifyCreatorApplicationByID(t, adminClient, adminToken, resp.JSON201.Data.ApplicationId.String(), expectedFromRequest(req, ""))
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
	verifyCreatorApplicationByID(t, adminClient, adminToken, resp.JSON201.Data.ApplicationId.String(), expectedFromRequest(req, other))
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

// expectedCreatorApplication is the verification target for
// verifyCreatorApplicationByID. The struct mirrors the fields
// the GET aggregate must echo back after a successful submit; dynamic data
// (id, timestamps, ipAddress/userAgent, dictionary names) is checked
// separately by the helper.
type expectedCreatorApplication struct {
	LastName          string
	FirstName         string
	MiddleName        string
	IIN               string
	Phone             string
	City              string
	Address           string
	CategoryOtherText string
	CategoryCodes     []string
	Socials           []apiclient.CreatorApplicationDetailSocial
}

// expectedFromRequest distils the submission payload into the verification
// shape: handle normalisation (lowercase, leading '@' stripped) is applied
// upfront so the helper doesn't need to know how the service mutates input.
func expectedFromRequest(req apiclient.CreatorApplicationSubmitRequest, otherText string) expectedCreatorApplication {
	socs := make([]apiclient.CreatorApplicationDetailSocial, len(req.Socials))
	for i, s := range req.Socials {
		handle := strings.ToLower(strings.TrimLeft(strings.TrimSpace(s.Handle), "@"))
		socs[i] = apiclient.CreatorApplicationDetailSocial{
			Platform: apiclient.SocialPlatform(s.Platform),
			Handle:   handle,
		}
	}
	// GET response sorts socials by (platform, handle); keep expected slice
	// in that same order so require.Equal lines up.
	sortSocials(socs)
	middle := ""
	if req.MiddleName != nil {
		middle = *req.MiddleName
	}
	return expectedCreatorApplication{
		LastName:          req.LastName,
		FirstName:         req.FirstName,
		MiddleName:        middle,
		IIN:               req.Iin,
		Phone:             req.Phone,
		City:              req.City,
		Address:           req.Address,
		CategoryOtherText: otherText,
		CategoryCodes:     append([]string(nil), req.Categories...),
		Socials:           socs,
	}
}

// sortSocials orders socials by (platform, handle) — the same key the server
// uses when reading the aggregate.
func sortSocials(s []apiclient.CreatorApplicationDetailSocial) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0; j-- {
			a, b := s[j-1], s[j]
			if string(a.Platform) > string(b.Platform) ||
				(a.Platform == b.Platform && a.Handle > b.Handle) {
				s[j-1], s[j] = b, a
				continue
			}
			break
		}
	}
}

// verifyCreatorApplicationByID reads the aggregate via admin GET and asserts
// that it matches the expected payload. Dynamic fields (id, createdAt,
// updatedAt, acceptedAt, ipAddress, userAgent, category Name/SortOrder)
// are checked independently and not part of the structural comparison.
func verifyCreatorApplicationByID(t *testing.T, c *apiclient.ClientWithResponses, adminToken, applicationID string, expected expectedCreatorApplication) {
	t.Helper()
	appUUID, err := uuid.Parse(applicationID)
	require.NoError(t, err)

	resp, err := c.GetCreatorApplicationWithResponse(context.Background(), appUUID, testutil.WithAuth(adminToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	got := resp.JSON200.Data

	require.Equal(t, applicationID, got.Id.String())
	require.WithinDuration(t, time.Now().UTC(), got.CreatedAt, 5*time.Minute)
	require.WithinDuration(t, time.Now().UTC(), got.UpdatedAt, 5*time.Minute)

	require.Equal(t, expected.LastName, got.LastName)
	require.Equal(t, expected.FirstName, got.FirstName)
	if expected.MiddleName == "" {
		require.Nil(t, got.MiddleName)
	} else {
		require.NotNil(t, got.MiddleName)
		require.Equal(t, expected.MiddleName, *got.MiddleName)
	}
	require.Equal(t, expected.IIN, got.Iin)
	require.Equal(t, expected.Phone, got.Phone)
	require.Equal(t, expected.City, got.City)
	require.Equal(t, expected.Address, got.Address)
	require.Equal(t, "1995-05-15", got.BirthDate.Format("2006-01-02"))
	require.Equal(t, apiclient.Pending, got.Status)

	if expected.CategoryOtherText == "" {
		require.Nil(t, got.CategoryOtherText)
	} else {
		require.NotNil(t, got.CategoryOtherText)
		require.Equal(t, expected.CategoryOtherText, *got.CategoryOtherText)
	}

	gotCodes := make([]string, len(got.Categories))
	for i, c := range got.Categories {
		gotCodes[i] = c.Code
	}
	require.ElementsMatch(t, expected.CategoryCodes, gotCodes)
	for i := 1; i < len(got.Categories); i++ {
		prev, cur := got.Categories[i-1], got.Categories[i]
		require.True(t, prev.SortOrder < cur.SortOrder ||
			(prev.SortOrder == cur.SortOrder && prev.Code < cur.Code),
			"categories not in (sort_order, code) order: %v then %v", prev, cur)
	}

	require.Equal(t, expected.Socials, got.Socials)

	require.Len(t, got.Consents, 4)
	require.Equal(t, []apiclient.CreatorApplicationDetailConsentConsentType{
		apiclient.Processing,
		apiclient.ThirdParty,
		apiclient.CrossBorder,
		apiclient.Terms,
	}, []apiclient.CreatorApplicationDetailConsentConsentType{
		got.Consents[0].ConsentType,
		got.Consents[1].ConsentType,
		got.Consents[2].ConsentType,
		got.Consents[3].ConsentType,
	})
	for _, c := range got.Consents {
		require.WithinDuration(t, time.Now().UTC(), c.AcceptedAt, 5*time.Minute)
		require.NotEmpty(t, c.DocumentVersion)
		require.NotEmpty(t, c.IpAddress)
		require.NotEmpty(t, c.UserAgent)
	}
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
		"city":       "Алматы",
		"address":    "ул. Абая 1",
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
	const minAge = 21 // mirrors domain.MinCreatorAge
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
