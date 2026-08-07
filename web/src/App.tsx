import { Navigate, Route, Routes } from "react-router";

import { useAuth } from "@/auth-context";
import { AppHeader } from "@/components/app-header";
import { Skeleton } from "@/components/ui/skeleton";
import { AccountPage } from "@/pages/account-page";
import { LoginPage } from "@/pages/login-page";
import { SystemStatusPage } from "@/pages/system-status-page";

function App() {
  const { state } = useAuth();
  return (
    <div className="min-h-svh bg-background">
      <AppHeader />
      {state.status === "loading" ? (
        <LoadingPage />
      ) : state.status === "anonymous" ? (
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      ) : (
        <Routes>
          <Route path="/" element={<SystemStatusPage />} />
          <Route path="/system/status" element={<SystemStatusPage />} />
          <Route path="/settings/account" element={<AccountPage />} />
          <Route path="/login" element={<Navigate to="/" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      )}
    </div>
  );
}

function LoadingPage() {
  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-16">
      <div className="space-y-3" aria-busy="true">
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-8 w-52" />
        <Skeleton className="mt-8 h-36 w-full" />
      </div>
    </main>
  );
}

export default App;
