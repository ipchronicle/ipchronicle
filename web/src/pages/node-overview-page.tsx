import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Activity,
  ArrowRight,
  History,
  Network,
  RefreshCw,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";

import { getNodeNetwork, type NodeNetworkState } from "@/api/network";
import {
  getNodeProbe,
  type NodeProbeState,
  type ProbeRunSummary,
} from "@/api/probes";
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
import { publicAddressAvailability } from "@/lib/public-address";

const refreshIntervalMilliseconds = 5_000;
const activityLimit = 6;

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; network: NodeNetworkState; probe: NodeProbeState }
  | { kind: "error" };

type ActivityItem =
  | {
      kind: "probe";
      id: string;
      time: string;
      run: ProbeRunSummary;
    }
  | {
      kind: "address";
      id: string;
      time: string;
      event: NodeNetworkState["addressEvents"][number];
    }
  | {
      kind: "gap";
      id: string;
      time: string;
      gap: NodeNetworkState["addressGaps"][number];
    };

export function NodeOverviewPage() {
  const { nodeId = "" } = useParams();
  const { node } = useNodeDetail();
  const { i18n, t } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });

  const load = useCallback(
    async (signal?: AbortSignal, initial = false, quiet = false) => {
      if (initial) setState({ kind: "loading" });
      try {
        const [network, probe] = await Promise.all([
          getNodeNetwork(nodeId, signal),
          getNodeProbe(nodeId, signal),
        ]);
        setState({ kind: "success", network, probe });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        if (!quiet) setState({ kind: "error" });
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
    let active = true;
    let controller: AbortController | undefined;
    const refresh = () => {
      if (!active || document.visibilityState !== "visible") return;
      controller?.abort();
      controller = new AbortController();
      void load(controller.signal, false, true);
    };
    const timer = window.setInterval(refresh, refreshIntervalMilliseconds);
    document.addEventListener("visibilitychange", refresh);
    window.addEventListener("focus", refresh);
    return () => {
      active = false;
      controller?.abort();
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", refresh);
      window.removeEventListener("focus", refresh);
    };
  }, [load, state.kind]);

  const activity = useMemo(
    () => (state.kind === "success" ? recentActivity(state) : []),
    [state],
  );

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

  const lastRun = state.probe.recentRuns[0];

  return (
    <div className="space-y-4" aria-live="polite">
      {node.configurationStatus !== "current" ? (
        <Alert
          variant={
            node.configurationStatus === "failed" ? "destructive" : "default"
          }
        >
          <TriangleAlert aria-hidden="true" />
          <AlertTitle>
            {t("nodeDetail.overview.attention.configurationTitle")}
          </AlertTitle>
          <AlertDescription>
            {t("nodeDetail.overview.attention.configurationDetail", {
              status: t(`nodes.configuration.${node.configurationStatus}`),
              applied: node.appliedConfigurationRevision,
              desired: node.desiredConfigurationRevision,
            })}
          </AlertDescription>
        </Alert>
      ) : null}

      {state.probe.pausedLowMemory ? (
        <Alert variant="destructive">
          <TriangleAlert aria-hidden="true" />
          <AlertTitle>{t("probe.lowMemory.title")}</AlertTitle>
          <AlertDescription>{t("probe.lowMemory.detail")}</AlertDescription>
        </Alert>
      ) : null}

      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,2.2fr)_minmax(18rem,0.9fr)]">
        <div className="min-w-0 space-y-4">
          <Card className="min-w-0">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Network aria-hidden="true" className="size-4" />
                {t("nodeDetail.overview.network.title")}
              </CardTitle>
              <CardDescription>
                {t("nodeDetail.overview.network.detail")}
              </CardDescription>
              <CardAction className="flex items-center gap-2">
                <Badge variant="info">
                  {state.network.publicAddresses.length}
                </Badge>
                <Button variant="outline" size="sm" asChild>
                  <Link to={`/nodes/${nodeId}/network`}>
                    {t("nodeDetail.overview.network.open")}
                  </Link>
                </Button>
              </CardAction>
            </CardHeader>
            <CardContent>
              {state.network.publicAddresses.length === 0 ? (
                <p className="py-8 text-center text-sm text-muted-foreground">
                  {t("nodeDetail.overview.network.empty")}
                </p>
              ) : (
                <div className="divide-y overflow-hidden rounded-lg border">
                  {state.network.publicAddresses.map((address) => {
                    const availability = publicAddressAvailability(address);
                    return (
                      <div
                        key={address.id}
                        className="grid min-w-0 gap-3 p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
                      >
                        <div className="min-w-0">
                          <p className="break-all font-mono text-base font-semibold">
                            {address.address}
                          </p>
                          <div className="mt-2 flex flex-wrap items-center gap-2">
                            <Badge variant="secondary">
                              {t(`network.family.${address.family}`)}
                            </Badge>
                            <Badge
                              variant={
                                availability === "available"
                                  ? "success"
                                  : "warning"
                              }
                            >
                              {t(
                                `network.publicAddresses.status.${availability}`,
                              )}
                            </Badge>
                            <Badge variant="outline">
                              {address.probeEnabled
                                ? t("nodeDetail.overview.network.probeEnabled")
                                : t(
                                    "nodeDetail.overview.network.probeDisabled",
                                  )}
                            </Badge>
                            {address.likelyNat ? (
                              <Badge variant="warning">
                                {t("network.publicAddresses.nat")}
                              </Badge>
                            ) : null}
                            {address.proxyPath ? (
                              <Badge variant="info">
                                {t("network.publicAddresses.proxy")}
                              </Badge>
                            ) : null}
                          </div>
                        </div>
                        <div className="min-w-0 text-sm sm:text-right">
                          <p className="text-muted-foreground">
                            {t("nodeDetail.overview.network.lastSeen")}
                          </p>
                          <p className="mt-1 font-medium">
                            {formatTime(
                              address.lastSeenAt,
                              i18n.resolvedLanguage,
                              t("nodes.notAvailable"),
                            )}
                          </p>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </CardContent>
          </Card>

          <Card className="min-w-0">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Activity aria-hidden="true" className="size-4" />
                {t("nodeDetail.overview.activity.title")}
              </CardTitle>
              <CardDescription>
                {t("nodeDetail.overview.activity.detail")}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {activity.length === 0 ? (
                <p className="py-6 text-center text-sm text-muted-foreground">
                  {t("nodeDetail.overview.activity.empty")}
                </p>
              ) : (
                <div className="divide-y overflow-hidden rounded-lg border">
                  {activity.map((item) => (
                    <ActivityRow
                      key={`${item.kind}:${item.id}`}
                      item={item}
                      nodeId={nodeId}
                      locale={i18n.resolvedLanguage}
                    />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        <aside className="min-w-0 space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>{t("nodeDetail.overview.node.title")}</CardTitle>
              <CardDescription>
                {t("nodeDetail.overview.node.detail")}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <dl className="divide-y">
                <SideValue
                  label={t("nodes.inventory.status")}
                  value={t(`nodes.status.${node.status}`)}
                  tone={node.status === "online" ? "success" : "warning"}
                />
                <SideValue
                  label={t("nodes.inventory.lastSeen")}
                  value={formatTime(
                    node.lastSeenAt,
                    i18n.resolvedLanguage,
                    t("nodes.notAvailable"),
                  )}
                />
                <SideValue
                  label={t("nodes.inventory.agent")}
                  value={node.agentVersion}
                />
                <SideValue
                  label={t("nodes.inventory.configuration")}
                  value={t(`nodes.configuration.${node.configurationStatus}`)}
                  detail={`${node.appliedConfigurationRevision}/${node.desiredConfigurationRevision}`}
                  tone={
                    node.configurationStatus === "current"
                      ? "success"
                      : "warning"
                  }
                />
              </dl>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>{t("nodeDetail.overview.probe.title")}</CardTitle>
              <CardDescription>
                {t("nodeDetail.overview.probe.detail")}
              </CardDescription>
              <CardAction>
                <Button variant="outline" size="sm" asChild>
                  <Link to={`/nodes/${nodeId}/probe`}>
                    {t("nodeDetail.overview.probe.open")}
                  </Link>
                </Button>
              </CardAction>
            </CardHeader>
            <CardContent>
              <dl className="divide-y">
                <SideValue
                  label={t("nodeDetail.overview.activity.latestProbe")}
                  value={
                    lastRun
                      ? t(`probe.state.${lastRun.status}`)
                      : t("nodeDetail.overview.activity.noProbe")
                  }
                  detail={
                    lastRun
                      ? formatTime(
                          lastRun.startedAt,
                          i18n.resolvedLanguage,
                          t("nodes.notAvailable"),
                        )
                      : undefined
                  }
                  tone={lastRun?.status === "succeeded" ? "success" : undefined}
                />
                <SideValue
                  label={t("probe.status.next")}
                  value={formatTime(
                    state.probe.agentStatus?.nextScheduledAt,
                    i18n.resolvedLanguage,
                    t("nodes.notAvailable"),
                  )}
                />
                <SideValue
                  label={t("nodeDetail.overview.activity.currentTask")}
                  value={
                    state.probe.task
                      ? t(`probe.state.${state.probe.task.status}`)
                      : t("nodeDetail.overview.activity.noTask")
                  }
                  tone={
                    state.probe.task?.status === "pending"
                      ? "warning"
                      : undefined
                  }
                />
              </dl>
            </CardContent>
          </Card>
        </aside>
      </div>
    </div>
  );
}

function ActivityRow({
  item,
  nodeId,
  locale,
}: {
  item: ActivityItem;
  nodeId: string;
  locale: string | undefined;
}) {
  const { t } = useTranslation();
  if (item.kind === "probe") {
    return (
      <Link
        to={`/probe-runs/${item.run.id}`}
        className="flex min-w-0 items-center gap-3 p-4 transition-colors hover:bg-muted/50"
      >
        <Activity aria-hidden="true" className="size-4 shrink-0" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">
              {t(`probe.trigger.${item.run.trigger}`)}
            </span>
            <ProbeStateBadge status={item.run.status} />
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("probe.runs.progress", {
              completed: item.run.completedExecutions,
              total: item.run.expectedExecutions,
            })}
          </p>
        </div>
        <time className="hidden shrink-0 text-sm text-muted-foreground sm:block">
          {formatTime(item.time, locale, "")}
        </time>
        <ArrowRight aria-hidden="true" className="size-4 shrink-0" />
      </Link>
    );
  }

  if (item.kind === "address") {
    return (
      <Link
        to={`/nodes/${nodeId}/changes`}
        className="flex min-w-0 items-center gap-3 p-4 transition-colors hover:bg-muted/50"
      >
        <History aria-hidden="true" className="size-4 shrink-0" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">
              {t(`network.addressHistory.kind.${item.event.kind}`)}
            </span>
            <Badge variant="secondary">
              {t(`network.family.${item.event.family}`)}
            </Badge>
          </div>
          <p className="mt-1 break-all font-mono text-sm text-muted-foreground">
            {item.event.publicAddress ?? t("network.observation.unknown")}
          </p>
        </div>
        <time className="hidden shrink-0 text-sm text-muted-foreground sm:block">
          {formatTime(item.time, locale, "")}
        </time>
        <ArrowRight aria-hidden="true" className="size-4 shrink-0" />
      </Link>
    );
  }

  return (
    <Link
      to={`/nodes/${nodeId}/changes`}
      className="flex min-w-0 items-center gap-3 p-4 transition-colors hover:bg-muted/50"
    >
      <TriangleAlert
        aria-hidden="true"
        className="size-4 shrink-0 text-destructive"
      />
      <div className="min-w-0 flex-1">
        <p className="font-medium">{t("network.addressHistory.gap")}</p>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("nodeDetail.overview.activity.gap", {
            count: item.gap.droppedCount,
          })}
        </p>
      </div>
      <time className="hidden shrink-0 text-sm text-muted-foreground sm:block">
        {formatTime(item.time, locale, "")}
      </time>
      <ArrowRight aria-hidden="true" className="size-4 shrink-0" />
    </Link>
  );
}

function ProbeStateBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  const destructive = ["failed", "rejected", "expired", "interrupted"].includes(
    status,
  );
  const warning = status === "partial" || status === "skipped";
  return (
    <Badge
      variant={
        destructive
          ? "destructive"
          : warning || status === "pending"
            ? "secondary"
            : "outline"
      }
    >
      {t(`probe.state.${status}`)}
    </Badge>
  );
}

function recentActivity({
  network,
  probe,
}: {
  network: NodeNetworkState;
  probe: NodeProbeState;
}) {
  const items: ActivityItem[] = [
    ...probe.recentRuns.map((run) => ({
      kind: "probe" as const,
      id: run.id,
      time: run.startedAt,
      run,
    })),
    ...network.addressEvents.map((event) => ({
      kind: "address" as const,
      id: event.id,
      time: event.observedAt,
      event,
    })),
    ...network.addressGaps.map((gap) => ({
      kind: "gap" as const,
      id: gap.id,
      time: gap.lastObservedAt,
      gap,
    })),
  ];
  return items
    .sort((left, right) => right.time.localeCompare(left.time))
    .slice(0, activityLimit);
}

function SideValue({
  label,
  value,
  detail,
  tone,
}: {
  label: string;
  value: string;
  detail?: string;
  tone?: "success" | "warning";
}) {
  return (
    <div className="flex min-w-0 items-start justify-between gap-4 py-3 first:pt-0 last:pb-0">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-right">
        <span
          className={
            tone === "success"
              ? "font-medium text-emerald-700 dark:text-emerald-300"
              : tone === "warning"
                ? "font-medium text-amber-700 dark:text-amber-300"
                : "font-medium"
          }
        >
          {value}
        </span>
        {detail ? (
          <span className="mt-1 block text-sm text-muted-foreground">
            {detail}
          </span>
        ) : null}
      </dd>
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
    <div className="space-y-4" aria-busy="true">
      {[0, 1].map((item) => (
        <Card key={item}>
          <CardHeader>
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-64 max-w-full" />
          </CardHeader>
          <CardContent className="space-y-3">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
