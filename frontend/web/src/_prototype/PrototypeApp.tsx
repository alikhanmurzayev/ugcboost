import { Navigate, Route, Routes } from "react-router-dom";
import PrototypeLayout from "./PrototypeLayout";
import { ROUTES } from "./routes";
import VerificationPage from "./features/creatorApplications/VerificationPage";
import ModerationPage from "./features/creatorApplications/ModerationPage";
import ContractsPage from "./features/creatorApplications/ContractsPage";
import RejectedPage from "./features/creatorApplications/RejectedPage";
import CreatorsPage from "./features/creatorApplications/CreatorsPage";
import CampaignsPage from "./features/campaigns/CampaignsPage";
import CampaignDetailPage from "./features/campaigns/CampaignDetailPage";
import CampaignNewPage from "./features/campaigns/CampaignNewPage";

export default function PrototypeApp() {
  return (
    <Routes>
      <Route element={<PrototypeLayout />}>
        <Route index element={<Navigate to={ROUTES.CAMPAIGNS_ACTIVE} replace />} />

        <Route
          path={ROUTES.CAMPAIGNS}
          element={<Navigate to={ROUTES.CAMPAIGNS_ACTIVE} replace />}
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

        <Route
          path={ROUTES.CREATOR_APP_VERIFICATION}
          element={<VerificationPage />}
        />
        <Route
          path={ROUTES.CREATOR_APP_MODERATION}
          element={<ModerationPage />}
        />
        <Route
          path={ROUTES.CREATOR_APP_CONTRACTS}
          element={<ContractsPage />}
        />
        <Route
          path={ROUTES.CREATOR_APP_REJECTED}
          element={<RejectedPage />}
        />
        <Route path={ROUTES.CREATORS} element={<CreatorsPage />} />
      </Route>
    </Routes>
  );
}
