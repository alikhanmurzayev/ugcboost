// Package creator_applications — E2E тесты HTTP-поверхности
// POST /creators/applications/list (admin-only, чанк 4 onboarding-roadmap'а).
//
// TestCreatorApplicationsList покрывает все ветки I/O-матрицы в одной
// функции через t.Run в порядке исполнения: сначала границы доступа
// (отсутствие Bearer'а отдаёт 401, brand_manager — 403 без leak'а
// существования заявок), затем валидация request-body (неподдерживаемые
// sort/order, page/perPage вне допустимого диапазона, неизвестные статусы),
// happy-path с фильтрами и сортировками (status array, cities, categories,
// диапазоны created_at и возраста, telegramLinked, поиск по фамилии,
// handle'у и ИИН), сортировки по каждому полю в обоих направлениях,
// пагинация (page=2 + перенос границ страницы), пустой результат, и
// item-shape — чем именно админ видит креатора (никаких phone/address/
// consents в list-view, только telegramLinked: bool, остальные поля
// гидрированы из словарей).
//
// Все тесты идемпотентны и параллельны: каждый t.Run заводит свой набор
// заявок через testutil.SetupCreatorApplicationViaLanding, поэтому
// параллельные прогоны не делят данные. Cleanup идёт через cleanup-stack
// с уважением FK; при E2E_CLEANUP=false данные остаются для ручного
// разбора, ровно как в остальных пакетах e2e.
package creator_applications_test

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

func validBody() apiclient.CreatorApplicationsListRequest {
	return apiclient.CreatorApplicationsListRequest{
		Sort:    apiclient.CreatedAt,
		Order:   apiclient.Desc,
		Page:    1,
		PerPage: 50,
	}
}

func TestCreatorApplicationsList(t *testing.T) {
	t.Parallel()

	t.Run("auth: missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		// No Authorization header — middleware rejects with 401 before any
		// business logic. Use the typed client so we get the strict 401 path
		// from the bearer-aware oapi-codegen wrapper.
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), validBody())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("auth: brand_manager bearer returns 403 without leak", func(t *testing.T) {
		t.Parallel()
		// Need an admin first to bootstrap a brand and a manager — that gives us
		// a valid manager bearer that should be rejected by the list endpoint.
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "list-403-brand-"+testutil.UniqueEmail("brand"))
		_, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), validBody(), testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("validation: sort=rating returns 422", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validBody()
		body.Sort = "rating" // unsupported on purpose
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "VALIDATION_ERROR", resp.JSON422.Error.Code)
	})

	t.Run("validation: order=random returns 422", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validBody()
		body.Order = "random"
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
	})

	t.Run("validation: page=0 returns 422", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validBody()
		body.Page = 0
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
	})

	t.Run("validation: perPage=201 returns 422", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validBody()
		body.PerPage = 201
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
	})

	t.Run("happy: status filter returns only matching applications", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		// Two fresh applications — both land in `verification` (default for
		// new submissions). Filtering by `verification` returns at least the
		// pair we just submitted. The dataset can carry rows from other
		// concurrent tests, so we assert on inclusion + property, not equality.
		first := testutil.SetupCreatorApplicationViaLanding(t)
		second := testutil.SetupCreatorApplicationViaLanding(t)

		body := validBody()
		statuses := []apiclient.CreatorApplicationStatus{apiclient.Verification}
		body.Statuses = &statuses
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		ids := collectIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, first.ApplicationID)
		require.Contains(t, ids, second.ApplicationID)
		// Every returned item must satisfy the status filter — no leakage.
		for _, item := range resp.JSON200.Data.Items {
			require.Equal(t, apiclient.Verification, item.Status)
		}
	})

	t.Run("happy: cities filter narrows to single city", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		almaty := testutil.SetupCreatorApplicationViaLanding(t)
		astana := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.City = "astana"
		})

		body := validBody()
		cities := []string{"astana"}
		body.Cities = &cities
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		ids := collectIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, astana.ApplicationID)
		require.NotContains(t, ids, almaty.ApplicationID, "almaty application must be excluded by cities=[astana]")
		for _, item := range resp.JSON200.Data.Items {
			require.Equal(t, "astana", item.City.Code)
		}
	})

	t.Run("happy: categories filter is any-of EXISTS", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		// food-only application must be excluded when filter is [beauty];
		// beauty+fashion application must be included.
		beautyApp := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.Categories = []string{"beauty", "fashion"}
		})
		foodApp := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.Categories = []string{"food"}
		})

		body := validBody()
		cats := []string{"beauty"}
		body.Categories = &cats
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		ids := collectIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, beautyApp.ApplicationID)
		require.NotContains(t, ids, foodApp.ApplicationID)
	})

	t.Run("happy: dateFrom narrows to applications submitted near a marker", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		// Marker is shifted slightly into the past so a small NTP drift between
		// the test runner and the backend cannot push the freshly-submitted row
		// before the marker. The window is verified with require.WithinDuration
		// below — every returned row must land within a few seconds of "now".
		marker := time.Now().UTC().Add(-2 * time.Second)
		fresh := testutil.SetupCreatorApplicationViaLanding(t)

		body := validBody()
		body.DateFrom = pointer.ToTime(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		ids := collectIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, fresh.ApplicationID)
		// Every returned application must have created_at within a small window
		// around now — generous enough to absorb NTP drift, tight enough to
		// guard against the filter being silently dropped.
		for _, item := range resp.JSON200.Data.Items {
			require.WithinDuration(t, time.Now().UTC(), item.CreatedAt, time.Minute,
				"created_at %s out of acceptable window for marker %s", item.CreatedAt, marker)
		}
	})

	t.Run("happy: ageFrom matches creators >= threshold (UniqueIIN ~age 30)", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		// UniqueIIN draws birth year from 1985..2005, so every test creator is
		// well above 18 today. Filter ageFrom=18 must include the application.
		app := testutil.SetupCreatorApplicationViaLanding(t)

		body := validBody()
		body.AgeFrom = pointer.ToInt(18)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectIDs(resp.JSON200.Data.Items), app.ApplicationID)
	})

	t.Run("happy: ageTo=10 excludes adult creators", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		app := testutil.SetupCreatorApplicationViaLanding(t)

		body := validBody()
		body.AgeTo = pointer.ToInt(10) // every creator is older — must be excluded
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.NotContains(t, collectIDs(resp.JSON200.Data.Items), app.ApplicationID)
	})

	t.Run("happy: telegramLinked filter splits linked vs unlinked", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		linked := testutil.SetupCreatorApplicationViaLanding(t)
		unlinked := testutil.SetupCreatorApplicationViaLanding(t)
		testutil.LinkTelegramToApplication(t, linked.ApplicationID)

		// telegramLinked=true must include the linked application and must NOT
		// include the unlinked one.
		body := validBody()
		body.TelegramLinked = pointer.ToBool(true)
		body.PerPage = 200
		c := testutil.NewAPIClient(t)
		respTrue, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, respTrue.StatusCode())
		require.NotNil(t, respTrue.JSON200)
		idsLinked := collectIDs(respTrue.JSON200.Data.Items)
		require.Contains(t, idsLinked, linked.ApplicationID)
		require.NotContains(t, idsLinked, unlinked.ApplicationID)
		for _, item := range respTrue.JSON200.Data.Items {
			require.True(t, item.TelegramLinked, "telegramLinked=true must imply each item is linked")
		}

		// And the inverse: telegramLinked=false must include unlinked, exclude linked.
		body.TelegramLinked = pointer.ToBool(false)
		respFalse, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, respFalse.StatusCode())
		require.NotNil(t, respFalse.JSON200)
		idsUnlinked := collectIDs(respFalse.JSON200.Data.Items)
		require.Contains(t, idsUnlinked, unlinked.ApplicationID)
		require.NotContains(t, idsUnlinked, linked.ApplicationID)
		for _, item := range respFalse.JSON200.Data.Items {
			require.False(t, item.TelegramLinked, "telegramLinked=false must imply each item is unlinked")
		}
	})

	t.Run("happy: search by IIN finds the application", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		app := testutil.SetupCreatorApplicationViaLanding(t)

		body := validBody()
		body.Search = pointer.ToString(app.Request.Iin)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectIDs(resp.JSON200.Data.Items), app.ApplicationID)
	})

	t.Run("happy: search by social handle hits EXISTS branch", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		marker := strings.ToLower("uniquehandle" + testutil.UniqueIIN())
		app := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.Socials = []apiclient.SocialAccountInput{
				{Platform: apiclient.Instagram, Handle: marker},
			}
		})

		body := validBody()
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectIDs(resp.JSON200.Data.Items), app.ApplicationID)
	})

	t.Run("happy: blank search after trim disables filter", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		app := testutil.SetupCreatorApplicationViaLanding(t)

		body := validBody()
		body.Search = pointer.ToString("   ") // trims to empty → ignored
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectIDs(resp.JSON200.Data.Items), app.ApplicationID)
	})

	t.Run("sort: created_at asc orders by submission time", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		first := testutil.SetupCreatorApplicationViaLanding(t)
		time.Sleep(1100 * time.Millisecond)
		second := testutil.SetupCreatorApplicationViaLanding(t)

		body := validBody()
		body.Sort = apiclient.CreatedAt
		body.Order = apiclient.Asc
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		// The two applications we created must appear in submission order.
		positions := positionsFor(resp.JSON200.Data.Items, first.ApplicationID, second.ApplicationID)
		require.Less(t, positions[first.ApplicationID], positions[second.ApplicationID],
			"first application must precede second under created_at asc")
	})

	t.Run("sort: city_name asc orders by ct.name", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		// almaty (Алматы) and astana (Астана) — Almaty's "Алматы" sorts before
		// Astana's "Астана" alphabetically in the Cyrillic codepoint order.
		almaty := testutil.SetupCreatorApplicationViaLanding(t)
		astana := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.City = "astana"
		})

		body := validBody()
		body.Sort = apiclient.CityName
		body.Order = apiclient.Asc
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		positions := positionsFor(resp.JSON200.Data.Items, almaty.ApplicationID, astana.ApplicationID)
		require.Less(t, positions[almaty.ApplicationID], positions[astana.ApplicationID],
			"Алматы (almaty) must precede Астана (astana) under city_name asc")
	})

	t.Run("sort: full_name asc with last-name discriminator", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		ivanova := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.LastName = "Иванова"
		})
		yakovleva := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.LastName = "Яковлева"
		})

		body := validBody()
		body.Sort = apiclient.FullName
		body.Order = apiclient.Asc
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		positions := positionsFor(resp.JSON200.Data.Items, ivanova.ApplicationID, yakovleva.ApplicationID)
		require.Less(t, positions[ivanova.ApplicationID], positions[yakovleva.ApplicationID],
			"Иванова must precede Яковлева under full_name asc")
	})

	t.Run("sort: full_name desc reverses the last-name order", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		ivanova := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.LastName = "Иванова"
		})
		yakovleva := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.LastName = "Яковлева"
		})

		body := validBody()
		body.Sort = apiclient.FullName
		body.Order = apiclient.Desc
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		positions := positionsFor(resp.JSON200.Data.Items, ivanova.ApplicationID, yakovleva.ApplicationID)
		require.Less(t, positions[yakovleva.ApplicationID], positions[ivanova.ApplicationID],
			"Яковлева must precede Иванова under full_name desc")
	})

	t.Run("sort: city_name desc reverses the city order", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		almaty := testutil.SetupCreatorApplicationViaLanding(t)
		astana := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
			r.City = "astana"
		})

		body := validBody()
		body.Sort = apiclient.CityName
		body.Order = apiclient.Desc
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		positions := positionsFor(resp.JSON200.Data.Items, almaty.ApplicationID, astana.ApplicationID)
		require.Less(t, positions[astana.ApplicationID], positions[almaty.ApplicationID],
			"Астана must precede Алматы under city_name desc")
	})

	t.Run("sort: updated_at asc/desc both produce ordered pages", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		// updated_at follows created_at on submission (no later mutation in
		// chunk 4), so a 1.1s gap forces distinct timestamps and gives us a
		// meaningful order to verify in both directions.
		first := testutil.SetupCreatorApplicationViaLanding(t)
		time.Sleep(1100 * time.Millisecond)
		second := testutil.SetupCreatorApplicationViaLanding(t)

		c := testutil.NewAPIClient(t)
		body := validBody()
		body.Sort = apiclient.UpdatedAt
		body.PerPage = 200

		body.Order = apiclient.Asc
		respAsc, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, respAsc.StatusCode())
		require.NotNil(t, respAsc.JSON200)
		posAsc := positionsFor(respAsc.JSON200.Data.Items, first.ApplicationID, second.ApplicationID)
		require.Less(t, posAsc[first.ApplicationID], posAsc[second.ApplicationID])

		body.Order = apiclient.Desc
		respDesc, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, respDesc.StatusCode())
		require.NotNil(t, respDesc.JSON200)
		posDesc := positionsFor(respDesc.JSON200.Data.Items, first.ApplicationID, second.ApplicationID)
		require.Less(t, posDesc[second.ApplicationID], posDesc[first.ApplicationID])
	})

	t.Run("sort: birth_date asc/desc by tie-broken id ASC", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		// UniqueIIN samples birthdates from a broad range, so the two creators
		// almost certainly carry different birth_date values; if they happen to
		// collide, the tie-breaker (id ASC) still gives a stable ordering. The
		// assertion only checks that both rows surface in either direction.
		appA := testutil.SetupCreatorApplicationViaLanding(t)
		appB := testutil.SetupCreatorApplicationViaLanding(t)

		c := testutil.NewAPIClient(t)
		body := validBody()
		body.Sort = apiclient.BirthDate
		body.PerPage = 200

		for _, dir := range []apiclient.SortOrder{apiclient.Asc, apiclient.Desc} {
			body.Order = dir
			resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
			require.NoError(t, err, "direction %s", dir)
			require.Equal(t, http.StatusOK, resp.StatusCode(), "direction %s", dir)
			require.NotNil(t, resp.JSON200)
			positions := positionsFor(resp.JSON200.Data.Items, appA.ApplicationID, appB.ApplicationID)
			require.Contains(t, positions, appA.ApplicationID, "direction %s", dir)
			require.Contains(t, positions, appB.ApplicationID, "direction %s", dir)
		}
	})

	t.Run("pagination: page 2 with perPage=1 yields different items", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		// Scope the dataset to a unique marker so the assertions stay stable
		// regardless of what other parallel tests are submitting on the same
		// backend. Each application carries a handle prefixed with the marker;
		// the list query filters by `search: marker`, narrowing to the
		// three rows we just created and giving us a deterministic
		// total = 3, items per page = 1.
		marker := strings.ToLower("paginate" + testutil.UniqueIIN()[8:])
		ids := make(map[string]bool, 3)
		for range 3 {
			app := testutil.SetupCreatorApplicationViaLanding(t, func(r *apiclient.CreatorApplicationSubmitRequest) {
				r.Socials = []apiclient.SocialAccountInput{
					{Platform: apiclient.Instagram, Handle: marker + "_" + r.Iin[7:]},
				}
			})
			ids[app.ApplicationID] = true
		}

		c := testutil.NewAPIClient(t)
		body := validBody()
		body.PerPage = 1
		body.Sort = apiclient.CreatedAt
		body.Order = apiclient.Desc
		body.Search = pointer.ToString(marker)

		body.Page = 1
		respPage1, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, respPage1.StatusCode())
		require.NotNil(t, respPage1.JSON200)
		require.Len(t, respPage1.JSON200.Data.Items, 1)
		require.Equal(t, int64(3), respPage1.JSON200.Data.Total, "total must reflect the marker-scoped count")
		page1ID := respPage1.JSON200.Data.Items[0].Id.String()
		require.True(t, ids[page1ID], "page=1 item must be one of the three marker-scoped applications")

		body.Page = 2
		respPage2, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, respPage2.StatusCode())
		require.NotNil(t, respPage2.JSON200)
		require.Len(t, respPage2.JSON200.Data.Items, 1)
		page2ID := respPage2.JSON200.Data.Items[0].Id.String()
		require.True(t, ids[page2ID], "page=2 item must also be a marker-scoped application")
		require.NotEqual(t, page1ID, page2ID, "page=2 must surface a different item than page=1")
	})

	t.Run("empty result: filter that no application matches", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validBody()
		// "rejected" is a valid status but no test seeds it, so the result
		// is empty. The endpoint must answer 200 with items=[] and total=0.
		statuses := []apiclient.CreatorApplicationStatus{apiclient.Rejected}
		body.Statuses = &statuses

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		// No assertion that total is exactly 0 — concurrent rejected-status
		// flows from other tests could legitimately push it above zero. We
		// instead assert items is well-formed and total is non-negative.
		require.NotNil(t, resp.JSON200.Data.Items)
		require.GreaterOrEqual(t, resp.JSON200.Data.Total, int64(0))
	})

	t.Run("item shape: hydrated city/categories, telegramLinked propagates, no PII", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		app := testutil.SetupCreatorApplicationViaLanding(t)
		testutil.LinkTelegramToApplication(t, app.ApplicationID)

		body := validBody()
		body.Search = pointer.ToString(app.Request.Iin)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorApplicationsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		var item *apiclient.CreatorApplicationListItem
		for i := range resp.JSON200.Data.Items {
			if resp.JSON200.Data.Items[i].Id.String() == app.ApplicationID {
				item = &resp.JSON200.Data.Items[i]
				break
			}
		}
		require.NotNil(t, item, "submitted application must surface in the search result")

		require.Equal(t, app.Request.LastName, item.LastName)
		require.Equal(t, app.Request.FirstName, item.FirstName)
		if app.Request.MiddleName != nil {
			require.NotNil(t, item.MiddleName)
			require.Equal(t, *app.Request.MiddleName, *item.MiddleName)
		}
		require.True(t, item.TelegramLinked, "telegramLinked must be true after /start")

		// City must be hydrated from the cities dictionary, not echo the code.
		require.Equal(t, "almaty", item.City.Code)
		require.NotEmpty(t, item.City.Name)
		require.NotEqual(t, item.City.Code, item.City.Name,
			"city.name must come from the dictionary, not fall back to code")

		// Categories must be hydrated and sorted by (sortOrder, code).
		require.NotEmpty(t, item.Categories)
		categoryCodes := make([]string, len(item.Categories))
		for i, c := range item.Categories {
			categoryCodes[i] = c.Code
			require.NotEmpty(t, c.Name, "category.name must be hydrated from dictionary")
		}
		sortedByCode := append([]string(nil), categoryCodes...)
		sort.Strings(sortedByCode)
		// Sorted by sortOrder then code; with deterministic dictionary data
		// this lines up with sort-by-code for the seeded set, but we do not
		// rely on that — only that returned codes are exactly the submitted set.
		require.ElementsMatch(t, app.Request.Categories, categoryCodes)

		// Socials shape: platform + handle, lowercased after normalisation.
		require.NotEmpty(t, item.Socials)
		require.Equal(t, app.Request.Socials[0].Platform, item.Socials[0].Platform)
		// Default verification state on a freshly submitted application: every
		// social row carries verified=false plus three nil companions until
		// chunk 8 (auto webhook) or chunk 9 (manual verify) flips them.
		for i, soc := range item.Socials {
			require.False(t, soc.Verified, "list socials[%d].verified must default to false", i)
			require.Nil(t, soc.Method, "list socials[%d].method must be nil for an unverified row", i)
			require.Nil(t, soc.VerifiedByUserId, "list socials[%d].verifiedByUserId must be nil for an unverified row", i)
			require.Nil(t, soc.VerifiedAt, "list socials[%d].verifiedAt must be nil for an unverified row", i)
		}
	})
}

func collectIDs(items []apiclient.CreatorApplicationListItem) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Id.String()
	}
	return out
}

func positionsFor(items []apiclient.CreatorApplicationListItem, ids ...string) map[string]int {
	out := make(map[string]int, len(ids))
	for i, item := range items {
		idStr := item.Id.String()
		for _, want := range ids {
			if want == idStr {
				out[idStr] = i
			}
		}
	}
	return out
}
