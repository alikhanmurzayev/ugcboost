// Package dictionary — E2E тесты публичной ручки GET /dictionaries/{type}.
//
// Лендинг тянет categories и cities при загрузке формы; этот тест-файл
// проверяет, что:
//   - категории отдают свежий сид (есть home_diy/animals/other, нет gaming),
//     отсортированный по sort_order/code;
//   - cities содержит 17 городов с лендинга, метро (Алматы/Астана/Шымкент)
//     стоят первыми;
//   - неизвестный тип отдаёт 404, а не 500.
package dictionary_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

func TestListDictionaryCategories(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	resp, err := c.ListDictionaryWithResponse(context.Background(), apiclient.Categories)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)

	codes := make([]string, 0, len(resp.JSON200.Data.Items))
	prevSort := -1
	for _, item := range resp.JSON200.Data.Items {
		require.GreaterOrEqual(t, item.SortOrder, prevSort, "sort_order must be non-decreasing")
		prevSort = item.SortOrder
		codes = append(codes, item.Code)
	}
	require.Contains(t, codes, "home_diy", "expected new category 'home_diy'")
	require.Contains(t, codes, "animals", "expected new category 'animals'")
	require.Contains(t, codes, "other", "expected new category 'other'")
	require.NotContains(t, codes, "gaming", "category 'gaming' should be removed")
}

func TestListDictionaryCities(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	resp, err := c.ListDictionaryWithResponse(context.Background(), apiclient.Cities)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)

	items := resp.JSON200.Data.Items
	require.Len(t, items, 17, "expected 17 cities seeded from landing content.ts")
	require.Equal(t, "almaty", items[0].Code, "Алматы should be first by sort_order")
	require.Equal(t, "astana", items[1].Code, "Астана should be second by sort_order")
	require.Equal(t, "shymkent", items[2].Code, "Шымкент should be third by sort_order")
}

// TestListDictionaryUnknownType locks the security/UX invariant that an
// unknown dictionary type degrades to 404 NOT_FOUND, never 500. oapi-codegen
// leaves the path-enum untyped at the wrapper level, so the value reaches the
// handler/service and is mapped onto domain.ErrNotFound through
// domain.ErrDictionaryUnknownType — the public response stays a clean 404 so
// scanners cannot fingerprint missing dictionaries via 5xx leaks.
func TestListDictionaryUnknownType(t *testing.T) {
	t.Parallel()

	c := testutil.NewAPIClient(t)
	resp, err := c.ListDictionaryWithResponse(
		context.Background(),
		apiclient.ListDictionaryParamsType("unicorns"),
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode())
	require.NotNil(t, resp.JSON404)
	require.Equal(t, "NOT_FOUND", resp.JSON404.Error.Code)
}
