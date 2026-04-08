import { BrowserRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import LoginPage from "@/features/auth/LoginPage";
import AuthGuard from "@/features/auth/AuthGuard";
import DashboardLayout from "@/shared/layouts/DashboardLayout";
import DashboardPage from "@/features/dashboard/DashboardPage";
import BrandsPage from "@/features/brands/BrandsPage";
import BrandDetailPage from "@/features/brands/BrandDetailPage";

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
          <Route path="/login" element={<LoginPage />} />

          <Route element={<AuthGuard />}>
            <Route element={<DashboardLayout />}>
              <Route index element={<DashboardPage />} />
              <Route path="brands" element={<BrandsPage />} />
              <Route path="brands/:brandId" element={<BrandDetailPage />} />
            </Route>
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
