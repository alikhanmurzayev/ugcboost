import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { login } from "@/api/auth";
import { ROUTES } from "@/shared/constants/routes";
import { useAuthStore } from "@/stores/auth";
import { ApiError } from "@/api/client";
import { getErrorMessage } from "@/shared/i18n/errors";

export default function LoginPage() {
  const { t } = useTranslation(["auth", "common"]);
  const navigate = useNavigate();
  const setAuth = useAuthStore((s) => s.setAuth);

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const res = await login(email, password);
      setAuth(res.data.user, res.data.accessToken);
      navigate(ROUTES.DASHBOARD, { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.status === 401 ? t("auth:wrongCredentials") : getErrorMessage(err.code));
      } else {
        setError(t("auth:networkError"));
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface-100">
      <div className="w-full max-w-sm rounded-card bg-white p-8 shadow-lg">
        <div className="mb-8 text-center">
          <h1 className="text-2xl font-bold text-gray-900">{t("auth:title")}</h1>
          <p className="mt-1 text-sm text-gray-500">{t("auth:subtitle")}</p>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4" data-testid="login-form">
          <div>
            <label htmlFor="email" className="mb-1 block text-sm font-medium text-gray-700">
              {t("auth:email")}
            </label>
            <input
              id="email"
              type="email"
              required
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full rounded-button border border-gray-300 px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
              placeholder="admin@ugcboost.kz"
              data-testid="email-input"
            />
          </div>

          <div>
            <label htmlFor="password" className="mb-1 block text-sm font-medium text-gray-700">
              {t("auth:password")}
            </label>
            <input
              id="password"
              type="password"
              required
              autoComplete="current-password"
              minLength={6}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-button border border-gray-300 px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
              data-testid="password-input"
            />
          </div>

          {error && (
            <p className="text-sm text-red-600" role="alert" data-testid="login-error">
              {error}
            </p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full rounded-button bg-primary px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-primary-600 disabled:opacity-50"
            data-testid="login-button"
          >
            {loading ? t("auth:loggingIn") : t("auth:login")}
          </button>
        </form>
      </div>
    </div>
  );
}
