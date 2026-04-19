// Package brand covers E2E tests for /brands:
//
//   - TestBrandCRUD — the full lifecycle for an admin caller:
//       • create: 422 on empty name, 403 when a brand_manager attempts it,
//         201 on success with the brand surfaced in GET /brands afterwards;
//       • get:    404 on an unknown ID, 200 on success with the managers
//         array populated by a follow-up assign;
//       • update: 200 renaming a brand, with the response reflecting the
//         new name;
//       • delete: 200 on success followed by 404 on a subsequent GET.
//   - TestBrandManagerAssignment — POST / DELETE /brands/{id}/managers:
//       • assigning a fresh email provisions a brand_manager and returns a
//         temp password;
//       • assigning an already-seeded brand_manager succeeds without issuing
//         a new temp password;
//       • a brand_manager is forbidden to call assign (403);
//       • remove 200 and the brand's managers array no longer lists the user.
//   - TestBrandIsolation — role-based visibility:
//       • manager listing returns only brands they manage;
//       • admin listing sees every brand created in the test;
//       • manager GET on own brand returns 200;
//       • manager GET on an unrelated brand returns 403.
//
// Data setup is composable through testutil.Setup* helpers; every seeded
// user and brand is cleaned up automatically via POST /test/cleanup-entity
// (users) or DELETE /brands/{id} (brands) after the test when
// E2E_CLEANUP=true (the default).
package brand

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// zeroUUID is a syntactically valid UUID the backend accepts as a path param
// but will never match a real brand — used for 404 assertions.
const zeroUUID = "00000000-0000-0000-0000-000000000000"

func TestBrandCRUD(t *testing.T) {
	t.Parallel()

	t.Run("create with empty name returns 422", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)

		resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
			Name: "",
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.NotEmpty(t, resp.JSON422.Error.Code)
		require.NotEmpty(t, resp.JSON422.Error.Message)
	})

	t.Run("create forbidden for brand_manager", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "HostBrand-"+testutil.UniqueEmail("host"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		resp, err := mgrClient.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
			Name: "Manager-Tries-" + testutil.UniqueEmail("mgrtries"),
		}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("create success appears in admin list", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		name := "Fresh-" + testutil.UniqueEmail("fresh")

		createResp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
			Name: name,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, createResp.StatusCode())
		require.NotNil(t, createResp.JSON201)
		require.NotEmpty(t, createResp.JSON201.Data.Id)
		require.Equal(t, name, createResp.JSON201.Data.Name)

		// Register cleanup manually — we bypassed SetupBrand to assert the
		// raw create response, but we still want the brand removed after.
		testutil.RegisterBrandCleanup(t, c, token, createResp.JSON201.Data.Id)

		listResp, err := c.ListBrandsWithResponse(context.Background(), testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, listResp.StatusCode())
		require.NotNil(t, listResp.JSON200)
		require.True(t, containsBrandNamed(listResp.JSON200.Data.Brands, name),
			"created brand must surface in admin list")
	})

	t.Run("get with unknown id returns 404", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)

		resp, err := c.GetBrandWithResponse(context.Background(), zeroUUID, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("get success returns full detail with managers", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		name := "WithMgr-" + testutil.UniqueEmail("withmgr")
		brandID := testutil.SetupBrand(t, c, token, name)
		mgrEmail, _ := testutil.SetupManager(t, c, token, brandID)

		resp, err := c.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		got := resp.JSON200.Data
		require.Equal(t, brandID, got.Id)
		require.Equal(t, name, got.Name)
		require.Nil(t, got.LogoUrl)
		require.Len(t, got.Managers, 1)
		require.Equal(t, mgrEmail, got.Managers[0].Email)
		require.NotEmpty(t, got.Managers[0].UserId)
	})

	t.Run("update renames the brand", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, c, token, "OldName-"+testutil.UniqueEmail("rename"))
		newName := "NewName-" + testutil.UniqueEmail("renamed")

		resp, err := c.UpdateBrandWithResponse(context.Background(), brandID, apiclient.UpdateBrandJSONRequestBody{
			Name: newName,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Equal(t, brandID, resp.JSON200.Data.Id)
		require.Equal(t, newName, resp.JSON200.Data.Name)
	})

	t.Run("delete makes subsequent get return 404", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		// Bypass SetupBrand to avoid the auto-cleanup double-deleting after
		// we delete the brand ourselves — we still want to cover the deletion
		// path end-to-end, including the follow-up 404.
		createResp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
			Name: "ToDelete-" + testutil.UniqueEmail("del"),
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, createResp.StatusCode())
		require.NotNil(t, createResp.JSON201)
		brandID := createResp.JSON201.Data.Id

		delResp, err := c.DeleteBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, delResp.StatusCode())

		getResp, err := c.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, getResp.StatusCode())
		require.NotNil(t, getResp.JSON404)
		require.Equal(t, "NOT_FOUND", getResp.JSON404.Error.Code)
	})
}

func TestBrandManagerAssignment(t *testing.T) {
	t.Parallel()

	t.Run("assign fresh email creates user with temp password", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, c, token, "NewMgrBrand-"+testutil.UniqueEmail("newmgrb"))
		newEmail := testutil.UniqueEmail("newmgr")

		resp, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
			Email: newEmail,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode())
		require.NotNil(t, resp.JSON201)

		data := resp.JSON201.Data
		require.Equal(t, newEmail, data.Email)
		require.Equal(t, string(apiclient.BrandManager), data.Role)
		require.NotEmpty(t, data.UserId)
		require.NotNil(t, data.TempPassword)
		require.NotEmpty(t, *data.TempPassword)

		// The created user is not routed through Setup*; ensure we still
		// clean them up so they don't leak across test runs.
		testutil.RegisterUserCleanup(t, data.UserId)
	})

	t.Run("assign existing user returns no temp password", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, c, token, "ExistingMgr-"+testutil.UniqueEmail("existmgrb"))
		existingEmail, _ := testutil.SeedUser(t, "brand_manager")

		resp, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
			Email: existingEmail,
		}, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode())
		require.NotNil(t, resp.JSON201)

		data := resp.JSON201.Data
		require.Equal(t, existingEmail, data.Email)
		require.Nil(t, data.TempPassword, "existing user must not be issued a new password")

		testutil.RegisterUserCleanup(t, data.UserId)
	})

	t.Run("assign forbidden for brand_manager", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken, "MgrBrand-"+testutil.UniqueEmail("mgrforbidden"))
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		resp, err := mgrClient.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
			Email: testutil.UniqueEmail("mgr-target"),
		}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("remove unassigns the user from the brand", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, c, token, "RemMgr-"+testutil.UniqueEmail("remmgrb"))
		mgrEmail, _ := testutil.SetupManager(t, c, token, brandID)

		// Resolve the user ID via GET /brands/{id} — the managers array lists it.
		getResp, err := c.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, getResp.StatusCode())
		require.Len(t, getResp.JSON200.Data.Managers, 1)
		userID := getResp.JSON200.Data.Managers[0].UserId
		require.Equal(t, mgrEmail, getResp.JSON200.Data.Managers[0].Email)

		delResp, err := c.RemoveManagerWithResponse(context.Background(), brandID, userID, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, delResp.StatusCode())

		// Managers array should be empty now.
		afterResp, err := c.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, afterResp.StatusCode())
		require.Empty(t, afterResp.JSON200.Data.Managers)
	})
}

func TestBrandIsolation(t *testing.T) {
	t.Parallel()

	t.Run("manager lists only brands they manage", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		ownName := "Own-" + testutil.UniqueEmail("ownlist")
		otherName := "Other-" + testutil.UniqueEmail("otherlist")

		ownBrandID := testutil.SetupBrand(t, adminClient, adminToken, ownName)
		testutil.SetupBrand(t, adminClient, adminToken, otherName)

		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, ownBrandID)

		resp, err := mgrClient.ListBrandsWithResponse(context.Background(), testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		require.True(t, containsBrandNamed(resp.JSON200.Data.Brands, ownName),
			"manager must see their own brand")
		require.False(t, containsBrandNamed(resp.JSON200.Data.Brands, otherName),
			"manager must NOT see an unrelated brand")
	})

	t.Run("admin list includes every brand created in this test", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)
		nameA := "AdminA-" + testutil.UniqueEmail("adminA")
		nameB := "AdminB-" + testutil.UniqueEmail("adminB")

		testutil.SetupBrand(t, c, token, nameA)
		testutil.SetupBrand(t, c, token, nameB)

		resp, err := c.ListBrandsWithResponse(context.Background(), testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.True(t, containsBrandNamed(resp.JSON200.Data.Brands, nameA))
		require.True(t, containsBrandNamed(resp.JSON200.Data.Brands, nameB))
	})

	t.Run("manager get on own brand returns 200", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		name := "OwnGet-" + testutil.UniqueEmail("ownget")
		brandID := testutil.SetupBrand(t, adminClient, adminToken, name)
		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		resp, err := mgrClient.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Equal(t, name, resp.JSON200.Data.Name)
	})

	t.Run("manager get on unrelated brand returns 403", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		ownBrandID := testutil.SetupBrand(t, adminClient, adminToken, "OwnHome-"+testutil.UniqueEmail("ownhome"))
		otherBrandID := testutil.SetupBrand(t, adminClient, adminToken, "Stranger-"+testutil.UniqueEmail("stranger"))

		mgrClient, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, ownBrandID)

		resp, err := mgrClient.GetBrandWithResponse(context.Background(), otherBrandID, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})
}

// containsBrandNamed scans a list of BrandListItem for the given name.
func containsBrandNamed(list []apiclient.BrandListItem, name string) bool {
	for _, b := range list {
		if b.Name == name {
			return true
		}
	}
	return false
}
