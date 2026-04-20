import "@/shared/i18n/config";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LoginPage from "@/features/auth/LoginPage";
import AuthGuard from "@/features/auth/AuthGuard";
import DashboardLayout from "@/shared/layouts/DashboardLayout";
import DashboardPage from "@/features/dashboard/DashboardPage";
import BrandsPage from "@/features/brands/BrandsPage";
import BrandDetailPage from "@/features/brands/BrandDetailPage";
import AuditLogPage from "@/features/audit/AuditLogPage";
import RoleGuard from "@/features/auth/RoleGuard";
import ErrorBoundary from "@/shared/components/ErrorBoundary";
import { ROUTES } from "@/shared/constants/routes";
import { Roles } from "@/shared/constants/roles";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
});

function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Routes>
          <Route path={ROUTES.LOGIN} element={<LoginPage />} />

          <Route element={<AuthGuard />}>
            <Route element={<DashboardLayout />}>
              <Route index element={<DashboardPage />} />
              <Route path={ROUTES.BRANDS} element={<BrandsPage />} />
              <Route path={ROUTES.BRAND_DETAIL_PATTERN} element={<BrandDetailPage />} />
              <Route element={<RoleGuard allowedRoles={[Roles.ADMIN]} />}>
                <Route path={ROUTES.AUDIT} element={<AuditLogPage />} />
              </Route>
            </Route>
          </Route>
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

export default App;
