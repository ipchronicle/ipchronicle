import { useCallback, useEffect, useState } from "react";
import {
  Database,
  HardDrive,
  RefreshCw,
  Save,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  cleanupHistory,
  updateHistoryRetention,
  type HistoryRetentionUpdate,
} from "@/api/history";
import { getHistoryState, resetHistory, type HistoryState } from "@/api/probes";
import { useAuth } from "@/auth-context";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
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
  | { kind: "success"; history: HistoryState }
  | { kind: "error" };

type Feedback = { kind: "success" | "error"; text: string };

export function HistorySettingsPage() {
  const { t, i18n } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [mode, setMode] = useState<"indefinite" | "age" | "size">("indefinite");
  const [ageDays, setAgeDays] = useState("30");
  const [sizeMiB, setSizeMiB] = useState("1024");
  const [working, setWorking] = useState(false);
  const [feedback, setFeedback] = useState<Feedback>();

  const applyState = useCallback((history: HistoryState) => {
    setState({ kind: "success", history });
    setMode(history.retention.mode);
    if (history.retention.maxAgeDays !== undefined) {
      setAgeDays(String(history.retention.maxAgeDays));
    }
    if (history.retention.maxLogicalBytes !== undefined) {
      setSizeMiB(
        String(Math.round(history.retention.maxLogicalBytes / 1024 / 1024)),
      );
    }
  }, []);

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setState({ kind: "loading" });
      try {
        applyState(await getHistoryState(signal));
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

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function clearHistory() {
    await runAction(async () => {
      applyState(await resetHistory(csrfToken));
      return t("historySettings.feedback.cleared");
    });
  }

  async function saveRetention() {
    const update = retentionUpdate(mode, ageDays, sizeMiB);
    if (!update) {
      setFeedback({
        kind: "error",
        text: t("historySettings.retention.invalid"),
      });
      return;
    }
    await runAction(async () => {
      applyState(await updateHistoryRetention(update, csrfToken));
      return t("historySettings.feedback.saved");
    });
  }

  async function cleanNow() {
    await runAction(async () => {
      const result = await cleanupHistory(csrfToken);
      applyState(await getHistoryState());
      return t("historySettings.feedback.cleaned", {
        count: result.deletedItems,
      });
    });
  }

  async function runAction(action: () => Promise<string>) {
    setWorking(true);
    setFeedback(undefined);
    try {
      setFeedback({ kind: "success", text: await action() });
    } catch (error) {
      setFeedback({ kind: "error", text: formatAPIError(error, t) });
    } finally {
      setWorking(false);
    }
  }

  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="max-w-2xl">
        <p className="text-xs font-medium text-muted-foreground uppercase">
          {t("settings.section")}
        </p>
        <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
          {t("historySettings.title")}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {t("historySettings.detail")}
        </p>
      </div>

      <div className="mt-8 space-y-4" aria-live="polite">
        {state.kind === "loading" ? <SettingsSkeleton /> : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("historySettings.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => void load()}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("historySettings.retry")}
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
            <AlertDescription>{feedback.text}</AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <>
            {state.history.usage.overBudget ? (
              <Alert variant="destructive">
                <TriangleAlert aria-hidden="true" />
                <AlertTitle>{t("historySettings.usage.overBudget")}</AlertTitle>
                <AlertDescription>
                  {t("historySettings.usage.overage", {
                    value: formatBytes(
                      state.history.usage.overageBytes,
                      i18n.resolvedLanguage,
                    ),
                  })}
                </AlertDescription>
              </Alert>
            ) : null}

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <HardDrive aria-hidden="true" className="size-4" />
                  {t("historySettings.usage.title")}
                </CardTitle>
                <CardDescription>
                  {t("historySettings.usage.detail")}
                </CardDescription>
                <CardAction>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void cleanNow()}
                    disabled={working}
                  >
                    <RefreshCw data-icon="inline-start" aria-hidden="true" />
                    {t("historySettings.cleanup.action")}
                  </Button>
                </CardAction>
              </CardHeader>
              <CardContent>
                <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                  <UsageItem
                    label={t("historySettings.usage.logical")}
                    value={formatBytes(
                      state.history.usage.logicalBytes,
                      i18n.resolvedLanguage,
                    )}
                  />
                  <UsageItem
                    label={t("historySettings.usage.protected")}
                    value={formatBytes(
                      state.history.usage.protectedLogicalBytes,
                      i18n.resolvedLanguage,
                    )}
                  />
                  <UsageItem
                    label={t("historySettings.usage.records")}
                    value={String(state.history.usage.recordCount)}
                  />
                  <UsageItem
                    label={t("historySettings.usage.physical")}
                    value={formatBytes(
                      state.history.usage.databaseBytes +
                        state.history.usage.walBytes +
                        state.history.usage.sharedMemoryBytes,
                      i18n.resolvedLanguage,
                    )}
                  />
                </dl>
                <div className="mt-4 flex flex-wrap gap-2 text-xs text-muted-foreground">
                  <Badge variant="outline">
                    DB{" "}
                    {formatBytes(
                      state.history.usage.databaseBytes,
                      i18n.resolvedLanguage,
                    )}
                  </Badge>
                  <Badge variant="outline">
                    WAL{" "}
                    {formatBytes(
                      state.history.usage.walBytes,
                      i18n.resolvedLanguage,
                    )}
                  </Badge>
                  <Badge variant="outline">
                    SHM{" "}
                    {formatBytes(
                      state.history.usage.sharedMemoryBytes,
                      i18n.resolvedLanguage,
                    )}
                  </Badge>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>{t("historySettings.retention.title")}</CardTitle>
                <CardDescription>
                  {t("historySettings.retention.detail")}
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-5">
                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="retention-mode">
                      {t("historySettings.retention.mode")}
                    </Label>
                    <Select
                      value={mode}
                      onValueChange={(value) => setMode(value as typeof mode)}
                    >
                      <SelectTrigger id="retention-mode" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="indefinite">
                          {t("historySettings.retention.indefinite")}
                        </SelectItem>
                        <SelectItem value="age">
                          {t("historySettings.retention.age")}
                        </SelectItem>
                        <SelectItem value="size">
                          {t("historySettings.retention.size")}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  {mode === "age" ? (
                    <div className="space-y-2">
                      <Label htmlFor="retention-age">
                        {t("historySettings.retention.days")}
                      </Label>
                      <Input
                        id="retention-age"
                        type="number"
                        min={1}
                        max={36500}
                        value={ageDays}
                        onChange={(event) => setAgeDays(event.target.value)}
                      />
                    </div>
                  ) : null}
                  {mode === "size" ? (
                    <div className="space-y-2">
                      <Label htmlFor="retention-size">
                        {t("historySettings.retention.mib")}
                      </Label>
                      <Input
                        id="retention-size"
                        type="number"
                        min={1}
                        max={1048576}
                        value={sizeMiB}
                        onChange={(event) => setSizeMiB(event.target.value)}
                      />
                    </div>
                  ) : null}
                </div>
                <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
                  <div className="text-xs text-muted-foreground">
                    {t("historySettings.retention.updated", {
                      value: formatTime(
                        state.history.retention.updatedAt,
                        i18n.resolvedLanguage,
                        t("probe.notAvailable"),
                      ),
                    })}
                  </div>
                  <Button
                    onClick={() => void saveRetention()}
                    disabled={working}
                  >
                    <Save data-icon="inline-start" aria-hidden="true" />
                    {t("historySettings.retention.save")}
                  </Button>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Database aria-hidden="true" className="size-4" />
                  {t("historySettings.state.title")}
                </CardTitle>
                <CardDescription>
                  {t("historySettings.state.detail")}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                  <UsageItem
                    label={t("historySettings.state.generation")}
                    value={state.history.generation}
                    mono
                  />
                  <UsageItem
                    label={t("historySettings.state.resetAt")}
                    value={formatTime(
                      state.history.resetAt,
                      i18n.resolvedLanguage,
                      t("historySettings.state.never"),
                    )}
                  />
                  <UsageItem
                    label={t("historySettings.cleanup.lastRun")}
                    value={formatTime(
                      state.history.retention.lastCleanupAt,
                      i18n.resolvedLanguage,
                      t("historySettings.cleanup.never"),
                    )}
                  />
                  <UsageItem
                    label={t("historySettings.cleanup.lastDeleted")}
                    value={String(
                      state.history.retention.lastCleanupDeletedItems,
                    )}
                  />
                </dl>
                {state.history.retention.lastCleanupError ? (
                  <Alert variant="destructive" className="mt-4">
                    <TriangleAlert aria-hidden="true" />
                    <AlertTitle>
                      {t("historySettings.cleanup.failed")}
                    </AlertTitle>
                    <AlertDescription>
                      {state.history.retention.lastCleanupError}
                    </AlertDescription>
                  </Alert>
                ) : null}
              </CardContent>
            </Card>

            <Card className="ring-destructive/30">
              <CardHeader>
                <CardTitle className="text-destructive">
                  {t("historySettings.danger.title")}
                </CardTitle>
                <CardDescription>
                  {t("historySettings.danger.detail")}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button variant="destructive" disabled={working}>
                      <Trash2 data-icon="inline-start" aria-hidden="true" />
                      {t("historySettings.danger.action")}
                    </Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogMedia>
                        <Trash2 aria-hidden="true" />
                      </AlertDialogMedia>
                      <AlertDialogTitle>
                        {t("historySettings.danger.confirmTitle")}
                      </AlertDialogTitle>
                      <AlertDialogDescription>
                        {t("historySettings.danger.confirmDetail")}
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>
                        {t("common.cancel")}
                      </AlertDialogCancel>
                      <AlertDialogAction
                        variant="destructive"
                        onClick={() => void clearHistory()}
                      >
                        {t("historySettings.danger.confirm")}
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </CardContent>
            </Card>
          </>
        ) : null}
      </div>
    </main>
  );
}

function UsageItem({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd
        className={`mt-1 break-all font-medium ${mono ? "font-mono text-xs" : ""}`}
      >
        {value}
      </dd>
    </div>
  );
}

function SettingsSkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-64 max-w-full" />
      </CardHeader>
      <CardContent>
        <Skeleton className="h-40 w-full" />
      </CardContent>
    </Card>
  );
}

function retentionUpdate(
  mode: "indefinite" | "age" | "size",
  ageDays: string,
  sizeMiB: string,
): HistoryRetentionUpdate | undefined {
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
