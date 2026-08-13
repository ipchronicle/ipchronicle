import { useCallback, useEffect, useState } from "react";
import type { FormEvent } from "react";
import {
  Activity,
  Check,
  Clock3,
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
  createCompleteProbeTask,
  getNodeProbe,
  updateNodeProbeSettings,
  type NodeProbeSettingsUpdate,
  type NodeProbeState,
  type ProbeRunSummary,
  type ProbeTask,
} from "@/api/probes";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { formatAPIError } from "@/lib/api-error";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; probe: NodeProbeState }
  | { kind: "error" };

const activeTaskStatuses = new Set(["pending", "acknowledged", "running"]);

export function NodeProbePage() {
  const { nodeId = "" } = useParams();
  const { node } = useNodeDetail();
  const { t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);
  const [starting, setStarting] = useState(false);
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
        setState({ kind: "error" });
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
      5000,
    );
    return () => window.clearInterval(timer);
  }, [load, shouldPoll]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function startProbe() {
    setStarting(true);
    setFeedback(undefined);
    try {
      const task = await createCompleteProbeTask(nodeId, csrfToken);
      setState((current) =>
        current.kind === "success"
          ? { ...current, probe: { ...current.probe, task } }
          : current,
      );
      setFeedback({ kind: "success", message: t("probe.task.created") });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setStarting(false);
    }
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
          <Button
            disabled={starting || immediateProbeUnavailable(node, state.probe)}
            onClick={() => void startProbe()}
          >
            {starting ? (
              <LoaderCircle
                data-icon="inline-start"
                aria-hidden="true"
                className="animate-spin"
              />
            ) : (
              <Play data-icon="inline-start" aria-hidden="true" />
            )}
            {t("probe.runNow")}
          </Button>
        ) : null}
      </div>

      <div className="space-y-4">
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
          <Alert
            variant={feedback.kind === "error" ? "destructive" : "default"}
          >
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
                <AlertDescription>
                  {t("probe.lowMemory.detail")}
                </AlertDescription>
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
            <ProbeStatusCard probe={state.probe} />
            <TaskCard task={state.probe.task} />
            <RecentRunsCard runs={state.probe.recentRuns} />
            <ProbeSettingsCard
              key={`${state.probe.schedule.enabled}:${state.probe.schedule.cron}:${state.probe.schedule.timezone}:${state.probe.lowMemoryOverride}`}
              nodeId={nodeId}
              probe={state.probe}
              csrfToken={csrfToken}
              onChange={replaceProbe}
            />
          </>
        ) : null}
      </div>
    </div>
  );
}

function ProbeStatusCard({ probe }: { probe: NodeProbeState }) {
  const { t, i18n } = useTranslation();
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
          <Badge variant={status?.activeRunId ? "default" : "outline"}>
            {status?.activeRunId
              ? t("probe.status.running")
              : t("probe.status.idle")}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <StatusValue
            label={t("probe.status.next")}
            value={formatTime(
              status?.nextScheduledAt,
              i18n.resolvedLanguage,
              t("probe.notAvailable"),
            )}
          />
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

function StatusValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
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
            <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <StatusValue
                label={t("probe.task.createdAt")}
                value={formatTime(
                  task.createdAt,
                  i18n.resolvedLanguage,
                  t("probe.notAvailable"),
                )}
              />
              <StatusValue
                label={t("probe.task.receivedAt")}
                value={formatTime(
                  task.acknowledgedAt,
                  i18n.resolvedLanguage,
                  t("probe.task.waiting"),
                )}
              />
              <StatusValue
                label={t("probe.task.startedAt")}
                value={formatTime(
                  task.startedAt,
                  i18n.resolvedLanguage,
                  t("probe.notAvailable"),
                )}
              />
              <StatusValue
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

function RecentRunsCard({ runs }: { runs: ProbeRunSummary[] }) {
  const { t, i18n } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("probe.runs.title")}</CardTitle>
        <CardDescription>{t("probe.runs.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">{runs.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-2">
        {runs.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("probe.runs.empty")}
          </p>
        ) : (
          runs.map((run) => (
            <Link
              key={run.id}
              to={`/probe-runs/${run.id}`}
              className="flex flex-col gap-3 rounded-md border p-4 transition-colors hover:bg-muted/50 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <ProbeStatusBadge status={run.status} />
                  <span className="text-sm font-medium">
                    {t(`probe.trigger.${run.trigger}`)}
                  </span>
                </div>
                <p className="mt-2 text-xs text-muted-foreground">
                  {formatTime(
                    run.startedAt,
                    i18n.resolvedLanguage,
                    t("probe.notAvailable"),
                  )}
                </p>
              </div>
              <span className="text-sm text-muted-foreground">
                {t("probe.runs.progress", {
                  completed: run.completedExecutions,
                  total: run.expectedExecutions,
                })}
              </span>
            </Link>
          ))
        )}
      </CardContent>
    </Card>
  );
}

function ProbeSettingsCard({
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
  const { t } = useTranslation();
  const [form, setForm] = useState<NodeProbeSettingsUpdate>({
    schedule: { ...probe.schedule },
    lowMemoryOverride: probe.lowMemoryOverride,
  });
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<string>();

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setFeedback(undefined);
    try {
      onChange(await updateNodeProbeSettings(nodeId, form, csrfToken));
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("probe.settings.title")}</CardTitle>
        <CardDescription>{t("probe.settings.detail")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="space-y-5" onSubmit={(event) => void submit(event)}>
          {feedback ? (
            <Alert variant="destructive">
              <TriangleAlert aria-hidden="true" />
              <AlertDescription>{feedback}</AlertDescription>
            </Alert>
          ) : null}
          <div className="flex items-start justify-between gap-4 rounded-md border p-4">
            <div>
              <Label htmlFor="probe-schedule-enabled">
                {t("probe.settings.scheduleEnabled")}
              </Label>
              <p className="mt-1 text-sm text-muted-foreground">
                {t("probe.settings.scheduleEnabledDetail")}
              </p>
            </div>
            <Switch
              id="probe-schedule-enabled"
              checked={form.schedule.enabled}
              disabled={saving}
              onCheckedChange={(enabled) =>
                setForm((current) => ({
                  ...current,
                  schedule: { ...current.schedule, enabled },
                }))
              }
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="probe-cron">{t("probe.settings.cron")}</Label>
              <Input
                id="probe-cron"
                value={form.schedule.cron}
                disabled={saving}
                autoComplete="off"
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    schedule: { ...current.schedule, cron: event.target.value },
                  }))
                }
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="probe-timezone">
                {t("probe.settings.timezone")}
              </Label>
              <Input
                id="probe-timezone"
                value={form.schedule.timezone}
                disabled={saving}
                autoComplete="off"
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    schedule: {
                      ...current.schedule,
                      timezone: event.target.value,
                    },
                  }))
                }
              />
            </div>
          </div>
          <div className="flex items-start justify-between gap-4 rounded-md border p-4">
            <div>
              <Label htmlFor="probe-memory-override">
                {t("probe.settings.memoryOverride")}
              </Label>
              <p className="mt-1 text-sm text-muted-foreground">
                {t("probe.settings.memoryOverrideDetail")}
              </p>
            </div>
            <Switch
              id="probe-memory-override"
              checked={form.lowMemoryOverride}
              disabled={saving}
              onCheckedChange={(lowMemoryOverride) =>
                setForm((current) => ({ ...current, lowMemoryOverride }))
              }
            />
          </div>
          <Button type="submit" disabled={saving}>
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
  const destructive =
    status === "failed" ||
    status === "rejected" ||
    status === "expired" ||
    status === "interrupted";
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

function formatBytes(value: number, locale: string | undefined) {
  return new Intl.NumberFormat(locale, {
    style: "unit",
    unit: "megabyte",
    maximumFractionDigits: 0,
  }).format(value / (1024 * 1024));
}

function ProbeSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      {[0, 1, 2].map((item) => (
        <Card key={item}>
          <CardHeader>
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-64 max-w-full" />
          </CardHeader>
          <CardContent>
            <Skeleton className="h-20 w-full" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
