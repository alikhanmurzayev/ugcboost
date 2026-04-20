// Package auth — E2E тесты HTTP-поверхности /auth/*.
//
// TestHealthCheck — санити-проверка перед любыми auth-сценариями: бэкенд
// доступен и отдаёт ожидаемый health-payload. Без этой страховки сетевые
// ошибки маскировались бы под auth-баги в соседних тестах.
//
// TestLogin проверяет POST /auth/login во всех задокументированных ответах.
// Пустой email отправляется сырым HTTP — openapi_types.Email в
// сгенерированном клиенте не пропускает пустую строку, так что достать
// серверную валидацию можно только в обход клиента; ответ 422. Несуществующий
// аккаунт и неправильный пароль отвечают 401 одинаково — сервер не должен
// разглашать факт существования пользователя. Короткий пароль возвращает
// тот же 401, чтобы не выдать формат подсказкой. Email нормализуется (trim
// + lowercase), а успешный логин отдаёт access token, payload пользователя
// и HttpOnly-cookie с refresh_token.
//
// TestRefresh покрывает POST /auth/refresh. Без cookie сервер отвечает 401.
// В успешном сценарии мы проходим rotation chain — cookie jar после каждого
// вызова заменяет refresh-cookie на новую ротированную, и следующий refresh
// работает только потому, что потребляет свежий токен. Значения access token
// между двумя близкими ротациями не сравниваем: iat/exp могут совпасть
// посекундно, а гарантия ротации — именно в том, что каждый refresh принимает
// свежий cookie.
//
// TestGetMe — GET /auth/me. Отсутствующий Authorization возвращает 401,
// мусорный JWT тоже 401 (одинаковый код — никакой утечки о типе проблемы),
// а авторизованный админ получает полный user-payload с корректными email
// и ролью.
//
// TestLogout — POST /auth/logout. Неавторизованный вызов — 401, успешный —
// 200. После успешного логаута последующий /auth/refresh уже отвергается:
// refresh_token действительно инвалидирован серверной стороной, а не просто
// очищен cookie у клиента.
//
// TestPasswordReset проходит полный lifecycle сброса пароля. Запрос на сброс
// для существующего и для несуществующего email одинаково отвечает 200 —
// это защита от enumeration. Пустой email отправляется сырым HTTP и даёт
// 422 от серверной валидации. В happy path новый пароль работает, старый
// блокируется. Невалидный токен — 401, повторное использование валидного
// токена — тоже 401 (single-use). Короткий пароль отсекается валидацией с
// 422. И ключевой инвариант: успешный reset инвалидирует все outstanding
// refresh tokens — сессии, заведённые до сброса, умирают.
//
// TestFullAuthFlow гоняет конец-в-конец два сценария интеграции: сначала
// login → refresh → me → logout, затем путь сброса login → request → reset
// → login. Здесь важны не edge cases отдельных ручек, а согласованность
// цепочки и то, что состояние пользователя проходит через неё без разрывов.
//
// Каждый тест сеет своих пользователей через testutil.SetupAdmin /
// SetupAdminClient — хелперы автоматически регистрируют cleanup через
// POST /test/cleanup-entity, и после теста данные удаляются при
// E2E_CLEANUP=true (это дефолт).
package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

const (
	validPassword   = testutil.DefaultPassword
	replacementPass = "newpassword123"
	redacted        = "<redacted>"
)

func TestHealthCheck(t *testing.T) {
	t.Parallel()
	c := testutil.NewAPIClient(t)

	resp, err := c.HealthCheckWithResponse(context.Background())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	require.Equal(t, "ok", resp.JSON200.Status)
	require.NotEmpty(t, resp.JSON200.Version)
}

func TestLogin(t *testing.T) {
	t.Parallel()

	t.Run("empty email returns 422", func(t *testing.T) {
		t.Parallel()
		// Raw HTTP: the generated client validates openapi_types.Email format
		// before serialization and refuses to send an empty string at all, so
		// the backend never gets a chance to respond. PostRaw sidesteps that
		// client-side guard so we can exercise the server's own validation.
		resp := testutil.PostRaw(t, "/auth/login", map[string]string{
			"email":    "",
			"password": validPassword,
		})
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	})

	t.Run("non-existent email returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)

		resp, err := c.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
			Email:    openapi_types.Email("nobody@example.com"),
			Password: validPassword,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
		require.NotEmpty(t, resp.JSON401.Error.Message)
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		t.Parallel()
		_, _, email := testutil.SetupAdminClient(t)
		// Setup seeds a fresh admin and registers cleanup; we log in again
		// below with the wrong password to confirm the server still rejects.

		c := testutil.NewAPIClient(t)
		resp, err := c.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
			Email:    openapi_types.Email(email),
			Password: "wrongpassword",
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
		require.NotEmpty(t, resp.JSON401.Error.Message)
	})

	t.Run("short password returns 401 without leaking format hint", func(t *testing.T) {
		t.Parallel()
		_, _, email := testutil.SetupAdminClient(t)

		c := testutil.NewAPIClient(t)
		resp, err := c.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
			Email:    openapi_types.Email(email),
			Password: "12345",
		})
		require.NoError(t, err)
		// Short password must not reveal account existence via a different
		// status code — bcrypt compare simply fails and we return 401.
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
	})

	t.Run("email normalization (trim + lowercase)", func(t *testing.T) {
		t.Parallel()
		_, _, email := testutil.SetupAdminClient(t)

		c := testutil.NewAPIClient(t)
		resp, err := c.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
			Email:    openapi_types.Email("  " + email + "  "),
			Password: validPassword,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
		require.Equal(t, email, string(resp.JSON200.Data.User.Email))
	})

	t.Run("success returns access token, user payload and HttpOnly refresh cookie", func(t *testing.T) {
		t.Parallel()
		_, _, email := testutil.SetupAdminClient(t)

		c := testutil.NewAPIClient(t)
		resp, err := c.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
			Email:    openapi_types.Email(email),
			Password: validPassword,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		data := resp.JSON200.Data
		requireUUID(t, data.User.Id)
		require.NotEmpty(t, data.AccessToken)
		data.User.Id = redacted
		data.AccessToken = redacted

		require.Equal(t, apiclient.LoginData{
			AccessToken: redacted,
			User: apiclient.User{
				Id:    redacted,
				Email: openapi_types.Email(email),
				Role:  apiclient.Admin,
			},
		}, data)

		refreshCookie := findCookie(resp.HTTPResponse.Cookies(), testutil.RefreshCookieName)
		require.NotNil(t, refreshCookie, "refresh_token cookie must be set")
		require.True(t, refreshCookie.HttpOnly, "refresh_token cookie must be HttpOnly")
		require.NotEmpty(t, refreshCookie.Value)
	})
}

func TestRefresh(t *testing.T) {
	t.Parallel()

	t.Run("no cookie returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)

		resp, err := c.RefreshTokenWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
	})

	t.Run("rotation chain keeps working through multiple refreshes", func(t *testing.T) {
		t.Parallel()
		c, _, _ := testutil.SetupAdminClient(t)

		// First rotation: the cookie jar replaces the initial refresh cookie
		// with the rotated one server-side, then we call refresh again to
		// verify the new cookie is honored.
		resp1, err := c.RefreshTokenWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp1.StatusCode())
		require.NotNil(t, resp1.JSON200)
		require.NotEmpty(t, resp1.JSON200.Data.AccessToken)

		resp2, err := c.RefreshTokenWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp2.StatusCode())
		require.NotNil(t, resp2.JSON200)
		require.NotEmpty(t, resp2.JSON200.Data.AccessToken)
		// Access tokens share the same iat/exp second when two refreshes run
		// back-to-back, so we don't compare their string values — what rotation
		// guarantees is that each refresh succeeds because it consumes a fresh
		// refresh_token cookie.
	})
}

func TestGetMe(t *testing.T) {
	t.Parallel()

	t.Run("no token returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		resp, err := c.GetMeWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		resp, err := c.GetMeWithResponse(context.Background(), testutil.WithAuth("not-a-jwt"))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
	})

	t.Run("success returns full user payload", func(t *testing.T) {
		t.Parallel()
		c, token, email := testutil.SetupAdminClient(t)

		resp, err := c.GetMeWithResponse(context.Background(), testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)

		got := resp.JSON200.Data
		requireUUID(t, got.Id)
		got.Id = redacted
		require.Equal(t, apiclient.User{
			Id:    redacted,
			Email: openapi_types.Email(email),
			Role:  apiclient.Admin,
		}, got)
	})
}

func TestLogout(t *testing.T) {
	t.Parallel()

	t.Run("no auth returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		resp, err := c.LogoutWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		// Logout only models JSONDefault for error responses.
		require.NotNil(t, resp.JSONDefault)
		require.Equal(t, "UNAUTHORIZED", resp.JSONDefault.Error.Code)
	})

	t.Run("success returns 200", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)

		resp, err := c.LogoutWithResponse(context.Background(), testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
		require.NotNil(t, resp.JSON200)
	})

	t.Run("refresh fails after logout", func(t *testing.T) {
		t.Parallel()
		c, token, _ := testutil.SetupAdminClient(t)

		logoutResp, err := c.LogoutWithResponse(context.Background(), testutil.WithAuth(token))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, logoutResp.StatusCode())

		refreshResp, err := c.RefreshTokenWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, refreshResp.StatusCode())
		require.NotNil(t, refreshResp.JSON401)
		require.Equal(t, "UNAUTHORIZED", refreshResp.JSON401.Error.Code)
	})
}

func TestPasswordReset(t *testing.T) {
	t.Parallel()

	t.Run("request for existing account returns 200", func(t *testing.T) {
		t.Parallel()
		c, _, email := testutil.SetupAdminClient(t)

		resp, err := c.RequestPasswordResetWithResponse(context.Background(), apiclient.RequestPasswordResetJSONRequestBody{
			Email: openapi_types.Email(email),
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
	})

	t.Run("request for non-existent account returns 200 (no enumeration)", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)

		resp, err := c.RequestPasswordResetWithResponse(context.Background(), apiclient.RequestPasswordResetJSONRequestBody{
			Email: openapi_types.Email("nobody@example.com"),
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode())
	})

	t.Run("request with empty email returns 422", func(t *testing.T) {
		t.Parallel()
		// Raw: openapi_types.Email blocks empty strings in the generated
		// client; send JSON manually to hit the backend's own validation.
		resp := testutil.PostRaw(t, "/auth/password-reset-request", map[string]string{"email": ""})
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	})

	t.Run("reset success swaps password and blocks the old one", func(t *testing.T) {
		t.Parallel()
		c, _, email := testutil.SetupAdminClient(t)

		requestReset(t, c, email)
		rawToken := testutil.GetResetToken(t, email)
		require.NotEmpty(t, rawToken)

		resetResp, err := c.ResetPasswordWithResponse(context.Background(), apiclient.ResetPasswordJSONRequestBody{
			Token: rawToken, NewPassword: replacementPass,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resetResp.StatusCode())

		// New password works.
		fresh := testutil.NewAPIClient(t)
		loginNew, err := fresh.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
			Email: openapi_types.Email(email), Password: replacementPass,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, loginNew.StatusCode())

		// Old password rejected.
		loginOld, err := fresh.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
			Email: openapi_types.Email(email), Password: validPassword,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, loginOld.StatusCode())
		require.NotNil(t, loginOld.JSON401)
		require.Equal(t, "UNAUTHORIZED", loginOld.JSON401.Error.Code)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		resp, err := c.ResetPasswordWithResponse(context.Background(), apiclient.ResetPasswordJSONRequestBody{
			Token: "invalid-token", NewPassword: replacementPass,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
		require.NotNil(t, resp.JSON401)
		require.Equal(t, "UNAUTHORIZED", resp.JSON401.Error.Code)
	})

	t.Run("reused token returns 401 (single-use)", func(t *testing.T) {
		t.Parallel()
		c, _, email := testutil.SetupAdminClient(t)

		requestReset(t, c, email)
		rawToken := testutil.GetResetToken(t, email)

		first, err := c.ResetPasswordWithResponse(context.Background(), apiclient.ResetPasswordJSONRequestBody{
			Token: rawToken, NewPassword: replacementPass,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, first.StatusCode())

		second, err := c.ResetPasswordWithResponse(context.Background(), apiclient.ResetPasswordJSONRequestBody{
			Token: rawToken, NewPassword: "anotherpass1",
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, second.StatusCode())
		require.NotNil(t, second.JSON401)
		require.Equal(t, "UNAUTHORIZED", second.JSON401.Error.Code)
	})

	t.Run("short password returns 422", func(t *testing.T) {
		t.Parallel()
		c, _, email := testutil.SetupAdminClient(t)

		requestReset(t, c, email)
		rawToken := testutil.GetResetToken(t, email)

		resp, err := c.ResetPasswordWithResponse(context.Background(), apiclient.ResetPasswordJSONRequestBody{
			Token: rawToken, NewPassword: "short",
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.NotEmpty(t, resp.JSON422.Error.Code)
		require.NotEmpty(t, resp.JSON422.Error.Message)
	})

	t.Run("reset invalidates outstanding refresh tokens", func(t *testing.T) {
		t.Parallel()
		c, _, email := testutil.SetupAdminClient(t)

		requestReset(t, c, email)
		rawToken := testutil.GetResetToken(t, email)

		_, err := c.ResetPasswordWithResponse(context.Background(), apiclient.ResetPasswordJSONRequestBody{
			Token: rawToken, NewPassword: replacementPass,
		})
		require.NoError(t, err)

		refreshResp, err := c.RefreshTokenWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, refreshResp.StatusCode())
	})
}

func TestFullAuthFlow(t *testing.T) {
	t.Parallel()

	t.Run("login → refresh → me → logout", func(t *testing.T) {
		t.Parallel()
		c, _, email := testutil.SetupAdminClient(t)

		refreshResp, err := c.RefreshTokenWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, refreshResp.StatusCode())
		require.NotEmpty(t, refreshResp.JSON200.Data.AccessToken)
		newToken := refreshResp.JSON200.Data.AccessToken

		meResp, err := c.GetMeWithResponse(context.Background(), testutil.WithAuth(newToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, meResp.StatusCode())
		require.Equal(t, email, string(meResp.JSON200.Data.Email))

		logoutResp, err := c.LogoutWithResponse(context.Background(), testutil.WithAuth(newToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, logoutResp.StatusCode())

		afterLogout, err := c.RefreshTokenWithResponse(context.Background())
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, afterLogout.StatusCode())
	})

	t.Run("password reset full cycle", func(t *testing.T) {
		t.Parallel()
		c, _, email := testutil.SetupAdminClient(t)

		requestReset(t, c, email)
		rawToken := testutil.GetResetToken(t, email)

		resetResp, err := c.ResetPasswordWithResponse(context.Background(), apiclient.ResetPasswordJSONRequestBody{
			Token: rawToken, NewPassword: replacementPass,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resetResp.StatusCode())

		fresh := testutil.NewAPIClient(t)
		loginResp, err := fresh.LoginWithResponse(context.Background(), apiclient.LoginJSONRequestBody{
			Email: openapi_types.Email(email), Password: replacementPass,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, loginResp.StatusCode())
		require.Equal(t, email, string(loginResp.JSON200.Data.User.Email))
	})
}

// requestReset is a thin wrapper to keep the reset-cycle tests readable.
func requestReset(t *testing.T, c *apiclient.ClientWithResponses, email string) {
	t.Helper()
	resp, err := c.RequestPasswordResetWithResponse(context.Background(), apiclient.RequestPasswordResetJSONRequestBody{
		Email: openapi_types.Email(email),
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
}

// requireUUID fails the test if the given string is not a valid UUID. Used
// to validate server-generated IDs before redacting them for equality
// assertions on the surrounding structure.
func requireUUID(t *testing.T, s string) {
	t.Helper()
	_, err := uuid.Parse(s)
	require.NoError(t, err, "expected a UUID, got %q", s)
}

// findCookie returns the first cookie with the given name, or nil.
func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}
