import { useCallback, useEffect, useState } from "react";
import {
  HardDrive,
  LoaderCircle,
  RefreshCw,
  Save,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  cleanupLogs,
  getLogRetention,
  updateLogRetention,
  type LogRetentionState,
  type LogRetentionUpdate,
} from "@/api/logs";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { formatAPIError } from "@/lib/api-error";
import { formatTime } from "@/pages/node-probe-page";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; value: LogRetentionState }
  | { kind: "error" };

type RetentionMode = LogRetentionUpdate["mode"];

export function LogRetentionSettings({ csrfToken }: { csrfToken: string }) {
  const { i18n, t } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [mode, setMode] = useState<RetentionMode>("age");
  const [ageDays, setAgeDays] = useState("7");
  const [sizeMiB, setSizeMiB] = useState("256");
  const [working, setWorking] = useState<"save" | "cleanup">();
  const [feedback, setFeedback] = useState<
    { kind: "success" | "error"; message: string } | undefined
  >();

  const applyState = useCallback((value: LogRetentionState) => {
    setState({ kind: "success", value });
    setMode(value.mode);
    if (value.maxAgeDays !== undefined) setAgeDays(String(value.maxAgeDays));
    if (value.maxLogicalBytes !== undefined) {
      setSizeMiB(String(Math.round(value.maxLogicalBytes / 1024 / 1024)));
    }
  }, []);

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setState({ kind: "loading" });
      setFeedback(undefined);
      try {
        applyState(await getLogRetention(signal));
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setState({ kind: "error" });
      }
    },
    [applyState],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  async function save() {
    const update = retentionUpdate(mode, ageDays, sizeMiB);
    if (update === undefined) {
      setFeedback({
        kind: "error",
        message: t("systemSettings.logRetention.invalid"),
      });
      return;
    }
    setWorking("save");
    setFeedback(undefined);
    try {
      applyState(await updateLogRetention(update, csrfToken));
      setFeedback({
        kind: "success",
        message: t("systemSettings.logRetention.saved"),
      });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(undefined);
    }
  }

  async function cleanNow() {
    setWorking("cleanup");
    setFeedback(undefined);
    try {
      const value = await cleanupLogs(csrfToken);
      applyState(value);
      setFeedback({
        kind: "success",
        message: t("systemSettings.logRetention.cleaned", {
          count: value.lastCleanupDeletedItems,
        }),
      });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(undefined);
    }
  }

  return (
    <Card className="mt-8" aria-live="polite">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <HardDrive aria-hidden="true" className="size-4" />
          {t("systemSettings.logRetention.title")}
        </CardTitle>
        <CardDescription>
          {t("systemSettings.logRetention.detail")}
        </CardDescription>
        {state.kind === "success" ? (
          <CardAction>
            <Button
              variant="outline"
              size="sm"
              disabled={working !== undefined}
              onClick={() => void cleanNow()}
            >
              {working === "cleanup" ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
              )}
              {t("systemSettings.logRetention.cleanNow")}
            </Button>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-5">
        {state.kind === "loading" ? (
          <Skeleton className="h-36 w-full" aria-busy="true" />
        ) : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>
              {t("systemSettings.logRetention.loadFailed")}
            </AlertTitle>
            <AlertDescription>
              <Button
                className="mt-3"
                variant="outline"
                size="sm"
                onClick={() => void load()}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("systemSettings.logRetention.retry")}
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
            ) : null}
            <AlertDescription>{feedback.message}</AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <>
            <dl className="grid gap-4 rounded-md border p-4 sm:grid-cols-2 lg:grid-cols-4">
              <UsageItem
                label={t("systemSettings.logRetention.logicalUsage")}
                value={formatBytes(
                  state.value.logicalBytes,
                  i18n.resolvedLanguage,
                )}
              />
              <UsageItem
                label={t("systemSettings.logRetention.records")}
                value={String(state.value.recordCount)}
              />
              <UsageItem
                label={t("systemSettings.logRetention.lastCleanup")}
                value={formatTime(
                  state.value.lastCleanupAt,
                  i18n.resolvedLanguage,
                  t("systemSettings.logRetention.never"),
                )}
              />
              <UsageItem
                label={t("systemSettings.logRetention.lastDeleted")}
                value={String(state.value.lastCleanupDeletedItems)}
              />
            </dl>
            {state.value.lastCleanupError ? (
              <Alert variant="destructive">
                <TriangleAlert aria-hidden="true" />
                <AlertTitle>
                  {t("systemSettings.logRetention.cleanupFailed")}
                </AlertTitle>
                <AlertDescription>
                  {state.value.lastCleanupError}
                </AlertDescription>
              </Alert>
            ) : null}
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="log-retention-mode">
                  {t("systemSettings.logRetention.mode")}
                </Label>
                <Select
                  value={mode}
                  disabled={working !== undefined}
                  onValueChange={(value) => setMode(value as RetentionMode)}
                >
                  <SelectTrigger id="log-retention-mode" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="indefinite">
                      {t("systemSettings.logRetention.indefinite")}
                    </SelectItem>
                    <SelectItem value="age">
                      {t("systemSettings.logRetention.age")}
                    </SelectItem>
                    <SelectItem value="size">
                      {t("systemSettings.logRetention.size")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {mode === "age" ? (
                <div className="space-y-2">
                  <Label htmlFor="log-retention-age">
                    {t("systemSettings.logRetention.days")}
                  </Label>
                  <Input
                    id="log-retention-age"
                    type="number"
                    min={1}
                    max={36500}
                    disabled={working !== undefined}
                    value={ageDays}
                    onChange={(event) => setAgeDays(event.target.value)}
                  />
                </div>
              ) : null}
              {mode === "size" ? (
                <div className="space-y-2">
                  <Label htmlFor="log-retention-size">
                    {t("systemSettings.logRetention.mib")}
                  </Label>
                  <Input
                    id="log-retention-size"
                    type="number"
                    min={1}
                    max={1048576}
                    disabled={working !== undefined}
                    value={sizeMiB}
                    onChange={(event) => setSizeMiB(event.target.value)}
                  />
                </div>
              ) : null}
            </div>
            <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
              <p className="text-sm text-muted-foreground">
                {t("systemSettings.logRetention.updated", {
                  value: formatTime(
                    state.value.updatedAt,
                    i18n.resolvedLanguage,
                    t("logs.entry.notAvailable"),
                  ),
                })}
              </p>
              <Button
                disabled={working !== undefined}
                onClick={() => void save()}
              >
                {working === "save" ? (
                  <LoaderCircle
                    data-icon="inline-start"
                    aria-hidden="true"
                    className="animate-spin"
                  />
                ) : (
                  <Save data-icon="inline-start" aria-hidden="true" />
                )}
                {t("systemSettings.logRetention.save")}
              </Button>
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}

function UsageItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-all font-medium">{value}</dd>
    </div>
  );
}

function retentionUpdate(
  mode: RetentionMode,
  ageDays: string,
  sizeMiB: string,
): LogRetentionUpdate | undefined {
  if (mode === "indefinite") return { mode };
  if (mode === "age") {
    const parsed = Number(ageDays);
    return Number.isInteger(parsed) && parsed >= 1 && parsed <= 36500
      ? { mode, maxAgeDays: parsed }
      : undefined;
  }
  const parsed = Number(sizeMiB);
  return Number.isInteger(parsed) && parsed >= 1 && parsed <= 1048576
    ? { mode, maxLogicalBytes: parsed * 1024 * 1024 }
    : undefined;
}

function formatBytes(bytes: number, locale?: string) {
  return new Intl.NumberFormat(locale, {
    style: "unit",
    unit: "megabyte",
    unitDisplay: "short",
    maximumFractionDigits: 1,
  }).format(bytes / 1024 / 1024);
}
