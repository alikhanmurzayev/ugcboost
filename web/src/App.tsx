import { BrowserRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LoginPage from "@/features/auth/LoginPage";
import AuthGuard from "@/features/auth/AuthGuard";
import DashboardLayout from "@/shared/layouts/DashboardLayout";
import DashboardPage from "@/features/dashboard/DashboardPage";
import BrandsPage from "@/features/brands/BrandsPage";
import BrandDetailPage from "@/features/brands/BrandDetailPage";
import AuditLogPage from "@/features/audit/AuditLogPage";
import { ROUTES } from "@/shared/constants/routes";

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
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path={ROUTES.LOGIN} element={<LoginPage />} />

          <Route element={<AuthGuard />}>
            <Route element={<DashboardLayout />}>
              <Route index element={<DashboardPage />} />
              <Route path={ROUTES.BRANDS} element={<BrandsPage />} />
              <Route path={ROUTES.BRANDS + "/:brandId"} element={<BrandDetailPage />} />
              <Route path={ROUTES.AUDIT} element={<AuditLogPage />} />
            </Route>
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
