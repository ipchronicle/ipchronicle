import { Navigate, Route, Routes } from "react-router";

import { useAuth } from "@/auth-context";
import { AppHeader } from "@/components/app-header";
import { AppSidebar } from "@/components/app-sidebar";
import { Card, CardContent } from "@/components/ui/card";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { Skeleton } from "@/components/ui/skeleton";
import { AccountPage } from "@/pages/account-page";
import { LoginPage } from "@/pages/login-page";
import { SystemStatusPage } from "@/pages/system-status-page";

function App() {
  const { state } = useAuth();
  if (state.status === "loading") {
    return (
      <div className="min-h-svh bg-background">
        <AppHeader />
        <LoadingPage />
      </div>
    );
  }

  if (state.status === "anonymous") {
    return (
      <div className="min-h-svh bg-background">
        <AppHeader />
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </Routes>
      </div>
    );
  }

  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset className="min-w-0">
        <AppHeader withSidebar />
        <Routes>
          <Route path="/" element={<SystemStatusPage />} />
          <Route path="/system/status" element={<SystemStatusPage />} />
          <Route path="/settings/account" element={<AccountPage />} />
          <Route path="/login" element={<Navigate to="/" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </SidebarInset>
    </SidebarProvider>
  );
}

function LoadingPage() {
  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-16">
      <div className="space-y-3" aria-busy="true">
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-8 w-52" />
        <Card className="mt-8">
          <CardContent className="space-y-3">
            <Skeleton className="h-10 w-10 rounded-md" />
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-64 max-w-full" />
          </CardContent>
        </Card>
      </div>
    </main>
  );
}

export default App;
