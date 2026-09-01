import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  Activity,
  CalendarClock,
  Check,
  Clock3,
  Eye,
  LoaderCircle,
  MemoryStick,
  Play,
  RefreshCw,
  Save,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";

import type { Node } from "@/api/nodes";
import {
  getNodeProbe,
  previewProbeSchedule,
  updateNodeProbeSettings,
  type NodeProbeState,
  type ProbeRunSummary,
  type ProbeTask,
} from "@/api/probes";
import { APIError } from "@/api/errors";
import { useAuth } from "@/auth-context";
import { CompleteProbeDialog } from "@/components/complete-probe-dialog";
import { useNodeDetail } from "@/components/node-detail-layout";
import { TimeZoneCombobox } from "@/components/time-zone-combobox";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { formatAPIError } from "@/lib/api-error";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; probe: NodeProbeState }
  | { kind: "error" };

type SchedulePreviewState =
  | { kind: "loading" }
  | { kind: "success"; nextScheduledAt: string }
  | { kind: "disabled" | "invalid" | "error" };

const activeTaskStatuses = new Set(["pending", "acknowledged", "running"]);

export function NodeProbePage() {
  const { nodeId = "" } = useParams();
  const { node } = useNodeDetail();
  const { t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);
  const [feedback, setFeedback] = useState<
    { kind: "success" | "error"; message: string } | undefined
  >();

  const load = useCallback(
    async (signal?: AbortSignal, initial = false, quiet = false) => {
      if (initial) setState({ kind: "loading" });
      else if (!quiet) setRefreshing(true);
      try {
        setState({
          kind: "success",
          probe: await getNodeProbe(nodeId, signal),
        });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        if (!quiet) setState({ kind: "error" });
      } finally {
        if (!quiet) setRefreshing(false);
      }
    },
    [nodeId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    return () => controller.abort();
  }, [load]);

  const shouldPoll =
    state.kind === "success" &&
    ((state.probe.task !== undefined &&
      activeTaskStatuses.has(state.probe.task.status)) ||
      state.probe.recentRuns.some((run) => run.status === "running"));
  useEffect(() => {
    if (!shouldPoll) return;
    const timer = window.setInterval(
      () => void load(undefined, false, true),
      5_000,
    );
    return () => window.clearInterval(timer);
  }, [load, shouldPoll]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  function probeCreated(task: ProbeTask) {
    setState((current) =>
      current.kind === "success"
        ? { ...current, probe: { ...current.probe, task } }
        : current,
    );
    setFeedback({ kind: "success", message: t("probe.task.created") });
  }

  function replaceProbe(probe: NodeProbeState) {
    setState((current) =>
      current.kind === "success" ? { ...current, probe } : current,
    );
  }

  return (
    <div className="space-y-4" aria-live="polite">
      <div className="flex flex-wrap justify-end gap-2">
        <Button
          variant="outline"
          disabled={refreshing || state.kind === "loading"}
          onClick={() => void load()}
        >
          <RefreshCw
            data-icon="inline-start"
            aria-hidden="true"
            className={refreshing ? "animate-spin" : undefined}
          />
          {t("probe.refresh")}
        </Button>
        {state.kind === "success" ? (
          <CompleteProbeDialog
            nodeId={nodeId}
            csrfToken={csrfToken}
            onCreated={probeCreated}
          >
            <Button disabled={immediateProbeUnavailable(node, state.probe)}>
              <Play data-icon="inline-start" aria-hidden="true" />
              {t("probe.runNow")}
            </Button>
          </CompleteProbeDialog>
        ) : null}
      </div>

      {state.kind === "loading" ? <ProbeSkeleton /> : null}
      {state.kind === "error" ? (
        <Alert variant="destructive">
          <TriangleAlert aria-hidden="true" />
          <AlertTitle>{t("probe.loadFailed")}</AlertTitle>
          <AlertDescription>
            <Button
              variant="outline"
              size="sm"
              className="mt-3"
              onClick={() => void load(undefined, true)}
            >
              <RefreshCw data-icon="inline-start" aria-hidden="true" />
              {t("probe.retry")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {feedback ? (
        <Alert variant={feedback.kind === "error" ? "destructive" : "default"}>
          {feedback.kind === "error" ? (
            <TriangleAlert aria-hidden="true" />
          ) : (
            <Check aria-hidden="true" />
          )}
          <AlertDescription>{feedback.message}</AlertDescription>
        </Alert>
      ) : null}

      {state.kind === "success" ? (
        <>
          {state.probe.pausedLowMemory ? (
            <Alert variant="destructive">
              <MemoryStick aria-hidden="true" />
              <AlertTitle>{t("probe.lowMemory.title")}</AlertTitle>
              <AlertDescription>{t("probe.lowMemory.detail")}</AlertDescription>
            </Alert>
          ) : null}
          {immediateProbeReason(node, state.probe) ? (
            <Alert>
              <Clock3 aria-hidden="true" />
              <AlertDescription>
                {t(
                  `probe.unavailable.${immediateProbeReason(node, state.probe)}`,
                )}
              </AlertDescription>
            </Alert>
          ) : null}

          <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,2.5fr)_minmax(290px,.8fr)]">
            <div className="space-y-4">
              <ProbeStatusCard probe={state.probe} />
              <RecentRunsCard runs={state.probe.recentRuns} />
            </div>
            <aside className="space-y-4">
              <ProbeScheduleCard
                key={`${state.probe.schedule.enabled}:${state.probe.schedule.cron}:${state.probe.schedule.timezone}`}
                nodeId={nodeId}
                probe={state.probe}
                csrfToken={csrfToken}
                onChange={replaceProbe}
              />
              <TaskCard task={state.probe.task} />
            </aside>
          </div>
        </>
      ) : null}
    </div>
  );
}

function ProbeStatusCard({ probe }: { probe: NodeProbeState }) {
  const { t, i18n } = useTranslation();
  const latestRun = probe.recentRuns[0];
  const task = probe.task;
  const status = probe.agentStatus;
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Activity aria-hidden="true" className="size-4" />
          {t("probe.status.title")}
        </CardTitle>
        <CardDescription>{t("probe.status.detail")}</CardDescription>
        <CardAction>
          <Badge variant={status?.activeRunId ? "info" : "outline"}>
            {status?.activeRunId
              ? t("probe.status.running")
              : t("probe.status.idle")}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className="grid gap-3 md:grid-cols-3">
          <SummaryItem
            label={t("probe.status.latestComplete")}
            value={
              latestRun
                ? t(`probe.state.${latestRun.status}`)
                : t("probe.runs.emptyShort")
            }
            detail={formatTime(
              latestRun?.startedAt,
              i18n.resolvedLanguage,
              t("probe.notAvailable"),
            )}
            tone={latestRun?.status === "succeeded" ? "success" : "neutral"}
          />
          <SummaryItem
            label={t("probe.status.currentTask")}
            value={
              task
                ? t(`probe.state.${task.status}`)
                : t("probe.task.emptyShort")
            }
            detail={
              task
                ? formatTime(
                    task.createdAt,
                    i18n.resolvedLanguage,
                    t("probe.notAvailable"),
                  )
                : t("probe.status.noActiveTask")
            }
            tone={
              task && activeTaskStatuses.has(task.status)
                ? "warning"
                : "neutral"
            }
          />
          <SummaryItem
            label={t("probe.status.next")}
            value={formatTime(
              status?.nextScheduledAt,
              i18n.resolvedLanguage,
              t("probe.notAvailable"),
            )}
            detail={
              probe.schedule.enabled
                ? t("probe.status.scheduleEnabled")
                : t("probe.status.scheduleDisabled")
            }
          />
        </div>

        <dl className="mt-4 grid gap-4 border-t pt-4 sm:grid-cols-3">
          <StatusValue
            label={t("probe.status.last")}
            value={formatTime(
              status?.lastOccurrenceAt,
              i18n.resolvedLanguage,
              t("probe.notAvailable"),
            )}
          />
          <StatusValue
            label={t("probe.status.trigger")}
            value={
              status?.lastOccurrenceTrigger
                ? t(`probe.trigger.${status.lastOccurrenceTrigger}`)
                : t("probe.notAvailable")
            }
          />
          <StatusValue
            label={t("probe.status.memory")}
            value={
              probe.physicalMemoryBytes === undefined
                ? t("probe.notAvailable")
                : formatBytes(probe.physicalMemoryBytes, i18n.resolvedLanguage)
            }
          />
        </dl>
        {status?.lastSkipReason ? (
          <p className="mt-4 border-t pt-4 text-sm text-muted-foreground">
            {t("probe.status.lastSkipped", {
              reason: t(`probe.skip.${status.lastSkipReason}`),
            })}
          </p>
        ) : null}
        {status?.historyResetAt ? (
          <p className="mt-4 border-t pt-4 text-sm text-muted-foreground">
            {t("probe.status.resetApplied", {
              value: formatTime(
                status.historyResetAt,
                i18n.resolvedLanguage,
                t("probe.notAvailable"),
              ),
              count:
                (status.historyResetDiscardedAddressItems ?? 0) +
                (status.historyResetDiscardedProbeItems ?? 0),
            })}
          </p>
        ) : null}
      </CardContent>
    </Card>
  );
}

function SummaryItem({
  label,
  value,
  detail,
  tone = "neutral",
}: {
  label: string;
  value: string;
  detail: string;
  tone?: "neutral" | "success" | "warning";
}) {
  const className =
    tone === "success"
      ? "bg-emerald-50 dark:bg-emerald-950/40"
      : tone === "warning"
        ? "bg-amber-50 dark:bg-amber-950/40"
        : "bg-muted/60";
  const valueClassName =
    tone === "success"
      ? "text-emerald-700 dark:text-emerald-300"
      : tone === "warning"
        ? "text-amber-700 dark:text-amber-300"
        : "text-foreground";
  return (
    <div className={`rounded-lg p-4 ${className}`}>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={`mt-1 text-base font-semibold ${valueClassName}`}>
        {value}
      </p>
      <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
    </div>
  );
}

function StatusValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-words font-medium">{value}</dd>
    </div>
  );
}

function TaskCard({ task }: { task?: ProbeTask }) {
  const { t, i18n } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Clock3 aria-hidden="true" className="size-4" />
          {t("probe.task.title")}
        </CardTitle>
        <CardDescription>{t("probe.task.detail")}</CardDescription>
        {task ? (
          <CardAction>
            <ProbeStatusBadge status={task.status} />
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent>
        {!task ? (
          <p className="py-5 text-center text-sm text-muted-foreground">
            {t("probe.task.empty")}
          </p>
        ) : (
          <div className="space-y-4">
            {task.offline && activeTaskStatuses.has(task.status) ? (
              <Alert variant="destructive">
                <TriangleAlert aria-hidden="true" />
                <AlertDescription>{t("probe.task.offline")}</AlertDescription>
              </Alert>
            ) : null}
            <dl className="space-y-3">
              <CompactValue
                label={t("probe.task.createdAt")}
                value={formatTime(
                  task.createdAt,
                  i18n.resolvedLanguage,
                  t("probe.notAvailable"),
                )}
              />
              <CompactValue
                label={t("probe.task.receivedAt")}
                value={formatTime(
                  task.acknowledgedAt,
                  i18n.resolvedLanguage,
                  t("probe.task.waiting"),
                )}
              />
              <CompactValue
                label={t("probe.task.completedAt")}
                value={formatTime(
                  task.completedAt,
                  i18n.resolvedLanguage,
                  t("probe.notAvailable"),
                )}
              />
            </dl>
            {task.runId ? (
              <Button variant="outline" size="sm" asChild>
                <Link to={`/probe-runs/${task.runId}`}>
                  <Eye data-icon="inline-start" aria-hidden="true" />
                  {t("probe.task.openRun")}
                </Link>
              </Button>
            ) : null}
            {task.rejectionReason ? (
              <p className="text-sm text-destructive">
                {t(`probe.skip.${task.rejectionReason}`)}
              </p>
            ) : null}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function CompactValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-4 border-t pt-3 first:border-t-0 first:pt-0">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="text-right text-sm font-medium">{value}</dd>
    </div>
  );
}

function RecentRunsCard({ runs }: { runs: ProbeRunSummary[] }) {
  const { t, i18n } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("probe.runs.title")}</CardTitle>
        <CardDescription>{t("probe.runs.detail")}</CardDescription>
        <CardAction>
          <Badge variant="info">{runs.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        {runs.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("probe.runs.empty")}
          </p>
        ) : (
          <div className="overflow-hidden rounded-lg border">
            {runs.map((run) => (
              <div
                key={run.id}
                className="grid gap-4 border-t p-4 first:border-t-0 md:grid-cols-[minmax(180px,1.2fr)_minmax(110px,.55fr)_minmax(120px,.65fr)_auto] md:items-center"
              >
                <div className="min-w-0">
                  <p className="font-medium">
                    {formatTime(
                      run.startedAt,
                      i18n.resolvedLanguage,
                      t("probe.notAvailable"),
                    )}
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {t(`probe.trigger.${run.trigger}`)}
                  </p>
                </div>
                <div>
                  <p className="mb-1 text-xs text-muted-foreground">
                    {t("probe.runs.status")}
                  </p>
                  <ProbeStatusBadge status={run.status} />
                </div>
                <div>
                  <p className="mb-1 text-xs text-muted-foreground">
                    {t("probe.runs.completed")}
                  </p>
                  <p className="text-sm font-medium">
                    {t("probe.runs.progress", {
                      completed: run.completedExecutions,
                      total: run.expectedExecutions,
                    })}
                  </p>
                </div>
                <Button variant="outline" size="sm" asChild>
                  <Link to={`/probe-runs/${run.id}`}>
                    <Eye data-icon="inline-start" aria-hidden="true" />
                    {t("probe.runs.open")}
                  </Link>
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function ProbeScheduleCard({
  nodeId,
  probe,
  csrfToken,
  onChange,
}: {
  nodeId: string;
  probe: NodeProbeState;
  csrfToken: string;
  onChange: (value: NodeProbeState) => void;
}) {
  const { t, i18n } = useTranslation();
  const [schedule, setSchedule] = useState({ ...probe.schedule });
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<string>();
  const [preview, setPreview] = useState<SchedulePreviewState>({
    kind: schedule.enabled ? "loading" : "disabled",
  });

  useEffect(() => {
    if (!schedule.enabled) {
      setPreview({ kind: "disabled" });
      return;
    }

    const controller = new AbortController();
    let active = true;
    setPreview({ kind: "loading" });
    const timeout = window.setTimeout(() => {
      previewProbeSchedule(schedule.cron, schedule.timezone, controller.signal)
        .then((result) => {
          if (active) {
            setPreview({
              kind: "success",
              nextScheduledAt: result.nextScheduledAt,
            });
          }
        })
        .catch((error: unknown) => {
          if (
            !active ||
            (error instanceof DOMException && error.name === "AbortError")
          ) {
            return;
          }
          setPreview({
            kind:
              error instanceof APIError && error.status === 400
                ? "invalid"
                : "error",
          });
        });
    }, 250);

    return () => {
      active = false;
      window.clearTimeout(timeout);
      controller.abort();
    };
  }, [schedule.cron, schedule.enabled, schedule.timezone]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setFeedback(undefined);
    try {
      onChange(
        await updateNodeProbeSettings(
          nodeId,
          {
            schedule,
            lowMemoryOverride: probe.lowMemoryOverride,
            probeOnNewAddress: probe.probeOnNewAddress,
          },
          csrfToken,
        ),
      );
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CalendarClock aria-hidden="true" className="size-4" />
          {t("probe.schedule.title")}
        </CardTitle>
        <CardDescription>{t("probe.schedule.detail")}</CardDescription>
        <CardAction>
          <Switch
            checked={schedule.enabled}
            disabled={saving}
            onCheckedChange={(enabled) =>
              setSchedule((current) => ({ ...current, enabled }))
            }
            aria-label={t("probe.settings.scheduleEnabled")}
          />
        </CardAction>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={(event) => void submit(event)}>
          {feedback ? (
            <Alert variant="destructive">
              <TriangleAlert aria-hidden="true" />
              <AlertDescription>{feedback}</AlertDescription>
            </Alert>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="probe-cron">{t("probe.settings.cron")}</Label>
            <Input
              id="probe-cron"
              value={schedule.cron}
              disabled={saving}
              autoComplete="off"
              onChange={(event) =>
                setSchedule((current) => ({
                  ...current,
                  cron: event.target.value,
                }))
              }
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="probe-timezone">
              {t("probe.settings.timezone")}
            </Label>
            <TimeZoneCombobox
              id="probe-timezone"
              value={schedule.timezone}
              disabled={saving}
              onValueChange={(timezone) =>
                setSchedule((current) => ({
                  ...current,
                  timezone,
                }))
              }
            />
          </div>
          <div
            className="flex min-h-16 items-center gap-3 rounded-md border bg-muted/30 px-3 py-2"
            aria-live="polite"
          >
            {preview.kind === "loading" ? (
              <LoaderCircle
                aria-hidden="true"
                className="size-4 shrink-0 animate-spin text-muted-foreground"
              />
            ) : (
              <Clock3
                aria-hidden="true"
                className={`size-4 shrink-0 ${preview.kind === "invalid" || preview.kind === "error" ? "text-destructive" : "text-muted-foreground"}`}
              />
            )}
            <div className="min-w-0">
              <p className="text-sm font-medium">{t("probe.schedule.next")}</p>
              <p
                className={`mt-0.5 text-sm ${preview.kind === "invalid" || preview.kind === "error" ? "text-destructive" : "text-muted-foreground"}`}
              >
                {preview.kind === "success"
                  ? formatScheduleTime(
                      preview.nextScheduledAt,
                      schedule.timezone,
                      i18n.resolvedLanguage,
                    )
                  : t(`probe.schedule.preview.${preview.kind}`)}
              </p>
            </div>
          </div>
          <Button type="submit" size="sm" disabled={saving}>
            {saving ? (
              <LoaderCircle
                data-icon="inline-start"
                aria-hidden="true"
                className="animate-spin"
              />
            ) : (
              <Save data-icon="inline-start" aria-hidden="true" />
            )}
            {t("probe.settings.save")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

export function ProbeStatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  const variant =
    status === "failed" ||
    status === "rejected" ||
    status === "expired" ||
    status === "interrupted"
      ? "destructive"
      : status === "succeeded"
        ? "success"
        : status === "running"
          ? "info"
          : status === "partial" || status === "skipped" || status === "pending"
            ? "warning"
            : "outline";
  return <Badge variant={variant}>{t(`probe.state.${status}`)}</Badge>;
}

function immediateProbeUnavailable(node: Node, probe: NodeProbeState) {
  return immediateProbeReason(node, probe) !== undefined;
}

function immediateProbeReason(node: Node, probe: NodeProbeState) {
  if (node.status !== "online")
    return node.status === "disabled" ? "disabled" : "offline";
  if (probe.pausedLowMemory) return "lowMemory";
  if (
    probe.agentStatus?.activeRunId ||
    probe.recentRuns.some((run) => run.status === "running")
  )
    return "running";
  if (probe.task && activeTaskStatuses.has(probe.task.status)) return "task";
  return undefined;
}

export function formatTime(
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

function formatScheduleTime(
  value: string,
  timezone: string,
  locale: string | undefined,
) {
  return new Intl.DateTimeFormat(locale, {
    timeZone: timezone,
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    timeZoneName: "short",
  }).format(new Date(value));
}

function formatBytes(value: number, locale: string | undefined) {
  return new Intl.NumberFormat(locale, {
    style: "unit",
    unit: "megabyte",
    maximumFractionDigits: 0,
  }).format(value / (1024 * 1024));
}

function ProbeSkeleton() {
  return (
    <div
      className="grid items-start gap-4 xl:grid-cols-[minmax(0,2.5fr)_minmax(290px,.8fr)]"
      aria-busy="true"
    >
      <div className="space-y-4">
        {[0, 1].map((item) => (
          <Card key={item}>
            <CardHeader>
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-64 max-w-full" />
            </CardHeader>
            <CardContent>
              <Skeleton className="h-28 w-full" />
            </CardContent>
          </Card>
        ))}
      </div>
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-32" />
        </CardHeader>
        <CardContent>
          <Skeleton className="h-40 w-full" />
        </CardContent>
      </Card>
    </div>
  );
}
