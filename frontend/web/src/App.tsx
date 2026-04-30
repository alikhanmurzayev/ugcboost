import "@/shared/i18n/config";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LoginPage from "@/features/auth/LoginPage";
import AuthGuard from "@/features/auth/AuthGuard";
import DashboardLayout from "@/shared/layouts/DashboardLayout";
import DashboardPage from "@/features/dashboard/DashboardPage";
import BrandsPage from "@/features/brands/BrandsPage";
import BrandDetailPage from "@/features/brands/BrandDetailPage";
import AuditLogPage from "@/features/audit/AuditLogPage";
import VerificationPage from "@/features/creatorApplications/VerificationPage";
import ModerationPage from "@/features/creatorApplications/ModerationPage";
import ContractsPage from "@/features/creatorApplications/ContractsPage";
import RejectedPage from "@/features/creatorApplications/RejectedPage";
import CreatorsPage from "@/features/creatorApplications/CreatorsPage";
import CampaignsPage from "@/features/campaigns/CampaignsPage";
import CampaignDetailPage from "@/features/campaigns/CampaignDetailPage";
import CampaignNewPage from "@/features/campaigns/CampaignNewPage";
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
              <Route
                path={ROUTES.CAMPAIGNS}
                element={<Navigate to={"/" + ROUTES.CAMPAIGNS_ACTIVE} replace />}
              />
              <Route path={ROUTES.CAMPAIGN_NEW} element={<CampaignNewPage />} />
              <Route
                path={ROUTES.CAMPAIGN_EDIT_PATTERN}
                element={<CampaignNewPage />}
              />
              <Route
                path={ROUTES.CAMPAIGNS_ACTIVE}
                element={<CampaignsPage status="active" />}
              />
              <Route
                path={ROUTES.CAMPAIGNS_PENDING}
                element={<CampaignsPage status="pending_moderation" />}
              />
              <Route
                path={ROUTES.CAMPAIGNS_REJECTED}
                element={<CampaignsPage status="rejected" />}
              />
              <Route
                path={ROUTES.CAMPAIGNS_DRAFT}
                element={<CampaignsPage status="draft" />}
              />
              <Route
                path={ROUTES.CAMPAIGNS_COMPLETED}
                element={<CampaignsPage status="completed" />}
              />
              <Route
                path={ROUTES.CAMPAIGN_DETAIL_PATTERN}
                element={<CampaignDetailPage />}
              />
              <Route element={<RoleGuard allowedRoles={[Roles.ADMIN]} />}>
                <Route path={ROUTES.AUDIT} element={<AuditLogPage />} />
                <Route path={ROUTES.CREATOR_APP_VERIFICATION} element={<VerificationPage />} />
                <Route path={ROUTES.CREATOR_APP_MODERATION} element={<ModerationPage />} />
                <Route path={ROUTES.CREATOR_APP_CONTRACTS} element={<ContractsPage />} />
                <Route path={ROUTES.CREATOR_APP_REJECTED} element={<RejectedPage />} />
                <Route path={ROUTES.CREATORS} element={<CreatorsPage />} />
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
