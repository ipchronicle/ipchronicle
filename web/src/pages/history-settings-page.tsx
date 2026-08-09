import { useCallback, useEffect, useState } from "react";
import { Database, RefreshCw, Trash2, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";

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
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatAPIError } from "@/lib/api-error";
import { formatTime } from "@/pages/node-probe-page";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; history: HistoryState }
  | { kind: "error" };

export function HistorySettingsPage() {
  const { t, i18n } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [working, setWorking] = useState(false);
  const [feedback, setFeedback] = useState<string>();

  const load = useCallback(async (signal?: AbortSignal) => {
    setState({ kind: "loading" });
    try {
      setState({ kind: "success", history: await getHistoryState(signal) });
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setState({ kind: "error" });
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function clearHistory() {
    setWorking(true);
    setFeedback(undefined);
    try {
      setState({ kind: "success", history: await resetHistory(csrfToken) });
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setWorking(false);
    }
  }

  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
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
        {state.kind === "loading" ? (
          <Card>
            <CardHeader>
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-64 max-w-full" />
            </CardHeader>
            <CardContent>
              <Skeleton className="h-28 w-full" />
            </CardContent>
          </Card>
        ) : null}
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
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertDescription>{feedback}</AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <>
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
                <dl className="grid gap-4 sm:grid-cols-2">
                  <div>
                    <dt className="text-xs text-muted-foreground">
                      {t("historySettings.state.generation")}
                    </dt>
                    <dd className="mt-1 break-all font-mono text-xs">
                      {state.history.generation}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted-foreground">
                      {t("historySettings.state.resetAt")}
                    </dt>
                    <dd className="mt-1 font-medium">
                      {formatTime(
                        state.history.resetAt,
                        i18n.resolvedLanguage,
                        t("historySettings.state.never"),
                      )}
                    </dd>
                  </div>
                </dl>
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
