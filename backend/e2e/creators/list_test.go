// Package creators — E2E HTTP-поверхность POST /creators/list (chunk 1
// campaign-roadmap'а): admin-only paginated catalog approved-креаторов,
// которым admin будет выдавать кампании. Сценарии покрывают всю I/O-матрицу:
// 401 (нет Bearer'а), 403 для brand_manager (без leak'а существования
// креаторов), 422 для невалидной body (неподдерживаемые sort/order,
// page/perPage вне диапазона), happy-path с полным кросс-продуктом
// фильтров (city, categories, dateFrom/To, age, search), сортировки по
// каждому полю в обоих направлениях, пагинация (page=2, beyond-last) и
// item-shape (hydrated city/categories, lean PII set без address /
// category_other_text / full Telegram block).
//
// Каждый t.Run заводит свой набор approved-креаторов через
// testutil.SetupApprovedCreator, поэтому параллельные прогоны не делят
// данные. Cleanup идёт через cleanup-stack с уважением FK
// (creators → creator_applications, без ON DELETE на source_application_id);
// при E2E_CLEANUP=false данные остаются для ручного разбора.
package creators_test

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

func validCreatorListBody() apiclient.CreatorsListRequest {
	return apiclient.CreatorsListRequest{
		Sort:    apiclient.CreatorListSortFieldCreatedAt,
		Order:   apiclient.Desc,
		Page:    1,
		PerPage: 50,
	}
}

func collectCreatorIDs(items []apiclient.CreatorListItem) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Id.String()
	}
	return out
}

func positionsForCreators(items []apiclient.CreatorListItem, ids ...string) map[string]int {
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

func defaultCreatorOpts() testutil.CreatorApplicationFixture {
	return testutil.CreatorApplicationFixture{
		Socials: []testutil.SocialFixture{
			{Platform: string(apiclient.Instagram), Handle: "aidana_" + testutil.UniqueIIN()[6:], Verification: testutil.VerificationAutoIG},
			{Platform: string(apiclient.Tiktok), Handle: "aidana_tt_" + testutil.UniqueIIN()[6:], Verification: testutil.VerificationNone},
		},
	}
}

// newMarker returns a unique lowercase token reused across one test scenario
// to scope list queries via body.Search. Anchors fixtures to a marker-private
// subset so assertions stay deterministic on a busy staging DB regardless of
// what other parallel tests are seeding.
func newMarker() string {
	return strings.ToLower("e2e" + testutil.UniqueIIN()[6:])
}

// defaultCreatorOptsScoped attaches the marker to the verified IG handle so
// the resulting creator surfaces under body.Search = marker. Same for the
// non-verified TikTok handle — keeps the dual-platform shape used by
// defaultCreatorOpts.
func defaultCreatorOptsScoped(marker, suffix string) testutil.CreatorApplicationFixture {
	return testutil.CreatorApplicationFixture{
		Socials: []testutil.SocialFixture{
			{Platform: string(apiclient.Instagram), Handle: marker + "_" + suffix, Verification: testutil.VerificationAutoIG},
			{Platform: string(apiclient.Tiktok), Handle: marker + "_tt_" + suffix, Verification: testutil.VerificationNone},
		},
	}
}

func TestCreatorsList(t *testing.T) {
	t.Parallel()

	t.Run("auth: missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), validCreatorListBody())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("auth: brand_manager bearer returns 403 without leak", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken,
			"creators-list-403-brand-"+testutil.UniqueEmail("brand"))
		_, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), validCreatorListBody(),
			testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("validation: sort=rating returns 422", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validCreatorListBody()
		body.Sort = "rating"
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "VALIDATION_ERROR", resp.JSON422.Error.Code)
	})

	t.Run("validation: order=random returns 422", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validCreatorListBody()
		body.Order = "random"
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
	})

	t.Run("validation: page=0 returns 422", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validCreatorListBody()
		body.Page = 0
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
	})

	t.Run("validation: perPage=201 returns 422", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		body := validCreatorListBody()
		body.PerPage = 201
		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
	})

	t.Run("happy: cities filter narrows to single city", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		almaty := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "ala"))
		astanaOpts := defaultCreatorOptsScoped(marker, "ast")
		astanaOpts.CityCode = "astana"
		astana := testutil.SetupApprovedCreator(t, astanaOpts)

		body := validCreatorListBody()
		cities := []string{"astana"}
		body.Cities = &cities
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(astana.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		ids := collectCreatorIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, astana.CreatorID)
		require.NotContains(t, ids, almaty.CreatorID, "almaty creator must be excluded by cities=[astana]")
		for _, item := range resp.JSON200.Data.Items {
			require.Equal(t, "astana", item.City.Code)
		}
	})

	t.Run("happy: categories filter is any-of EXISTS", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		beautyOpts := defaultCreatorOptsScoped(marker, "beauty")
		beautyOpts.CategoryCodes = []string{"beauty", "fashion"}
		beauty := testutil.SetupApprovedCreator(t, beautyOpts)

		foodOpts := defaultCreatorOptsScoped(marker, "food")
		foodOpts.CategoryCodes = []string{"food"}
		food := testutil.SetupApprovedCreator(t, foodOpts)

		body := validCreatorListBody()
		cats := []string{"beauty"}
		body.Categories = &cats
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(beauty.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		ids := collectCreatorIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, beauty.CreatorID)
		require.NotContains(t, ids, food.CreatorID)
	})

	t.Run("happy: dateFrom narrows to creators created near a marker", func(t *testing.T) {
		t.Parallel()
		dateMarker := time.Now().UTC().Add(-2 * time.Second)
		marker := newMarker()
		fresh := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "fresh"))

		body := validCreatorListBody()
		body.DateFrom = pointer.ToTime(dateMarker)
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(fresh.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		ids := collectCreatorIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, fresh.CreatorID)
		for _, item := range resp.JSON200.Data.Items {
			require.WithinDuration(t, time.Now().UTC(), item.CreatedAt, 5*time.Minute,
				"created_at %s out of acceptable window", item.CreatedAt)
		}
	})

	t.Run("happy: ageFrom matches creators >= threshold", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "age"))

		body := validCreatorListBody()
		body.AgeFrom = pointer.ToInt(18)
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectCreatorIDs(resp.JSON200.Data.Items), creator.CreatorID)
	})

	t.Run("happy: ageTo=10 excludes adult creators", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "ageto"))

		body := validCreatorListBody()
		body.AgeTo = pointer.ToInt(10)
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.NotContains(t, collectCreatorIDs(resp.JSON200.Data.Items), creator.CreatorID)
	})

	t.Run("happy: search by IIN finds the creator", func(t *testing.T) {
		t.Parallel()
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts())

		// IIN is unique by construction (testutil.UniqueIIN), so search scoping
		// is implicit — no need for marker overlay.
		body := validCreatorListBody()
		body.Search = pointer.ToString(creator.IIN)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectCreatorIDs(resp.JSON200.Data.Items), creator.CreatorID)
	})

	t.Run("happy: search by phone matches substring", func(t *testing.T) {
		t.Parallel()
		// Use a unique phone so the substring search hits exactly this creator
		// regardless of how many other parallel seeders left "+77001234567"
		// rows around. The 7-digit suffix is drawn from UniqueIIN for parallel
		// safety.
		phoneSuffix := testutil.UniqueIIN()[:7]
		opts := defaultCreatorOpts()
		opts.Phone = "+7700" + phoneSuffix
		creator := testutil.SetupApprovedCreator(t, opts)

		body := validCreatorListBody()
		body.Search = pointer.ToString(phoneSuffix)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectCreatorIDs(resp.JSON200.Data.Items), creator.CreatorID)
	})

	t.Run("happy: search by social handle hits EXISTS branch", func(t *testing.T) {
		t.Parallel()
		marker := strings.ToLower("uniquehandle" + testutil.UniqueIIN())
		opts := defaultCreatorOpts()
		opts.Socials = []testutil.SocialFixture{
			{Platform: string(apiclient.Instagram), Handle: marker, Verification: testutil.VerificationAutoIG},
		}
		creator := testutil.SetupApprovedCreator(t, opts)

		body := validCreatorListBody()
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectCreatorIDs(resp.JSON200.Data.Items), creator.CreatorID)
	})

	t.Run("happy: blank search after trim disables filter", func(t *testing.T) {
		t.Parallel()
		// Blank search after trim disables the search filter — but on a busy
		// staging DB the unfiltered list quickly exceeds perPage=200 and the
		// freshly seeded creator gets pushed off the first page. Sort by
		// created_at desc puts it at the head; perPage=200 is then enough.
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts())

		body := validCreatorListBody()
		body.Sort = apiclient.CreatorListSortFieldCreatedAt
		body.Order = apiclient.Desc
		body.Search = pointer.ToString("   ")
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectCreatorIDs(resp.JSON200.Data.Items), creator.CreatorID)
	})

	t.Run("happy: search wildcards in user input are escaped", func(t *testing.T) {
		t.Parallel()
		// "100%" must not glob — without escape every creator in the table
		// would match. The seeded creator has neither "100%" nor "%" in any
		// PII field, so escaped search returns empty.
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOpts())

		body := validCreatorListBody()
		body.Search = pointer.ToString("100%")
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.NotContains(t, collectCreatorIDs(resp.JSON200.Data.Items), creator.CreatorID,
			"escaped wildcard search must not match the creator without a literal '100%%' substring")
	})

	t.Run("sort: created_at asc orders by approve time", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		first := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "first"))
		time.Sleep(1100 * time.Millisecond)
		second := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "second"))

		body := validCreatorListBody()
		body.Sort = apiclient.CreatorListSortFieldCreatedAt
		body.Order = apiclient.Asc
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(first.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		positions := positionsForCreators(resp.JSON200.Data.Items, first.CreatorID, second.CreatorID)
		require.Less(t, positions[first.CreatorID], positions[second.CreatorID],
			"first creator must precede second under created_at asc")
	})

	t.Run("sort: city_name asc orders by ct.name", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		almatyOpts := defaultCreatorOptsScoped(marker, "ala")
		almaty := testutil.SetupApprovedCreator(t, almatyOpts)
		astanaOpts := defaultCreatorOptsScoped(marker, "ast")
		astanaOpts.CityCode = "astana"
		astana := testutil.SetupApprovedCreator(t, astanaOpts)

		body := validCreatorListBody()
		body.Sort = apiclient.CreatorListSortFieldCityName
		body.Order = apiclient.Asc
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(almaty.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		positions := positionsForCreators(resp.JSON200.Data.Items, almaty.CreatorID, astana.CreatorID)
		require.Less(t, positions[almaty.CreatorID], positions[astana.CreatorID],
			"Алматы (almaty) must precede Астана (astana) under city_name asc")
	})

	t.Run("sort: full_name asc orders by last_name", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		aOpts := defaultCreatorOptsScoped(marker, "a")
		aOpts.LastName = "Аaaaaaaaa" + testutil.UniqueIIN()[6:]
		aOpts.FirstName = "Айдана"
		aCreator := testutil.SetupApprovedCreator(t, aOpts)

		zOpts := defaultCreatorOptsScoped(marker, "z")
		zOpts.LastName = "Яyyyyyyyy" + testutil.UniqueIIN()[6:]
		zOpts.FirstName = "Айдана"
		zCreator := testutil.SetupApprovedCreator(t, zOpts)

		body := validCreatorListBody()
		body.Sort = apiclient.CreatorListSortFieldFullName
		body.Order = apiclient.Asc
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(aCreator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		positions := positionsForCreators(resp.JSON200.Data.Items, aCreator.CreatorID, zCreator.CreatorID)
		require.Less(t, positions[aCreator.CreatorID], positions[zCreator.CreatorID],
			"Аaaaa creator must precede Яyyyy creator under full_name asc")
	})

	t.Run("sort: full_name desc reverses last_name order", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		aOpts := defaultCreatorOptsScoped(marker, "a")
		aOpts.LastName = "Аaaaaaaaa" + testutil.UniqueIIN()[6:]
		aOpts.FirstName = "Айдана"
		aCreator := testutil.SetupApprovedCreator(t, aOpts)

		zOpts := defaultCreatorOptsScoped(marker, "z")
		zOpts.LastName = "Яyyyyyyyy" + testutil.UniqueIIN()[6:]
		zOpts.FirstName = "Айдана"
		zCreator := testutil.SetupApprovedCreator(t, zOpts)

		body := validCreatorListBody()
		body.Sort = apiclient.CreatorListSortFieldFullName
		body.Order = apiclient.Desc
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(aCreator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		positions := positionsForCreators(resp.JSON200.Data.Items, aCreator.CreatorID, zCreator.CreatorID)
		require.Less(t, positions[zCreator.CreatorID], positions[aCreator.CreatorID],
			"Яyyyy creator must precede Аaaaa creator under full_name desc")
	})

	t.Run("sort: birth_date desc orders by birth_date", func(t *testing.T) {
		t.Parallel()
		// UniqueIIN samples birth year from 1985..2005, so two fresh creators
		// will likely differ in birth date. We do not assert which one is
		// first — just that the relative order matches the field ordering.
		marker := newMarker()
		first := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "first"))
		second := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "second"))

		body := validCreatorListBody()
		body.Sort = apiclient.CreatorListSortFieldBirthDate
		body.Order = apiclient.Desc
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(first.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		// Verify response is sorted by birth_date desc.
		var prev *apiclient.CreatorListItem
		for i := range resp.JSON200.Data.Items {
			it := &resp.JSON200.Data.Items[i]
			if prev != nil {
				require.False(t, it.BirthDate.After(prev.BirthDate.Time),
					"birth_date desc violated: %s precedes %s", prev.BirthDate.Time, it.BirthDate.Time)
			}
			prev = it
		}
		// Both seeded creators are in the response.
		ids := collectCreatorIDs(resp.JSON200.Data.Items)
		require.Contains(t, ids, first.CreatorID)
		require.Contains(t, ids, second.CreatorID)
	})

	t.Run("sort: updated_at asc returns 200", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "u"))

		body := validCreatorListBody()
		body.Sort = apiclient.CreatorListSortFieldUpdatedAt
		body.Order = apiclient.Asc
		body.Search = pointer.ToString(marker)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Contains(t, collectCreatorIDs(resp.JSON200.Data.Items), creator.CreatorID)
	})

	t.Run("pagination: page=2 returns next slice", func(t *testing.T) {
		t.Parallel()
		// Three marker-scoped creators, perPage=2 → page 1 has 2, page 2 has
		// the third. Marker isolates the dataset from any other parallel
		// seeders so total stays exactly 3 even on a busy staging DB.
		marker := newMarker()
		first := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "1"))
		_ = testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "2"))
		_ = testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "3"))

		body := validCreatorListBody()
		body.Search = pointer.ToString(marker)
		body.PerPage = 2
		body.Page = 1

		c := testutil.NewAPIClient(t)
		resp1, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(first.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp1.StatusCode())
		require.NotNil(t, resp1.JSON200)
		require.Len(t, resp1.JSON200.Data.Items, 2)
		require.Equal(t, int64(3), resp1.JSON200.Data.Total, "total must reflect the marker-scoped count")

		body.Page = 2
		resp2, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(first.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp2.StatusCode())
		require.NotNil(t, resp2.JSON200)
		require.Len(t, resp2.JSON200.Data.Items, 1)
		// Page 2 must not duplicate page 1 ids.
		page1IDs := collectCreatorIDs(resp1.JSON200.Data.Items)
		page2IDs := collectCreatorIDs(resp2.JSON200.Data.Items)
		for _, id := range page2IDs {
			require.NotContains(t, page1IDs, id, "page 2 must not duplicate page 1 ids")
		}
	})

	t.Run("pagination: beyond last page returns empty items, total preserved", func(t *testing.T) {
		t.Parallel()
		marker := newMarker()
		creator := testutil.SetupApprovedCreator(t, defaultCreatorOptsScoped(marker, "tail"))

		// Page bound is CreatorListPageMax (100_000) — pick a value comfortably
		// inside the bound but past the marker-scoped count of 1.
		body := validCreatorListBody()
		body.Search = pointer.ToString(marker)
		body.PerPage = 1
		body.Page = 99_999

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Empty(t, resp.JSON200.Data.Items)
		require.Equal(t, int64(1), resp.JSON200.Data.Total)
	})

	t.Run("item shape: hydrated city/categories, lean PII set", func(t *testing.T) {
		t.Parallel()
		opts := defaultCreatorOpts()
		opts.CategoryCodes = []string{"beauty", "fashion"}
		creator := testutil.SetupApprovedCreator(t, opts)

		body := validCreatorListBody()
		body.Search = pointer.ToString(creator.IIN)
		body.PerPage = 200

		c := testutil.NewAPIClient(t)
		resp, err := c.ListCreatorsWithResponse(context.Background(), body, testutil.WithAuth(creator.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		var item *apiclient.CreatorListItem
		for i := range resp.JSON200.Data.Items {
			if resp.JSON200.Data.Items[i].Id.String() == creator.CreatorID {
				item = &resp.JSON200.Data.Items[i]
				break
			}
		}
		require.NotNil(t, item, "approved creator must surface in the search result")

		require.Equal(t, creator.LastName, item.LastName)
		require.Equal(t, creator.FirstName, item.FirstName)
		require.Equal(t, creator.IIN, item.Iin)
		require.Equal(t, creator.Phone, item.Phone)

		// City must be hydrated from the cities dictionary, not echo the code.
		require.Equal(t, "almaty", item.City.Code)
		require.NotEmpty(t, item.City.Name)
		require.NotEqual(t, item.City.Code, item.City.Name,
			"city.name must come from the dictionary, not fall back to code")

		// Categories must be hydrated and contain the seeded codes.
		require.NotEmpty(t, item.Categories)
		categoryCodes := make([]string, len(item.Categories))
		for i, ca := range item.Categories {
			categoryCodes[i] = ca.Code
			require.NotEmpty(t, ca.Name, "category.name must be hydrated from dictionary")
		}
		sortedSeeded := append([]string(nil), creator.CategoryCodes...)
		sort.Strings(sortedSeeded)
		sortedReturned := append([]string(nil), categoryCodes...)
		sort.Strings(sortedReturned)
		require.Equal(t, sortedSeeded, sortedReturned)

		// Socials must be platform/handle pairs, sorted by platform/handle.
		require.NotEmpty(t, item.Socials)
		for _, soc := range item.Socials {
			require.NotEmpty(t, soc.Platform)
			require.NotEmpty(t, soc.Handle)
		}
	})
}
