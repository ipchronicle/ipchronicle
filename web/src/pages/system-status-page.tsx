import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Activity,
  ArrowRight,
  CalendarClock,
  Check,
  CircleAlert,
  Clipboard,
  Clock3,
  KeyRound,
  Network,
  RefreshCw,
  Server,
  ShieldAlert,
  Terminal,
  TriangleAlert,
  Wifi,
  WifiOff,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";

import {
  getAgentEnrollment,
  rotateAgentEnrollmentKey,
  type AgentEnrollmentSettings,
} from "@/api/nodes";
import {
  getOverview,
  type Overview,
  type OverviewNode,
  type OverviewPublicAddress,
} from "@/api/overview";
import { getSystemSettings, getSystemStatus } from "@/api/system";
import { getAgentUpdateState, type ReleaseChannel } from "@/api/updates";
import { useAuth } from "@/auth-context";
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
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatAPIError } from "@/lib/api-error";
import { agentInstallationCommand } from "@/lib/agent-installer";
import { browserTimeZone } from "@/lib/time-zone";

const refreshIntervalMilliseconds = 5_000;
const attentionLimit = 8;
const activityLimit = 8;

type SystemStatus = Awaited<ReturnType<typeof getSystemStatus>>;

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; overview: Overview; status: SystemStatus }
  | { kind: "error" };

type AttentionItem = {
  id: string;
  priority: number;
  title: string;
  detail: string;
  to: string;
  variant: "destructive" | "warning" | "info";
};

type ActivityItem =
  | {
      kind: "probe";
      id: string;
      time: string;
      run: Overview["recentProbeRuns"][number];
    }
  | {
      kind: "address";
      id: string;
      time: string;
      event: Overview["recentAddressEvents"][number];
    };

export function SystemStatusPage() {
  const { i18n, t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(
    async (signal?: AbortSignal, initial = false, quiet = false) => {
      if (initial) setState({ kind: "loading" });
      else if (!quiet) setRefreshing(true);
      try {
        const [overview, status] = await Promise.all([
          getOverview(signal),
          getSystemStatus(signal),
        ]);
        setState({ kind: "success", overview, status });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }
        if (!quiet) setState({ kind: "error" });
      } finally {
        if (!quiet) setRefreshing(false);
      }
    },
    [],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    if (state.kind !== "success") return;
    let disposed = false;
    let inFlight = false;
    let controller: AbortController | undefined;
    const refresh = async () => {
      if (disposed || inFlight || document.visibilityState !== "visible") {
        return;
      }
      inFlight = true;
      controller = new AbortController();
      await load(controller.signal, false, true);
      inFlight = false;
      controller = undefined;
    };
    const wake = () => void refresh();
    const timer = window.setInterval(wake, refreshIntervalMilliseconds);
    document.addEventListener("visibilitychange", wake);
    window.addEventListener("focus", wake);
    return () => {
      disposed = true;
      controller?.abort();
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", wake);
      window.removeEventListener("focus", wake);
    };
  }, [load, state.kind]);

  const account =
    authState.status === "authenticated" ? authState.session.account : null;

  return (
    <div className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          <p className="text-sm font-medium text-muted-foreground uppercase">
            {t("overview.section")}
          </p>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
            {t("overview.title")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("overview.detail")}
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => void load()}
          disabled={refreshing || state.kind === "loading"}
        >
          <RefreshCw
            data-icon="inline-start"
            aria-hidden="true"
            className={refreshing ? "animate-spin" : undefined}
          />
          {t("overview.refresh")}
        </Button>
      </div>

      <div className="mt-8 space-y-3">
        {account?.usesDefaultCredentials ? (
          <Alert variant="destructive">
            <ShieldAlert aria-hidden="true" />
            <AlertTitle>{t("status.defaultCredentialsTitle")}</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>{t("status.defaultCredentialsDetail")}</span>
              <Button asChild variant="outline" size="sm">
                <Link to="/settings/account">{t("status.openAccount")}</Link>
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" && state.status.transportWarning ? (
          <Alert>
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("status.httpWarningTitle")}</AlertTitle>
            <AlertDescription>{t("status.httpWarningDetail")}</AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" && state.overview.historyOverBudget ? (
          <Alert variant="destructive">
            <CircleAlert aria-hidden="true" />
            <AlertTitle>{t("overview.historyBudget.title")}</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>{t("overview.historyBudget.detail")}</span>
              <Button asChild variant="outline" size="sm">
                <Link to="/settings/history">
                  {t("overview.historyBudget.open")}
                </Link>
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
      </div>

      <div className="mt-6" aria-live="polite">
        {state.kind === "loading" ? <OverviewSkeleton /> : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("overview.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                className="mt-3"
                variant="outline"
                size="sm"
                onClick={() => void load(undefined, true)}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("overview.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" && state.overview.nodes.length === 0 ? (
          <EmptyOverview />
        ) : null}
        {state.kind === "success" && state.overview.nodes.length > 0 ? (
          <OverviewContent
            overview={state.overview}
            locale={i18n.resolvedLanguage}
          />
        ) : null}
      </div>
    </div>
  );
}

function OverviewContent({
  overview,
  locale,
}: {
  overview: Overview;
  locale?: string;
}) {
  const { t } = useTranslation();
  const summary = useMemo(() => summarize(overview), [overview]);
  const attention = useMemo(() => buildAttention(overview, t), [overview, t]);
  const activity = useMemo(() => recentActivity(overview), [overview]);
  const nodeNames = useMemo(
    () => new Map(overview.nodes.map((node) => [node.id, node.name])),
    [overview.nodes],
  );

  return (
    <div className="space-y-4">
      <Card className="gap-0 overflow-hidden">
        <CardContent className="px-0">
          <dl className="grid sm:grid-cols-2 xl:grid-cols-6">
            <SummaryField
              icon={<Server aria-hidden="true" />}
              label={t("overview.summary.nodes")}
              value={`${summary.onlineNodes} / ${summary.totalNodes}`}
              detail={t("overview.summary.offlineNodes", {
                count: summary.offlineNodes,
              })}
              tone={summary.offlineNodes > 0 ? "warning" : "success"}
            />
            <SummaryField
              icon={<Network aria-hidden="true" />}
              label={t("overview.summary.publicAddresses")}
              value={String(summary.addresses)}
              detail={t("overview.summary.addressFamilies", {
                ipv4: summary.ipv4,
                ipv6: summary.ipv6,
              })}
              tone="info"
            />
            <SummaryField
              icon={<CircleAlert aria-hidden="true" />}
              label={t("overview.summary.unprobed")}
              value={String(summary.unprobed)}
              detail={t("overview.summary.unprobedDetail")}
              tone={summary.unprobed > 0 ? "warning" : "success"}
            />
            <SummaryField
              icon={<Activity aria-hidden="true" />}
              label={t("overview.summary.probeIssues")}
              value={String(summary.probeIssues)}
              detail={t("overview.summary.probeIssuesDetail")}
              tone={summary.probeIssues > 0 ? "destructive" : "success"}
            />
            <SummaryField
              icon={<Clock3 aria-hidden="true" />}
              label={t("overview.summary.activeTasks")}
              value={String(overview.activeTasks.length)}
              detail={t("overview.summary.activeTasksDetail")}
              tone={overview.activeTasks.length > 0 ? "info" : "neutral"}
            />
            <SummaryField
              icon={<CalendarClock aria-hidden="true" />}
              label={t("overview.summary.nextSchedule")}
              value={
                summary.nextScheduledAt === undefined
                  ? t("overview.notAvailable")
                  : formatDate(summary.nextScheduledAt, locale)
              }
              detail={t("overview.summary.nextScheduleDetail")}
              tone="neutral"
              compact
            />
          </dl>
        </CardContent>
      </Card>

      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(20rem,0.8fr)]">
        <div className="min-w-0 space-y-4">
          <AttentionCard items={attention} />
          <NodesOverviewCard nodes={overview.nodes} locale={locale} />
        </div>
        <div className="min-w-0 space-y-4">
          <ActiveTasksCard
            tasks={overview.activeTasks}
            nodeNames={nodeNames}
            locale={locale}
          />
          <RecentActivityCard
            activity={activity}
            nodeNames={nodeNames}
            locale={locale}
          />
        </div>
      </div>

      <p className="flex items-center justify-end gap-1.5 text-xs text-muted-foreground">
        <RefreshCw aria-hidden="true" className="size-3" />
        {t("overview.checkedAt", {
          value: formatDate(overview.checkedAt, locale),
        })}
      </p>
    </div>
  );
}

function SummaryField({
  icon,
  label,
  value,
  detail,
  tone,
  compact = false,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  detail: string;
  tone: "success" | "warning" | "destructive" | "info" | "neutral";
  compact?: boolean;
}) {
  const tones = {
    success:
      "bg-emerald-50 text-emerald-700 dark:bg-emerald-950/60 dark:text-emerald-300",
    warning:
      "bg-amber-50 text-amber-700 dark:bg-amber-950/60 dark:text-amber-300",
    destructive: "bg-destructive/10 text-destructive",
    info: "bg-blue-50 text-blue-700 dark:bg-blue-950/60 dark:text-blue-300",
    neutral: "bg-muted text-muted-foreground",
  };
  return (
    <div className="min-w-0 border-b p-4 last:border-b-0 sm:border-r sm:nth-[2n]:border-r-0 sm:nth-[n+5]:border-b-0 xl:border-b-0 xl:nth-[2n]:border-r xl:nth-[6n]:border-r-0">
      <dt className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
        <span
          className={`flex size-7 shrink-0 items-center justify-center rounded-md [&_svg]:size-3.5 ${tones[tone]}`}
        >
          {icon}
        </span>
        {label}
      </dt>
      <dd
        className={`mt-3 font-semibold ${compact ? "text-base leading-6" : "text-2xl"}`}
      >
        {value}
      </dd>
      <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
    </div>
  );
}

function AttentionCard({ items }: { items: AttentionItem[] }) {
  const { t } = useTranslation();
  const visible = items.slice(0, attentionLimit);
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <TriangleAlert aria-hidden="true" className="size-4" />
          {t("overview.attention.title")}
        </CardTitle>
        <CardDescription>{t("overview.attention.detail")}</CardDescription>
        <CardAction>
          <Badge variant={items.length > 0 ? "warning" : "success"}>
            {items.length}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        {visible.length === 0 ? (
          <div className="flex items-start gap-3 rounded-md border border-emerald-200 bg-emerald-50 p-4 text-emerald-800 dark:border-emerald-900 dark:bg-emerald-950/50 dark:text-emerald-200">
            <Check aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
            <div>
              <p className="text-sm font-medium">
                {t("overview.attention.healthy")}
              </p>
              <p className="mt-1 text-sm opacity-80">
                {t("overview.attention.healthyDetail")}
              </p>
            </div>
          </div>
        ) : (
          <div className="divide-y overflow-hidden rounded-md border">
            {visible.map((item) => (
              <Link
                key={item.id}
                to={item.to}
                className="group flex items-start gap-3 p-4 transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
              >
                <span
                  className={`mt-0.5 size-2 shrink-0 rounded-full ${
                    item.variant === "destructive"
                      ? "bg-destructive"
                      : item.variant === "warning"
                        ? "bg-amber-500"
                        : "bg-blue-500"
                  }`}
                />
                <span className="min-w-0 flex-1">
                  <span className="block text-sm font-medium">
                    {item.title}
                  </span>
                  <span className="mt-1 block text-sm text-muted-foreground">
                    {item.detail}
                  </span>
                </span>
                <ArrowRight
                  aria-hidden="true"
                  className="mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                />
              </Link>
            ))}
          </div>
        )}
        {items.length > attentionLimit ? (
          <p className="mt-3 text-sm text-muted-foreground">
            {t("overview.attention.more", {
              count: items.length - attentionLimit,
            })}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

function NodesOverviewCard({
  nodes,
  locale,
}: {
  nodes: OverviewNode[];
  locale?: string;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Server aria-hidden="true" className="size-4" />
          {t("overview.nodes.title")}
        </CardTitle>
        <CardDescription>{t("overview.nodes.detail")}</CardDescription>
        <CardAction>
          <Button variant="outline" size="sm" asChild>
            <Link to="/nodes">{t("overview.nodes.openAll")}</Link>
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className="overflow-hidden rounded-md border">
          <div className="hidden grid-cols-[minmax(9rem,1fr)_minmax(13rem,1.4fr)_minmax(9rem,0.8fr)_minmax(9rem,0.8fr)] gap-4 border-b bg-muted/40 px-4 py-2 text-xs font-medium text-muted-foreground md:grid">
            <span>{t("overview.nodes.node")}</span>
            <span>{t("overview.nodes.addresses")}</span>
            <span>{t("overview.nodes.latestProbe")}</span>
            <span>{t("overview.nodes.nextSchedule")}</span>
          </div>
          <div className="divide-y">
            {nodes.map((node) => (
              <Link
                key={node.id}
                to={`/nodes/${node.id}`}
                className="group grid min-w-0 gap-4 p-4 transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset md:grid-cols-[minmax(9rem,1fr)_minmax(13rem,1.4fr)_minmax(9rem,0.8fr)_minmax(9rem,0.8fr)] md:items-center"
              >
                <div className="min-w-0">
                  <p className="truncate font-medium">{node.name}</p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    <OverviewNodeStatusBadge status={node.status} />
                    {node.configurationStatus !== "current" ? (
                      <Badge
                        variant={
                          node.configurationStatus === "failed"
                            ? "destructive"
                            : "warning"
                        }
                      >
                        {t(`nodes.configuration.${node.configurationStatus}`)}
                      </Badge>
                    ) : null}
                  </div>
                </div>
                <div className="min-w-0">
                  <p className="mb-1 text-xs text-muted-foreground md:hidden">
                    {t("overview.nodes.addresses")}
                  </p>
                  {node.publicAddresses.length === 0 ? (
                    <span className="text-sm text-muted-foreground">
                      {t("overview.nodes.noAddresses")}
                    </span>
                  ) : (
                    <div className="flex min-w-0 flex-col gap-1.5">
                      {node.publicAddresses.map((address) => (
                        <span
                          key={address.id}
                          className="flex min-w-0 items-center gap-2"
                        >
                          <Badge variant="secondary">{address.family}</Badge>
                          <code className="min-w-0 break-all text-sm">
                            {address.address}
                          </code>
                        </span>
                      ))}
                    </div>
                  )}
                </div>
                <div className="min-w-0">
                  <p className="mb-1 text-xs text-muted-foreground md:hidden">
                    {t("overview.nodes.latestProbe")}
                  </p>
                  {node.latestProbeRun === undefined ? (
                    <span className="text-sm text-muted-foreground">
                      {t("overview.nodes.notProbed")}
                    </span>
                  ) : (
                    <div>
                      <ProbeStatusBadge status={node.latestProbeRun.status} />
                      <p className="mt-1 text-xs text-muted-foreground">
                        {formatDate(node.latestProbeRun.startedAt, locale)}
                      </p>
                    </div>
                  )}
                </div>
                <div className="flex min-w-0 items-center justify-between gap-3">
                  <div>
                    <p className="mb-1 text-xs text-muted-foreground md:hidden">
                      {t("overview.nodes.nextSchedule")}
                    </p>
                    <p className="text-sm">
                      {node.nextScheduledAt === undefined
                        ? t("overview.notAvailable")
                        : formatDate(node.nextScheduledAt, locale)}
                    </p>
                  </div>
                  <ArrowRight
                    aria-hidden="true"
                    className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5 md:hidden"
                  />
                </div>
              </Link>
            ))}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function ActiveTasksCard({
  tasks,
  nodeNames,
  locale,
}: {
  tasks: Overview["activeTasks"];
  nodeNames: Map<string, string>;
  locale?: string;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Clock3 aria-hidden="true" className="size-4" />
          {t("overview.tasks.title")}
        </CardTitle>
        <CardDescription>{t("overview.tasks.detail")}</CardDescription>
        <CardAction>
          <Badge variant={tasks.length > 0 ? "info" : "secondary"}>
            {tasks.length}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        {tasks.length === 0 ? (
          <p className="py-5 text-center text-sm text-muted-foreground">
            {t("overview.tasks.empty")}
          </p>
        ) : (
          <div className="divide-y overflow-hidden rounded-md border">
            {tasks.map((task) => (
              <Link
                key={task.id}
                to={
                  task.runId === undefined
                    ? `/nodes/${task.nodeId}/probe`
                    : `/probe-runs/${task.runId}`
                }
                className="block p-3 transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
              >
                <div className="flex items-center justify-between gap-3">
                  <p className="truncate text-sm font-medium">
                    {nodeNames.get(task.nodeId) ?? task.nodeId}
                  </p>
                  <Badge variant="info">
                    {t(`overview.tasks.status.${task.status}`)}
                  </Badge>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t(`overview.tasks.kind.${task.kind}`)} ·{" "}
                  {formatDate(task.createdAt, locale)}
                </p>
              </Link>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function RecentActivityCard({
  activity,
  nodeNames,
  locale,
}: {
  activity: ActivityItem[];
  nodeNames: Map<string, string>;
  locale?: string;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Activity aria-hidden="true" className="size-4" />
          {t("overview.activity.title")}
        </CardTitle>
        <CardDescription>{t("overview.activity.detail")}</CardDescription>
      </CardHeader>
      <CardContent>
        {activity.length === 0 ? (
          <p className="py-5 text-center text-sm text-muted-foreground">
            {t("overview.activity.empty")}
          </p>
        ) : (
          <div className="divide-y overflow-hidden rounded-md border">
            {activity.map((item) => {
              const nodeID =
                item.kind === "probe" ? item.run.nodeId : item.event.nodeId;
              const to =
                item.kind === "probe"
                  ? `/probe-runs/${item.run.id}`
                  : `/history?tab=addresses&nodeId=${item.event.nodeId}&egressId=${item.event.publicAddressId}`;
              return (
                <Link
                  key={`${item.kind}-${item.id}`}
                  to={to}
                  className="block p-3 transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset"
                >
                  <div className="flex items-start gap-3">
                    <span
                      className={`mt-1.5 size-2 shrink-0 rounded-full ${
                        item.kind === "probe"
                          ? item.run.status === "failed"
                            ? "bg-destructive"
                            : item.run.status === "partial"
                              ? "bg-amber-500"
                              : "bg-emerald-500"
                          : item.event.kind === "check-failure"
                            ? "bg-destructive"
                            : "bg-blue-500"
                      }`}
                    />
                    <div className="min-w-0">
                      <p className="text-sm font-medium">
                        {item.kind === "probe"
                          ? t("overview.activity.probe", {
                              node: nodeNames.get(nodeID) ?? nodeID,
                              status: t(`probe.state.${item.run.status}`),
                            })
                          : t("overview.activity.address", {
                              node: nodeNames.get(nodeID) ?? nodeID,
                              event: t(
                                `network.addressHistory.kind.${item.event.kind}`,
                              ),
                            })}
                      </p>
                      {item.kind === "address" &&
                      item.event.publicAddress !== undefined ? (
                        <code className="mt-1 block break-all text-xs text-muted-foreground">
                          {item.event.publicAddress}
                        </code>
                      ) : null}
                      <p className="mt-1 text-xs text-muted-foreground">
                        {formatDate(item.time, locale)}
                      </p>
                    </div>
                  </div>
                </Link>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function OverviewNodeStatusBadge({
  status,
}: {
  status: OverviewNode["status"];
}) {
  const { t } = useTranslation();
  return (
    <Badge
      variant={
        status === "online"
          ? "success"
          : status === "offline"
            ? "warning"
            : "destructive"
      }
    >
      {status === "online" ? (
        <Wifi aria-hidden="true" />
      ) : (
        <WifiOff aria-hidden="true" />
      )}
      {t(`nodes.status.${status}`)}
    </Badge>
  );
}

function ProbeStatusBadge({
  status,
}: {
  status: Overview["recentProbeRuns"][number]["status"];
}) {
  const { t } = useTranslation();
  return (
    <Badge
      variant={
        status === "succeeded"
          ? "success"
          : status === "partial" || status === "running"
            ? "warning"
            : "destructive"
      }
    >
      {t(`probe.state.${status}`)}
    </Badge>
  );
}

type EmptySetupState =
  | { kind: "loading" }
  | {
      kind: "success";
      enrollment: AgentEnrollmentSettings;
      centerURL: string;
      channel: ReleaseChannel;
    }
  | { kind: "error" };

function EmptyOverview() {
  const { t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<EmptySetupState>({ kind: "loading" });
  const [working, setWorking] = useState(false);
  const [feedback, setFeedback] = useState<string>();
  const [copied, setCopied] = useState(false);
  const copyTimeout = useRef<number | undefined>(undefined);
  const timezone = useMemo(browserTimeZone, []);
  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  const load = useCallback(async (signal?: AbortSignal) => {
    setState({ kind: "loading" });
    try {
      const [enrollment, settings, updates] = await Promise.all([
        getAgentEnrollment(signal),
        getSystemSettings(signal),
        getAgentUpdateState(signal),
      ]);
      setState({
        kind: "success",
        enrollment,
        centerURL: settings.effectiveOrigin,
        channel: updates.channel,
      });
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setState({ kind: "error" });
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => {
      controller.abort();
      if (copyTimeout.current !== undefined) {
        window.clearTimeout(copyTimeout.current);
      }
    };
  }, [load]);

  const command =
    state.kind === "success" && state.enrollment.registrationKey !== undefined
      ? agentInstallationCommand(
          state.centerURL,
          state.enrollment.registrationKey,
          state.channel,
        )
      : undefined;

  async function generate() {
    if (state.kind !== "success") return;
    setWorking(true);
    setFeedback(undefined);
    try {
      const enrollment = await rotateAgentEnrollmentKey(timezone, csrfToken);
      setState({ ...state, enrollment });
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setWorking(false);
    }
  }

  async function copy() {
    if (command === undefined) return;
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      if (copyTimeout.current !== undefined) {
        window.clearTimeout(copyTimeout.current);
      }
      copyTimeout.current = window.setTimeout(() => setCopied(false), 2_000);
    } catch {
      setFeedback(t("errors.actionFailed"));
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Terminal aria-hidden="true" className="size-4" />
          {t("overview.empty.title")}
        </CardTitle>
        <CardDescription>{t("overview.empty.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">{t("overview.empty.badge")}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-5">
        {state.kind === "loading" ? (
          <div className="space-y-3" aria-busy="true">
            <Skeleton className="h-10 w-48" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("overview.empty.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                className="mt-3"
                variant="outline"
                size="sm"
                onClick={() => void load()}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("overview.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <>
            {feedback !== undefined ? (
              <Alert variant="destructive">
                <TriangleAlert aria-hidden="true" />
                <AlertDescription>{feedback}</AlertDescription>
              </Alert>
            ) : null}
            {command === undefined ? (
              <div className="flex flex-col items-start justify-between gap-4 rounded-md border p-4 sm:flex-row sm:items-center">
                <div>
                  <p className="text-sm font-medium">
                    {t("overview.empty.generateTitle")}
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {t("overview.empty.generateDetail")}
                  </p>
                </div>
                <Button onClick={() => void generate()} disabled={working}>
                  <KeyRound data-icon="inline-start" aria-hidden="true" />
                  {t("overview.empty.generate")}
                </Button>
              </div>
            ) : (
              <div>
                <div className="mb-2 flex flex-wrap items-center justify-between gap-3">
                  <div>
                    <p className="text-sm font-medium">
                      {t("overview.empty.command")}
                    </p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {t("overview.empty.commandDetail")}
                    </p>
                  </div>
                  <Tooltip open={copied}>
                    <TooltipTrigger asChild>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => void copy()}
                      >
                        <Clipboard
                          data-icon="inline-start"
                          aria-hidden="true"
                        />
                        {t("overview.empty.copy")}
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent side="top" sideOffset={6}>
                      {t("overview.empty.copied")}
                    </TooltipContent>
                  </Tooltip>
                </div>
                <pre className="overflow-x-auto rounded-md bg-muted p-4 text-sm leading-5">
                  <code>{command}</code>
                </pre>
                {!state.enrollment.enabled ? (
                  <Alert className="mt-4">
                    <TriangleAlert aria-hidden="true" />
                    <AlertTitle>
                      {t("overview.empty.enrollmentDisabled")}
                    </AlertTitle>
                    <AlertDescription>
                      {t("overview.empty.enrollmentDisabledDetail")}
                    </AlertDescription>
                  </Alert>
                ) : null}
              </div>
            )}
            <Button variant="outline" asChild>
              <Link to="/nodes">
                {t("overview.empty.openNodes")}
                <ArrowRight data-icon="inline-end" aria-hidden="true" />
              </Link>
            </Button>
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}

function summarize(overview: Overview) {
  const addresses = uniqueAddresses(overview.nodes);
  const nextScheduledAt = overview.nodes
    .map((node) => node.nextScheduledAt)
    .filter((value): value is string => value !== undefined)
    .sort()[0];
  return {
    totalNodes: overview.nodes.length,
    onlineNodes: overview.nodes.filter((node) => node.status === "online")
      .length,
    offlineNodes: overview.nodes.filter((node) => node.status === "offline")
      .length,
    addresses: addresses.length,
    ipv4: addresses.filter((address) => address.family === "ipv4").length,
    ipv6: addresses.filter((address) => address.family === "ipv6").length,
    unprobed: addresses.filter(
      (address) => address.latestSnapshotId === undefined,
    ).length,
    probeIssues: addresses.filter(
      (address) =>
        address.latestProbeOutcome === "failed" ||
        address.formatStatus === "mismatch",
    ).length,
    nextScheduledAt,
  };
}

function buildAttention(
  overview: Overview,
  t: ReturnType<typeof useTranslation>["t"],
) {
  const items: AttentionItem[] = [];
  for (const node of overview.nodes) {
    if (node.configurationStatus !== "current") {
      items.push({
        id: `configuration-${node.id}`,
        priority: node.configurationStatus === "failed" ? 10 : 30,
        title: t("overview.attention.configurationTitle", {
          node: node.name,
        }),
        detail: t("overview.attention.configurationDetail", {
          status: t(`nodes.configuration.${node.configurationStatus}`),
          applied: node.appliedConfigurationRevision,
          desired: node.desiredConfigurationRevision,
        }),
        to: `/nodes/${node.id}`,
        variant:
          node.configurationStatus === "failed" ? "destructive" : "warning",
      });
    }
    if (node.status === "offline") {
      items.push({
        id: `offline-${node.id}`,
        priority: 20,
        title: t("overview.attention.offlineTitle", { node: node.name }),
        detail: t("overview.attention.offlineDetail", {
          time:
            node.lastSeenAt === undefined
              ? t("overview.notAvailable")
              : formatDate(node.lastSeenAt),
        }),
        to: `/nodes/${node.id}`,
        variant: "warning",
      });
    }
    if (node.pausedLowMemory) {
      items.push({
        id: `memory-${node.id}`,
        priority: 15,
        title: t("overview.attention.memoryTitle", { node: node.name }),
        detail: t("overview.attention.memoryDetail"),
        to: `/nodes/${node.id}/probe`,
        variant: "destructive",
      });
    }
  }
  for (const { address, node } of uniqueAddressContexts(overview.nodes)) {
    const addressName = address.address;
    if (address.latestProbeOutcome === "failed") {
      items.push({
        id: `probe-${address.id}`,
        priority: 5,
        title: t("overview.attention.probeTitle", { address: addressName }),
        detail: t("overview.attention.probeDetail", { node: node.name }),
        to:
          address.latestProbeRunId === undefined
            ? `/nodes/${node.id}/probe`
            : `/probe-runs/${address.latestProbeRunId}`,
        variant: "destructive",
      });
    }
    if (address.formatStatus === "mismatch") {
      items.push({
        id: `format-${address.id}`,
        priority: 8,
        title: t("overview.attention.formatTitle", { address: addressName }),
        detail: t("overview.attention.formatDetail"),
        to:
          address.latestSnapshotId === undefined
            ? `/history?nodeId=${node.id}&egressId=${address.id}`
            : `/probe-snapshots/${address.latestSnapshotId}`,
        variant: "destructive",
      });
    }
    if (address.latestSnapshotId === undefined) {
      items.push({
        id: `unprobed-${address.id}`,
        priority: 40,
        title: t("overview.attention.unprobedTitle", {
          address: addressName,
        }),
        detail: t("overview.attention.unprobedDetail", { node: node.name }),
        to: `/nodes/${node.id}/network`,
        variant: "info",
      });
    }
    if (address.likelyNat) {
      items.push({
        id: `nat-${address.id}`,
        priority: 50,
        title: t("overview.attention.natTitle", { address: addressName }),
        detail: t("overview.attention.natDetail", { node: node.name }),
        to: `/nodes/${node.id}/network`,
        variant: "warning",
      });
    }
  }
  return items.sort((left, right) => left.priority - right.priority);
}

function uniqueAddresses(nodes: OverviewNode[]) {
  return uniqueAddressContexts(nodes).map((item) => item.address);
}

function uniqueAddressContexts(nodes: OverviewNode[]) {
  const values = new Map<
    string,
    { address: OverviewPublicAddress; node: OverviewNode }
  >();
  for (const node of nodes) {
    for (const address of node.publicAddresses) {
      if (!values.has(address.id)) values.set(address.id, { address, node });
    }
  }
  return [...values.values()];
}

function recentActivity(overview: Overview) {
  const items: ActivityItem[] = [
    ...overview.recentProbeRuns.map((run): ActivityItem => ({
      kind: "probe",
      id: run.id,
      time: run.completedAt ?? run.startedAt,
      run,
    })),
    ...overview.recentAddressEvents.map((event): ActivityItem => ({
      kind: "address",
      id: event.id,
      time: event.observedAt,
      event,
    })),
  ];
  return items
    .sort((left, right) => right.time.localeCompare(left.time))
    .slice(0, activityLimit);
}

function OverviewSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      <Card>
        <CardContent className="grid gap-4 pt-6 sm:grid-cols-2 xl:grid-cols-6">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="h-24 w-full" />
          ))}
        </CardContent>
      </Card>
      <div className="grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(20rem,0.8fr)]">
        <div className="space-y-4">
          <Skeleton className="h-64 w-full" />
          <Skeleton className="h-80 w-full" />
        </div>
        <div className="space-y-4">
          <Skeleton className="h-52 w-full" />
          <Skeleton className="h-72 w-full" />
        </div>
      </div>
    </div>
  );
}

function formatDate(value: string, locale?: string) {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
