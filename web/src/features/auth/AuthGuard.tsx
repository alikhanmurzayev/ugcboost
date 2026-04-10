import { useEffect, useState } from "react";
import { Navigate, Outlet } from "react-router-dom";
import { useAuthStore } from "@/stores/auth";
import { restoreSession } from "@/api/auth";
import { ROUTES } from "@/shared/constants/routes";

export default function AuthGuard() {
  const user = useAuthStore((s) => s.user);
  const setAuth = useAuthStore((s) => s.setAuth);
  const clearAuth = useAuthStore((s) => s.clearAuth);
  const [checking, setChecking] = useState(!user);

  useEffect(() => {
    if (user) return;

    // Restore session via refresh cookie (works after page reload)
    restoreSession()
      .then((session) => {
        if (session) {
          setAuth(session.user, session.token);
        } else {
          clearAuth();
        }
      })
      .catch(() => {
        clearAuth();
      })
      .finally(() => {
        setChecking(false);
      });
  }, [user, setAuth, clearAuth]);

  if (checking) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-surface-100">
        <div className="text-sm text-gray-400">Загрузка...</div>
      </div>
    );
  }

  if (!user) {
    return <Navigate to={"/" + ROUTES.LOGIN} replace />;
  }

  return <Outlet />;
}
