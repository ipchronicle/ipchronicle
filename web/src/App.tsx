import { Navigate, Route, Routes } from "react-router";

import { useAuth } from "@/auth-context";
import { AppHeader } from "@/components/app-header";
import { AppSidebar } from "@/components/app-sidebar";
import { NodeDetailLayout } from "@/components/node-detail-layout";
import { Card, CardContent } from "@/components/ui/card";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { Skeleton } from "@/components/ui/skeleton";
import { AccountPage } from "@/pages/account-page";
import { HistorySettingsPage } from "@/pages/history-settings-page";
import { HistoryPage } from "@/pages/history-page";
import { LoginPage } from "@/pages/login-page";
import { NodeNetworkPage } from "@/pages/node-network-page";
import { NodeProbePage } from "@/pages/node-probe-page";
import { NetworkSettingsPage } from "@/pages/network-settings-page";
import { NodeChangesPage } from "@/pages/node-changes-page";
import { NodesPage } from "@/pages/nodes-page";
import { NotificationsPage } from "@/pages/notifications-page";
import { NodeOverviewPage } from "@/pages/node-overview-page";
import { ProbeRunPage } from "@/pages/probe-run-page";
import { ProbeComparisonPage } from "@/pages/probe-comparison-page";
import { ProbeSnapshotPage } from "@/pages/probe-snapshot-page";
import { NodeSettingsPage } from "@/pages/node-settings-page";
import { SystemStatusPage } from "@/pages/system-status-page";
import { SystemSettingsPage } from "@/pages/system-settings-page";

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
          <Route path="/nodes" element={<NodesPage />} />
          <Route path="/nodes/:nodeId" element={<NodeDetailLayout />}>
            <Route index element={<NodeOverviewPage />} />
            <Route path="network" element={<NodeNetworkPage />} />
            <Route path="probe" element={<NodeProbePage />} />
            <Route path="changes" element={<NodeChangesPage />} />
            <Route path="settings" element={<NodeSettingsPage />} />
          </Route>
          <Route path="/probe-runs/:runId" element={<ProbeRunPage />} />
          <Route path="/history" element={<HistoryPage />} />
          <Route path="/notifications" element={<NotificationsPage />} />
          <Route path="/history/compare" element={<ProbeComparisonPage />} />
          <Route
            path="/probe-snapshots/:snapshotId"
            element={<ProbeSnapshotPage />}
          />
          <Route path="/settings/account" element={<AccountPage />} />
          <Route path="/settings/system" element={<SystemSettingsPage />} />
          <Route path="/settings/network" element={<NetworkSettingsPage />} />
          <Route path="/settings/history" element={<HistorySettingsPage />} />
          <Route path="/login" element={<Navigate to="/" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </SidebarInset>
    </SidebarProvider>
  );
}

function LoadingPage() {
  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-16">
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
