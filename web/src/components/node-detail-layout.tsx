import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  ArrowLeft,
  History,
  LayoutDashboard,
  LoaderCircle,
  Network,
  RadioTower,
  RefreshCw,
  ScanSearch,
  Settings,
  TriangleAlert,
  Unplug,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  Link,
  Outlet,
  useLocation,
  useNavigate,
  useOutletContext,
  useParams,
} from "react-router";

import {
  listNodes,
  startNodeSyncSession,
  stopNodeSyncSession,
  type Node,
} from "@/api/nodes";
import { useAuth } from "@/auth-context";
import { CompleteProbeDialog } from "@/components/complete-probe-dialog";
import { NodeStatusBadge } from "@/components/node-status-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatAPIError } from "@/lib/api-error";

const nodeRefreshIntervalMilliseconds = 3_000;

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; node: Node }
  | { kind: "not-found" }
  | { kind: "error" };

type NodeDetailContext = {
  node: Node;
  replaceNode: (node: Node) => void;
};

export function NodeDetailLayout() {
  const { nodeId = "" } = useParams();
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const { state: authState } = useAuth();
  const { t } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [syncing, setSyncing] = useState(false);
  const [actionError, setActionError] = useState<string>();

  const load = useCallback(
    async (signal?: AbortSignal, initial = false) => {
      if (initial) setState({ kind: "loading" });
      try {
        const nodes = await listNodes(signal);
        const node = nodes.find((item) => item.id === nodeId);
        setState(
          node === undefined
            ? { kind: "not-found" }
            : { kind: "success", node },
        );
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        if (initial) setState({ kind: "error" });
      }
    },
    [nodeId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    if (state.kind !== "success") return;
    let inFlight = false;
    let disposed = false;
    let controller: AbortController | undefined;
    const refresh = async () => {
      if (disposed || inFlight || document.visibilityState !== "visible")
        return;
      inFlight = true;
      controller = new AbortController();
      try {
        await load(controller.signal);
      } finally {
        inFlight = false;
        controller = undefined;
      }
    };
    const wake = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    const interval = window.setInterval(
      () => void refresh(),
      nodeRefreshIntervalMilliseconds,
    );
    document.addEventListener("visibilitychange", wake);
    window.addEventListener("focus", wake);
    return () => {
      disposed = true;
      controller?.abort();
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", wake);
      window.removeEventListener("focus", wake);
    };
  }, [load, state.kind]);

  const activeTab = pathname.endsWith("/network")
    ? "network"
    : pathname.endsWith("/probe")
      ? "probe"
      : pathname.endsWith("/changes")
        ? "changes"
        : pathname.endsWith("/settings")
          ? "settings"
          : "overview";
  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function toggleSync(node: Node) {
    setSyncing(true);
    setActionError(undefined);
    try {
      const updated = await (node.syncStatus === undefined
        ? startNodeSyncSession(node.id, csrfToken)
        : stopNodeSyncSession(node.id, csrfToken));
      setState({ kind: "success", node: updated });
    } catch (error) {
      setActionError(formatAPIError(error, t));
    } finally {
      setSyncing(false);
    }
  }

  return (
    <main className="w-full min-w-0 px-4 py-8 sm:px-6 sm:py-10">
      {state.kind === "loading" ? <NodeDetailSkeleton /> : null}
      {state.kind === "not-found" ? (
        <Alert variant="destructive">
          <AlertTitle>{t("nodeDetail.notFound")}</AlertTitle>
          <AlertDescription>
            <Button variant="outline" size="sm" className="mt-3" asChild>
              <Link to="/nodes">
                <ArrowLeft data-icon="inline-start" aria-hidden="true" />
                {t("nodeDetail.back")}
              </Link>
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {state.kind === "error" ? (
        <Alert variant="destructive">
          <AlertTitle>{t("nodeDetail.loadFailed")}</AlertTitle>
          <AlertDescription>
            <Button
              variant="outline"
              size="sm"
              className="mt-3"
              onClick={() => void load(undefined, true)}
            >
              <RefreshCw data-icon="inline-start" aria-hidden="true" />
              {t("nodeDetail.retry")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {state.kind === "success" ? (
        <>
          <Card className="gap-0 py-0">
            <CardContent className="px-4 py-4 sm:px-5">
              <div className="flex min-w-0 flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div className="flex min-w-0 items-center gap-3">
                  <Button variant="outline" size="icon" asChild>
                    <Link to="/nodes" aria-label={t("nodeDetail.back")}>
                      <ArrowLeft aria-hidden="true" />
                    </Link>
                  </Button>
                  <div className="min-w-0">
                    <div className="flex min-w-0 flex-wrap items-center gap-2.5">
                      <h1 className="min-w-0 truncate text-2xl font-semibold">
                        {state.node.name}
                      </h1>
                      <NodeStatusBadge node={state.node} />
                    </div>
                    <p className="mt-1 truncate text-sm text-muted-foreground">
                      {state.node.hostname !== state.node.name
                        ? state.node.hostname
                        : t("nodeDetail.section")}
                    </p>
                  </div>
                </div>

                <div className="flex w-full flex-wrap items-center gap-2 lg:w-auto lg:justify-end">
                  {state.node.syncStatus ? (
                    <Badge variant="secondary" className="max-w-full">
                      {t(`nodes.sync.${state.node.syncStatus}`)}
                    </Badge>
                  ) : null}
                  <Button
                    variant="outline"
                    className="flex-1 sm:flex-none"
                    disabled={
                      syncing ||
                      state.node.deletionStatus !== undefined ||
                      (state.node.syncStatus === undefined &&
                        !state.node.capabilities.includes("sync-wakeup-v1"))
                    }
                    onClick={() => void toggleSync(state.node)}
                  >
                    {syncing ? (
                      <LoaderCircle
                        data-icon="inline-start"
                        aria-hidden="true"
                        className="animate-spin"
                      />
                    ) : state.node.syncStatus === undefined ? (
                      <RadioTower data-icon="inline-start" aria-hidden="true" />
                    ) : (
                      <Unplug data-icon="inline-start" aria-hidden="true" />
                    )}
                    {state.node.syncStatus === undefined
                      ? t("nodes.sync.start")
                      : t("nodes.sync.stop")}
                  </Button>
                  {activeTab !== "probe" ? (
                    <CompleteProbeDialog
                      nodeId={nodeId}
                      csrfToken={csrfToken}
                      onCreated={() => navigate(`/nodes/${nodeId}/probe`)}
                    >
                      <Button
                        className="flex-1 sm:flex-none"
                        disabled={
                          state.node.status !== "online" ||
                          !state.node.enabled ||
                          state.node.deletionStatus !== undefined
                        }
                      >
                        <ScanSearch
                          data-icon="inline-start"
                          aria-hidden="true"
                        />
                        {t("probe.runNow")}
                      </Button>
                    </CompleteProbeDialog>
                  ) : null}
                </div>
              </div>
            </CardContent>

            <div className="border-t px-3 sm:px-4">
              <div className="overflow-x-auto">
                <Tabs value={activeTab}>
                  <TabsList
                    variant="line"
                    className="h-11 min-w-max justify-start gap-0 pb-1 [&_svg]:hidden sm:gap-2 sm:[&_svg]:block"
                    aria-label={t("nodeDetail.tabs.label")}
                  >
                    <TabsTrigger
                      value="overview"
                      asChild
                      className="flex-none px-2.5"
                    >
                      <Link to={`/nodes/${nodeId}`}>
                        <LayoutDashboard aria-hidden="true" />
                        {t("nodeDetail.tabs.overview")}
                      </Link>
                    </TabsTrigger>
                    <TabsTrigger
                      value="network"
                      asChild
                      className="flex-none px-2.5"
                    >
                      <Link to={`/nodes/${nodeId}/network`}>
                        <Network aria-hidden="true" />
                        {t("nodeDetail.tabs.network")}
                      </Link>
                    </TabsTrigger>
                    <TabsTrigger
                      value="probe"
                      asChild
                      className="flex-none px-2.5"
                    >
                      <Link to={`/nodes/${nodeId}/probe`}>
                        <Activity aria-hidden="true" />
                        {t("nodeDetail.tabs.probe")}
                      </Link>
                    </TabsTrigger>
                    <TabsTrigger
                      value="changes"
                      asChild
                      className="flex-none px-2.5"
                    >
                      <Link to={`/nodes/${nodeId}/changes`}>
                        <History aria-hidden="true" />
                        {t("nodeDetail.tabs.changes")}
                      </Link>
                    </TabsTrigger>
                    <TabsTrigger
                      value="settings"
                      asChild
                      className="flex-none px-2.5"
                    >
                      <Link to={`/nodes/${nodeId}/settings`}>
                        <Settings aria-hidden="true" />
                        {t("nodeDetail.tabs.settings")}
                      </Link>
                    </TabsTrigger>
                  </TabsList>
                </Tabs>
              </div>
            </div>
          </Card>
          {actionError ? (
            <Alert variant="destructive" className="mt-4">
              <TriangleAlert aria-hidden="true" />
              <AlertDescription>{actionError}</AlertDescription>
            </Alert>
          ) : null}
          <div className="mt-4">
            <Outlet
              context={
                {
                  node: state.node,
                  replaceNode: (node: Node) =>
                    setState({ kind: "success", node }),
                } satisfies NodeDetailContext
              }
            />
          </div>
        </>
      ) : null}
    </main>
  );
}

export function useNodeDetail() {
  return useOutletContext<NodeDetailContext>();
}

function NodeDetailSkeleton() {
  return (
    <Card size="sm" aria-busy="true">
      <CardContent className="space-y-3">
        <Skeleton className="h-7 w-24" />
        <Skeleton className="h-8 w-52" />
        <Skeleton className="h-4 w-80 max-w-full" />
        <Skeleton className="h-9 w-full" />
      </CardContent>
    </Card>
  );
}
