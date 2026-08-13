import { useCallback, useEffect, useState } from "react";
import {
  Activity,
  LoaderCircle,
  Network,
  RadioTower,
  RefreshCw,
  Server,
  TriangleAlert,
  Unplug,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";

import { getNodeNetwork, type NodeNetworkState } from "@/api/network";
import { startNodeSyncSession, stopNodeSyncSession } from "@/api/nodes";
import { getNodeProbe, type NodeProbeState } from "@/api/probes";
import { useAuth } from "@/auth-context";
import { useNodeDetail } from "@/components/node-detail-layout";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
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
import { formatAPIError } from "@/lib/api-error";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; network: NodeNetworkState; probe: NodeProbeState }
  | { kind: "error" };

export function NodeOverviewPage() {
  const { nodeId = "" } = useParams();
  const { node, replaceNode } = useNodeDetail();
  const { state: authState } = useAuth();
  const { i18n, t } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [feedback, setFeedback] = useState<string>();

  const load = useCallback(
    async (signal?: AbortSignal, initial = false) => {
      if (initial) setState({ kind: "loading" });
      else setRefreshing(true);
      try {
        const [network, probe] = await Promise.all([
          getNodeNetwork(nodeId, signal),
          getNodeProbe(nodeId, signal),
        ]);
        setState({ kind: "success", network, probe });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setState({ kind: "error" });
      } finally {
        setRefreshing(false);
      }
    },
    [nodeId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    return () => controller.abort();
  }, [load]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function toggleSync() {
    setSyncing(true);
    setFeedback(undefined);
    try {
      replaceNode(
        await (node.syncStatus === undefined
          ? startNodeSyncSession(node.id, csrfToken)
          : stopNodeSyncSession(node.id, csrfToken)),
      );
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setSyncing(false);
    }
  }

  if (state.kind === "loading") {
    return <OverviewSkeleton />;
  }
  if (state.kind === "error") {
    return (
      <Alert variant="destructive">
        <TriangleAlert aria-hidden="true" />
        <AlertTitle>{t("nodeDetail.overview.loadFailed")}</AlertTitle>
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
    );
  }

  const publicAddresses = state.network.publicAddresses;
  const natCount = publicAddresses.filter(
    (address) => address.likelyNat,
  ).length;
  const supportsSync = node.capabilities.includes("sync-wakeup-v1");
  const lastRun = state.probe.recentRuns[0];

  return (
    <div className="space-y-4" aria-live="polite">
      {feedback ? (
        <Alert variant="destructive">
          <TriangleAlert aria-hidden="true" />
          <AlertDescription>{feedback}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex justify-end">
        <Button
          variant="outline"
          disabled={refreshing}
          onClick={() => void load()}
        >
          <RefreshCw
            data-icon="inline-start"
            aria-hidden="true"
            className={refreshing ? "animate-spin" : undefined}
          />
          {t("nodeDetail.overview.refresh")}
        </Button>
      </div>
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Server aria-hidden="true" className="size-4" />
              {t("nodeDetail.overview.node.title")}
            </CardTitle>
            <CardDescription>
              {t("nodeDetail.overview.node.detail")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <OverviewValue
                label={t("nodes.inventory.agent")}
                value={`${node.agentVersion} · ${node.operatingSystem}/${node.architecture}`}
              />
              <OverviewValue
                label={t("nodes.inventory.configuration")}
                value={t("nodeDetail.overview.node.configuration", {
                  status: t(`nodes.configuration.${node.configurationStatus}`),
                  applied: node.appliedConfigurationRevision,
                  desired: node.desiredConfigurationRevision,
                })}
              />
              <OverviewValue
                label={t("nodes.inventory.lastSeen")}
                value={formatTime(
                  node.lastSeenAt,
                  i18n.resolvedLanguage,
                  t("nodes.notAvailable"),
                )}
              />
              <OverviewValue
                label={t("nodeDetail.overview.node.registered")}
                value={formatTime(
                  node.registeredAt,
                  i18n.resolvedLanguage,
                  t("nodes.notAvailable"),
                )}
              />
              <OverviewValue
                label={t("nodeDetail.overview.node.capabilities")}
                value={String(node.capabilities.length)}
              />
              <OverviewValue
                label={t("nodeDetail.overview.node.source")}
                value={
                  node.sourceRevision?.slice(0, 12) ?? t("nodes.notAvailable")
                }
              />
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <RadioTower aria-hidden="true" className="size-4" />
              {t("nodeDetail.overview.sync.title")}
            </CardTitle>
            <CardDescription>
              {t("nodeDetail.overview.sync.detail")}
            </CardDescription>
            <CardAction>
              <Badge variant={node.syncStatus ? "secondary" : "outline"}>
                {node.syncStatus
                  ? t(`nodes.sync.${node.syncStatus}`)
                  : t("nodeDetail.overview.sync.inactive")}
              </Badge>
            </CardAction>
          </CardHeader>
          <CardContent className="space-y-4">
            {node.syncExpiresAt ? (
              <p className="text-sm text-muted-foreground">
                {t("nodes.sync.until")}{" "}
                {formatTime(
                  node.syncExpiresAt,
                  i18n.resolvedLanguage,
                  t("nodes.notAvailable"),
                )}
              </p>
            ) : null}
            <Button
              variant="outline"
              disabled={
                syncing ||
                node.deletionStatus !== undefined ||
                (node.syncStatus === undefined && !supportsSync)
              }
              onClick={() => void toggleSync()}
            >
              {syncing ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : node.syncStatus === undefined ? (
                <RadioTower data-icon="inline-start" aria-hidden="true" />
              ) : (
                <Unplug data-icon="inline-start" aria-hidden="true" />
              )}
              {node.syncStatus === undefined
                ? t("nodes.sync.start")
                : t("nodes.sync.stop")}
            </Button>
            {!supportsSync ? (
              <p className="text-xs text-muted-foreground">
                {t("nodes.sync.unsupported")}
              </p>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Network aria-hidden="true" className="size-4" />
              {t("nodeDetail.overview.network.title")}
            </CardTitle>
            <CardDescription>
              {t("nodeDetail.overview.network.detail")}
            </CardDescription>
            <CardAction>
              <Badge variant="secondary">{publicAddresses.length}</Badge>
            </CardAction>
          </CardHeader>
          <CardContent className="space-y-3">
            {publicAddresses.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t("nodeDetail.overview.network.empty")}
              </p>
            ) : (
              publicAddresses.map((address) => (
                <div
                  key={address.id}
                  className="flex flex-wrap items-center justify-between gap-2 rounded-md border p-3"
                >
                  <span className="font-mono text-xs">{address.address}</span>
                  <div className="flex gap-2">
                    <Badge variant={address.available ? "default" : "outline"}>
                      {address.available
                        ? t("network.publicAddresses.available")
                        : t("network.publicAddresses.unavailable")}
                    </Badge>
                    <Badge variant="secondary">
                      {address.family.toUpperCase()}
                    </Badge>
                  </div>
                </div>
              ))
            )}
            {natCount > 0 ? (
              <p className="text-xs text-muted-foreground">
                {t("nodeDetail.overview.network.nat", { count: natCount })}
              </p>
            ) : null}
            <Button variant="outline" size="sm" asChild>
              <Link to={`/nodes/${nodeId}/network`}>
                {t("nodeDetail.overview.network.open")}
              </Link>
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity aria-hidden="true" className="size-4" />
              {t("nodeDetail.overview.probe.title")}
            </CardTitle>
            <CardDescription>
              {t("nodeDetail.overview.probe.detail")}
            </CardDescription>
            {lastRun ? (
              <CardAction>
                <Badge
                  variant={
                    ["failed", "interrupted"].includes(lastRun.status)
                      ? "destructive"
                      : "outline"
                  }
                >
                  {t(`probe.state.${lastRun.status}`)}
                </Badge>
              </CardAction>
            ) : null}
          </CardHeader>
          <CardContent className="space-y-4">
            {state.probe.pausedLowMemory ? (
              <Alert>
                <TriangleAlert aria-hidden="true" />
                <AlertDescription>
                  {t("probe.lowMemory.detail")}
                </AlertDescription>
              </Alert>
            ) : null}
            <dl className="grid gap-4 sm:grid-cols-2">
              <OverviewValue
                label={t("probe.status.next")}
                value={formatTime(
                  state.probe.agentStatus?.nextScheduledAt,
                  i18n.resolvedLanguage,
                  t("nodes.notAvailable"),
                )}
              />
              <OverviewValue
                label={t("probe.status.last")}
                value={formatTime(
                  state.probe.agentStatus?.lastOccurrenceAt,
                  i18n.resolvedLanguage,
                  t("nodes.notAvailable"),
                )}
              />
            </dl>
            <Button variant="outline" size="sm" asChild>
              <Link to={`/nodes/${nodeId}/probe`}>
                {t("nodeDetail.overview.probe.open")}
              </Link>
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function OverviewValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-words font-medium">{value}</dd>
    </div>
  );
}

function formatTime(
  value: string | undefined,
  locale: string | undefined,
  fallback: string,
) {
  if (!value) return fallback;
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function OverviewSkeleton() {
  return (
    <div className="grid gap-4 xl:grid-cols-2" aria-busy="true">
      {[0, 1, 2, 3].map((item) => (
        <Card key={item}>
          <CardHeader>
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-64 max-w-full" />
          </CardHeader>
          <CardContent>
            <Skeleton className="h-24 w-full" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
