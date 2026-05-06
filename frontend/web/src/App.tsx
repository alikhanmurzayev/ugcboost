import "@/shared/i18n/config";
import { lazy, Suspense } from "react";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import LoginPage from "@/features/auth/LoginPage";
import AuthGuard from "@/features/auth/AuthGuard";
import DashboardLayout from "@/shared/layouts/DashboardLayout";
import DashboardPage from "@/features/dashboard/DashboardPage";
import BrandsPage from "@/features/brands/BrandsPage";
import BrandDetailPage from "@/features/brands/BrandDetailPage";
import AuditLogPage from "@/features/audit/AuditLogPage";
import VerificationPage from "@/features/creatorApplications/VerificationPage";
import ModerationPage from "@/features/creatorApplications/ModerationPage";
import RejectedPage from "@/features/creatorApplications/stubs/RejectedPage";
import CreatorsListPage from "@/features/creators/CreatorsListPage";
import CampaignsListPage from "@/features/campaigns/CampaignsListPage";
import CampaignCreatePage from "@/features/campaigns/CampaignCreatePage";
import RoleGuard from "@/features/auth/RoleGuard";
import ErrorBoundary from "@/shared/components/ErrorBoundary";
import Spinner from "@/shared/components/Spinner";
import { ROUTES } from "@/shared/constants/routes";
import { Roles } from "@/shared/constants/roles";

// Aidana's brand-cabinet prototype — isolated under /prototype/* with its own
// layout, mock API, types and i18n namespaces. Lazy-loaded so it stays out of
// the main bundle until visited. As features are reimplemented properly
// against the real backend, drop the corresponding folders from src/_prototype/.
const PrototypeApp = lazy(() => import("@/_prototype/PrototypeApp"));

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
              <Route
                path="/prototype/*"
                element={
                  <Suspense fallback={<Spinner className="mt-12" />}>
                    <PrototypeApp />
                  </Suspense>
                }
              />

              <Route element={<DashboardLayout />}>
                <Route index element={<DashboardPage />} />
                <Route path={ROUTES.BRANDS} element={<BrandsPage />} />
                <Route
                  path={ROUTES.BRAND_DETAIL_PATTERN}
                  element={<BrandDetailPage />}
                />

                <Route element={<RoleGuard allowedRoles={[Roles.ADMIN]} />}>
                  <Route path={ROUTES.AUDIT} element={<AuditLogPage />} />
                  <Route
                    path={ROUTES.CREATOR_APP_VERIFICATION}
                    element={<VerificationPage />}
                  />
                  <Route
                    path={ROUTES.CREATOR_APP_MODERATION}
                    element={<ModerationPage />}
                  />
                  <Route
                    path={ROUTES.CREATOR_APP_REJECTED}
                    element={<RejectedPage />}
                  />
                  <Route path={ROUTES.CREATORS} element={<CreatorsListPage />} />
                  <Route path={ROUTES.CAMPAIGNS} element={<CampaignsListPage />} />
                  <Route
                    path={ROUTES.CAMPAIGN_NEW}
                    element={<CampaignCreatePage />}
                  />
                  <Route
                    path={ROUTES.CAMPAIGN_DETAIL_PATTERN}
                    element={<ComingSoonPage testid="campaign-detail-stub" />}
                  />
                </Route>
              </Route>
            </Route>
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}

function ComingSoonPage({ testid }: { testid: string }) {
  const { t } = useTranslation("common");
  return (
    <div data-testid={testid} className="text-sm text-gray-500">
      {t("comingSoon")}
    </div>
  );
}

export default App;
