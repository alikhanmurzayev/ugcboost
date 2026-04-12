package brand

// brand_test.go contains E2E tests for brand CRUD, manager assignment, and brand isolation.

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// --- Brand CRUD ---

func TestCreateBrand_Success(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	name := "Brand-" + testutil.UniqueEmail("create")

	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: name,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	require.Equal(t, name, resp.JSON201.Data.Name)
	require.NotEmpty(t, resp.JSON201.Data.Id)
}

func TestCreateBrand_EmptyName(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)

	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: "",
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
}

func TestCreateBrand_ForbiddenForManager(t *testing.T) {
	t.Parallel()
	c, token, _, _ := testutil.LoginAsBrandManager(t)

	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: "Some Brand",
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode())
}

func TestListBrands_Admin(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	name := "ListBrand-" + testutil.UniqueEmail("list")
	testutil.SeedBrand(t, name)

	resp, err := c.ListBrandsWithResponse(context.Background(), testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)

	found := false
	for _, b := range resp.JSON200.Data.Brands {
		if b.Name == name {
			found = true
			break
		}
	}
	require.True(t, found, "created brand should appear in admin list")
}

func TestListBrands_ManagerSeesOwnOnly(t *testing.T) {
	t.Parallel()
	c, token, email, _ := testutil.LoginAsBrandManager(t)
	ownBrand := "Own-" + testutil.UniqueEmail("own")
	otherBrand := "Other-" + testutil.UniqueEmail("other")

	testutil.SeedBrandWithManager(t, ownBrand, email)
	testutil.SeedBrand(t, otherBrand)

	resp, err := c.ListBrandsWithResponse(context.Background(), testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)

	for _, b := range resp.JSON200.Data.Brands {
		require.NotEqual(t, otherBrand, b.Name, "manager should NOT see other brand")
	}
}

func TestGetBrand_Success(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	name := "GetBrand-" + testutil.UniqueEmail("get")
	brandID := testutil.SeedBrand(t, name)

	resp, err := c.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	require.Equal(t, name, resp.JSON200.Data.Name)
	require.NotNil(t, resp.JSON200.Data.Managers)
}

func TestGetBrand_ForbiddenForUnrelatedManager(t *testing.T) {
	t.Parallel()
	c, token, _, _ := testutil.LoginAsBrandManager(t)
	brandID := testutil.SeedBrand(t, "Unrelated-"+testutil.UniqueEmail("unrelated"))

	resp, err := c.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode())
}

func TestUpdateBrand_Success(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	brandID := testutil.SeedBrand(t, "OldName-"+testutil.UniqueEmail("update"))
	newName := "NewName-" + testutil.UniqueEmail("updated")

	resp, err := c.UpdateBrandWithResponse(context.Background(), brandID, apiclient.UpdateBrandJSONRequestBody{
		Name: newName,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	require.Equal(t, newName, resp.JSON200.Data.Name)
}

func TestDeleteBrand_Success(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	brandID := testutil.SeedBrand(t, "ToDelete-"+testutil.UniqueEmail("delete"))

	resp, err := c.DeleteBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())

	// Verify it's gone
	getResp, err := c.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, getResp.StatusCode())
}

// --- Manager Assignment ---

func TestAssignManager_Success(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	brandID := testutil.SeedBrand(t, "ForManager-"+testutil.UniqueEmail("assign"))
	managerEmail := testutil.UniqueEmail("newmgr")

	resp, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
		Email: managerEmail,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	require.Equal(t, managerEmail, resp.JSON201.Data.Email)
	require.NotEmpty(t, resp.JSON201.Data.TempPassword, "new user should get temp password")
}

func TestAssignManager_ExistingUser(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	brandID := testutil.SeedBrand(t, "ForExisting-"+testutil.UniqueEmail("existmgr"))
	email, _ := testutil.SeedUser(t, "brand_manager")

	resp, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
		Email: email,
	}, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	require.Empty(t, resp.JSON201.Data.TempPassword, "existing user should NOT get temp password")
}

func TestRemoveManager_Success(t *testing.T) {
	t.Parallel()
	c, token := testutil.LoginAsAdmin(t)
	email, _ := testutil.SeedUser(t, "brand_manager")
	brandID := testutil.SeedBrandWithManager(t, "RemMgr-"+testutil.UniqueEmail("remmgr"), email)

	// Get the user ID
	managerC := testutil.NewAPIClient(t)
	managerToken := testutil.LoginAs(t, managerC, email, "testpass123")
	meResp, err := managerC.GetMeWithResponse(context.Background(), testutil.WithAuth(managerToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, meResp.StatusCode())
	userID := meResp.JSON200.Data.Id

	resp, err := c.RemoveManagerWithResponse(context.Background(), brandID, userID, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
}

// --- Brand Isolation ---

func TestBrandIsolation_ManagerCanViewOwnBrand(t *testing.T) {
	t.Parallel()
	_, _, email, password := testutil.LoginAsBrandManager(t)
	brandID := testutil.SeedBrandWithManager(t, "OwnBrand-"+testutil.UniqueEmail("isolation"), email)

	c := testutil.NewAPIClient(t)
	token := testutil.LoginAs(t, c, email, password)

	resp, err := c.GetBrandWithResponse(context.Background(), brandID, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
}

func TestBrandIsolation_ManagerCannotViewOtherBrand(t *testing.T) {
	t.Parallel()
	c, token, _, _ := testutil.LoginAsBrandManager(t)
	otherBrandID := testutil.SeedBrand(t, "OtherBrand-"+testutil.UniqueEmail("iso2"))

	resp, err := c.GetBrandWithResponse(context.Background(), otherBrandID, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode())
}
