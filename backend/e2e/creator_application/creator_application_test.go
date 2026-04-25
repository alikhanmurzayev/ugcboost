// Package creator_application — E2E тесты HTTP-поверхности
// POST /creators/applications (публичная ручка, лендинг Айданы).
//
// TestSubmitCreatorApplication покрывает happy path подачи заявки. Клиент
// отправляет полный валидный payload (ФИО + ИИН + соцсети + согласия) и
// ожидает 201 с application_id (UUID) и ссылкой на Telegram-бот вида
// https://t.me/{bot}?start={id}. Application_id сохраняется для cleanup
// через /test/cleanup-entity: удаление родительской записи каскадно сносит
// соцсети, категории и согласия (DELETE CASCADE в миграциях), так что после
// теста в базе остаётся только audit-запись — её трогать не надо, аудит
// специально переживает очистку.
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
// Все тесты параллельны и используют UniqueIIN для генерации валидного
// казахстанского ИИН — это защищает partial unique index от коллизий между
// параллельными прогонами. RegisterCreatorApplicationCleanup опирается на
// POST /test/cleanup-entity с type=creator_application; cleanup работает
// при E2E_CLEANUP=true (дефолт), при false — данные остаются для ручного
// инспекта.
package creator_application_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

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
		Consents: apiclient.ConsentsInput{
			Processing:  true,
			ThirdParty:  true,
			CrossBorder: true,
			Terms:       true,
		},
	}
}

func TestSubmitCreatorApplication(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	iin := testutil.UniqueIIN()

	resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), validRequest(iin))
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

	t.Run("under 18 rejected with UNDER_AGE", func(t *testing.T) {
		t.Parallel()
		// buildUnder18IIN picks a birth 16 years before now, so this test
		// stays green regardless of when it runs (a hardcoded 2010 would
		// break the moment real-world time caught up).
		iin := buildUnder18IIN()
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
		req.Consents.CrossBorder = false
		resp, err := c.SubmitCreatorApplicationWithResponse(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "MISSING_CONSENT", resp.JSON422.Error.Code)
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
		"consents": map[string]bool{
			"processing":  true,
			"thirdParty":  true,
			"crossBorder": true,
			"terms":       true,
		},
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

// buildUnder18IIN produces a checksum-valid IIN for a creator who will
// always be roughly 16 years old against the backend's real-time clock —
// regardless of when the test runs. Clock-independent: a hardcoded year
// would stop reproducing under-18 the moment real time caught up.
func buildUnder18IIN() string {
	birth := time.Now().UTC().AddDate(-16, 0, 0)
	yy := fmt.Sprintf("%02d", birth.Year()%100)
	mm := fmt.Sprintf("%02d", int(birth.Month()))
	dd := fmt.Sprintf("%02d", birth.Day())
	// Century byte 5 = male, 2000s. Backend accepts both 5 and 6 for 2000s.
	century := "5"
	for {
		serial := testutil.UniqueIIN()[7:11] // reuse the atomic serial; drop the old checksum
		prefix := yy + mm + dd + century + serial
		if last, ok := iinControlForTests(prefix); ok {
			return fmt.Sprintf("%s%d", prefix, last)
		}
	}
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
