import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  ArrowLeft,
  History,
  LayoutDashboard,
  Network,
  RefreshCw,
  Settings,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import {
  Link,
  Outlet,
  useLocation,
  useOutletContext,
  useParams,
} from "react-router";

import { listNodes, type Node } from "@/api/nodes";
import { NodeStatusBadge } from "@/components/node-status-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

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
  const { t } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });

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

  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
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
          <Card>
            <CardHeader>
              <CardTitle>
                <Button
                  variant="ghost"
                  size="sm"
                  asChild
                  className="mb-3 -ml-2"
                >
                  <Link to="/nodes">
                    <ArrowLeft data-icon="inline-start" aria-hidden="true" />
                    {t("nodeDetail.back")}
                  </Link>
                </Button>
                <p className="text-xs font-medium text-muted-foreground uppercase">
                  {t("nodeDetail.section")}
                </p>
                <h1 className="mt-2 truncate text-2xl font-semibold sm:text-3xl">
                  {state.node.name}
                </h1>
              </CardTitle>
              <CardDescription className="mt-1">
                {t("nodeDetail.identity", {
                  hostname: state.node.hostname,
                  version: state.node.agentVersion,
                })}
              </CardDescription>
              <CardAction>
                <NodeStatusBadge node={state.node} />
              </CardAction>
            </CardHeader>
            <CardContent className="border-t pt-3">
              <Tabs value={activeTab}>
                <TabsList
                  variant="line"
                  className="h-auto w-full justify-start overflow-x-auto pb-1"
                  aria-label={t("nodeDetail.tabs.label")}
                >
                  <TabsTrigger value="overview" asChild>
                    <Link to={`/nodes/${nodeId}`}>
                      <LayoutDashboard aria-hidden="true" />
                      {t("nodeDetail.tabs.overview")}
                    </Link>
                  </TabsTrigger>
                  <TabsTrigger value="network" asChild>
                    <Link to={`/nodes/${nodeId}/network`}>
                      <Network aria-hidden="true" />
                      {t("nodeDetail.tabs.network")}
                    </Link>
                  </TabsTrigger>
                  <TabsTrigger value="probe" asChild>
                    <Link to={`/nodes/${nodeId}/probe`}>
                      <Activity aria-hidden="true" />
                      {t("nodeDetail.tabs.probe")}
                    </Link>
                  </TabsTrigger>
                  <TabsTrigger value="changes" asChild>
                    <Link to={`/nodes/${nodeId}/changes`}>
                      <History aria-hidden="true" />
                      {t("nodeDetail.tabs.changes")}
                    </Link>
                  </TabsTrigger>
                  <TabsTrigger value="settings" asChild>
                    <Link to={`/nodes/${nodeId}/settings`}>
                      <Settings aria-hidden="true" />
                      {t("nodeDetail.tabs.settings")}
                    </Link>
                  </TabsTrigger>
                </TabsList>
              </Tabs>
            </CardContent>
          </Card>
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
    <Card aria-busy="true">
      <CardHeader>
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-8 w-52" />
        <Skeleton className="h-4 w-64 max-w-full" />
      </CardHeader>
      <CardContent>
        <Skeleton className="h-8 w-full" />
      </CardContent>
    </Card>
  );
}
