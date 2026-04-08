package e2etest

import (
	"context"
	"net/http"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/e2etest/apiclient"
	"github.com/alikhanmurzayev/ugcboost/e2etest/testclient"
)

// --- Helpers ---

func seedBrand(t *testing.T, name string) string {
	t.Helper()
	tc := newTestClient(t)
	resp, err := tc.SeedBrandWithResponse(context.Background(), testclient.SeedBrandJSONRequestBody{
		Name: name,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	return resp.JSON201.Data.Id
}

func seedBrandWithManager(t *testing.T, name, managerEmail string) string {
	t.Helper()
	tc := newTestClient(t)
	resp, err := tc.SeedBrandWithResponse(context.Background(), testclient.SeedBrandJSONRequestBody{
		Name:         name,
		ManagerEmail: (*openapi_types.Email)(&managerEmail),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	return resp.JSON201.Data.Id
}

func loginAsAdmin(t *testing.T) (*apiclient.ClientWithResponses, string) {
	t.Helper()
	email, password := seedUser(t, "admin")
	c := newAPIClient(t)
	token := loginAs(t, c, email, password)
	return c, token
}

func loginAsBrandManager(t *testing.T) (*apiclient.ClientWithResponses, string, string, string) {
	t.Helper()
	email, password := seedUser(t, "brand_manager")
	c := newAPIClient(t)
	token := loginAs(t, c, email, password)
	return c, token, email, password
}

// --- Brand CRUD ---

func TestCreateBrand_Success(t *testing.T) {
	c, token := loginAsAdmin(t)
	name := "Brand-" + uniqueEmail("create")

	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: name,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	assert.Equal(t, name, resp.JSON201.Data.Name)
	assert.NotEmpty(t, resp.JSON201.Data.Id)
}

func TestCreateBrand_EmptyName(t *testing.T) {
	c, token := loginAsAdmin(t)

	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: "",
	}, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
}

func TestCreateBrand_ForbiddenForManager(t *testing.T) {
	c, token, _, _ := loginAsBrandManager(t)

	resp, err := c.CreateBrandWithResponse(context.Background(), apiclient.CreateBrandJSONRequestBody{
		Name: "Some Brand",
	}, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode())
}

func TestListBrands_Admin(t *testing.T) {
	c, token := loginAsAdmin(t)
	name := "ListBrand-" + uniqueEmail("list")
	seedBrand(t, name)

	resp, err := c.ListBrandsWithResponse(context.Background(), withAuth(token))
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
	assert.True(t, found, "created brand should appear in admin list")
}

func TestListBrands_ManagerSeesOwnOnly(t *testing.T) {
	c, token, email, _ := loginAsBrandManager(t)
	ownBrand := "Own-" + uniqueEmail("own")
	otherBrand := "Other-" + uniqueEmail("other")

	seedBrandWithManager(t, ownBrand, email)
	seedBrand(t, otherBrand)

	resp, err := c.ListBrandsWithResponse(context.Background(), withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)

	for _, b := range resp.JSON200.Data.Brands {
		assert.NotEqual(t, otherBrand, b.Name, "manager should NOT see other brand")
	}
}

func TestGetBrand_Success(t *testing.T) {
	c, token := loginAsAdmin(t)
	name := "GetBrand-" + uniqueEmail("get")
	brandID := seedBrand(t, name)

	resp, err := c.GetBrandWithResponse(context.Background(), brandID, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	assert.Equal(t, name, resp.JSON200.Data.Name)
	assert.NotNil(t, resp.JSON200.Data.Managers)
}

func TestGetBrand_ForbiddenForUnrelatedManager(t *testing.T) {
	c, token, _, _ := loginAsBrandManager(t)
	brandID := seedBrand(t, "Unrelated-"+uniqueEmail("unrelated"))

	resp, err := c.GetBrandWithResponse(context.Background(), brandID, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode())
}

func TestUpdateBrand_Success(t *testing.T) {
	c, token := loginAsAdmin(t)
	brandID := seedBrand(t, "OldName-"+uniqueEmail("update"))
	newName := "NewName-" + uniqueEmail("updated")

	resp, err := c.UpdateBrandWithResponse(context.Background(), brandID, apiclient.UpdateBrandJSONRequestBody{
		Name: newName,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	assert.Equal(t, newName, resp.JSON200.Data.Name)
}

func TestDeleteBrand_Success(t *testing.T) {
	c, token := loginAsAdmin(t)
	brandID := seedBrand(t, "ToDelete-"+uniqueEmail("delete"))

	resp, err := c.DeleteBrandWithResponse(context.Background(), brandID, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())

	// Verify it's gone
	getResp, err := c.GetBrandWithResponse(context.Background(), brandID, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, getResp.StatusCode())
}

// --- Manager Assignment ---

func TestAssignManager_Success(t *testing.T) {
	c, token := loginAsAdmin(t)
	brandID := seedBrand(t, "ForManager-"+uniqueEmail("assign"))
	managerEmail := uniqueEmail("newmgr")

	resp, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
		Email: managerEmail,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	assert.Equal(t, managerEmail, resp.JSON201.Data.Email)
	assert.NotEmpty(t, resp.JSON201.Data.TempPassword, "new user should get temp password")
}

func TestAssignManager_ExistingUser(t *testing.T) {
	c, token := loginAsAdmin(t)
	brandID := seedBrand(t, "ForExisting-"+uniqueEmail("existmgr"))
	email, _ := seedUser(t, "brand_manager")

	resp, err := c.AssignManagerWithResponse(context.Background(), brandID, apiclient.AssignManagerJSONRequestBody{
		Email: email,
	}, withAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode())
	require.NotNil(t, resp.JSON201)
	assert.Empty(t, resp.JSON201.Data.TempPassword, "existing user should NOT get temp password")
}

func TestRemoveManager_Success(t *testing.T) {
	c, token := loginAsAdmin(t)
	email, _ := seedUser(t, "brand_manager")
	brandID := seedBrandWithManager(t, "RemMgr-"+uniqueEmail("remmgr"), email)

	// Get the user ID
	managerC := newAPIClient(t)
	managerToken := loginAs(t, managerC, email, "testpass123")
	meResp, err := managerC.GetMeWithResponse(context.Background(), withAuth(managerToken))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, meResp.StatusCode())
	userID := meResp.JSON200.Data.Id

	resp, err := c.RemoveManagerWithResponse(context.Background(), brandID, userID, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
}

// --- Brand Isolation ---

func TestBrandIsolation_ManagerCanViewOwnBrand(t *testing.T) {
	_, _, email, password := loginAsBrandManager(t)
	brandID := seedBrandWithManager(t, "OwnBrand-"+uniqueEmail("isolation"), email)

	c := newAPIClient(t)
	token := loginAs(t, c, email, password)

	resp, err := c.GetBrandWithResponse(context.Background(), brandID, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
}

func TestBrandIsolation_ManagerCannotViewOtherBrand(t *testing.T) {
	c, token, _, _ := loginAsBrandManager(t)
	otherBrandID := seedBrand(t, "OtherBrand-"+uniqueEmail("iso2"))

	resp, err := c.GetBrandWithResponse(context.Background(), otherBrandID, withAuth(token))
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode())
}
