import { useCallback, useEffect, useState } from "react";
import { Check, RefreshCw, Server, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";

import { getSystemStatus } from "@/api/system";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";

type SystemStatus = Awaited<ReturnType<typeof getSystemStatus>>;

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; status: SystemStatus; checkedAt: Date }
  | { kind: "error"; checkedAt: Date };

export function SystemStatusPage() {
  const { i18n, t } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });

  const loadStatus = useCallback((signal?: AbortSignal) => {
    setState({ kind: "loading" });
    void getSystemStatus(signal)
      .then((status) =>
        setState({ kind: "success", status, checkedAt: new Date() }),
      )
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }
        setState({ kind: "error", checkedAt: new Date() });
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    loadStatus(controller.signal);
    return () => controller.abort();
  }, [loadStatus]);

  const checkedAt =
    state.kind === "loading"
      ? t("status.notAvailable")
      : new Intl.DateTimeFormat(i18n.resolvedLanguage, {
          dateStyle: "medium",
          timeStyle: "medium",
        }).format(state.checkedAt);

  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-16">
      <div className="max-w-2xl">
        <p className="text-xs font-medium text-muted-foreground uppercase">
          {t("status.section")}
        </p>
        <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
          {t("status.title")}
        </h1>
      </div>

      <section
        className="mt-8 border-y"
        aria-live="polite"
        aria-busy={state.kind === "loading"}
      >
        <div className="flex min-h-36 flex-col justify-between gap-6 py-6 sm:flex-row sm:items-center">
          <StatusSummary state={state} />
          {state.kind === "error" ? (
            <Button variant="outline" onClick={() => loadStatus()}>
              <RefreshCw data-icon="inline-start" aria-hidden="true" />
              {t("status.retry")}
            </Button>
          ) : null}
        </div>

        <Separator />

        <dl className="grid sm:grid-cols-3">
          <StatusField
            label={t("status.service")}
            value={state.kind === "success" ? state.status.service : undefined}
          />
          <StatusField
            label={t("status.version")}
            value={state.kind === "success" ? state.status.version : undefined}
          />
          <StatusField label={t("status.checkedAt")} value={checkedAt} />
        </dl>
      </section>
    </main>
  );
}

function StatusSummary({ state }: { state: ViewState }) {
  const { t } = useTranslation();

  if (state.kind === "loading") {
    return (
      <div className="flex items-start gap-4">
        <Skeleton className="size-10 shrink-0 rounded-md" />
        <div className="space-y-2">
          <Skeleton className="h-5 w-36" />
          <Skeleton className="h-4 w-56 max-w-full" />
          <span className="sr-only">{t("status.checking")}</span>
        </div>
      </div>
    );
  }

  const healthy = state.kind === "success";
  return (
    <div className="flex items-start gap-4">
      <span
        className={
          healthy
            ? "flex size-10 shrink-0 items-center justify-center rounded-md bg-emerald-100 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300"
            : "flex size-10 shrink-0 items-center justify-center rounded-md bg-destructive/10 text-destructive"
        }
      >
        {healthy ? (
          <Check aria-hidden="true" />
        ) : (
          <TriangleAlert aria-hidden="true" />
        )}
      </span>
      <div>
        <Badge variant={healthy ? "outline" : "destructive"}>
          {healthy ? t("status.operational") : t("status.unavailable")}
        </Badge>
        <p className="mt-2 text-sm text-muted-foreground">
          {healthy ? t("status.healthyDetail") : t("status.errorDetail")}
        </p>
      </div>
    </div>
  );
}

function StatusField({ label, value }: { label: string; value?: string }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 border-b py-5 last:border-b-0 sm:border-r sm:border-b-0 sm:px-5 sm:first:pl-0 sm:last:border-r-0 sm:last:pr-0">
      <dt className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        <Server aria-hidden="true" className="size-3.5" />
        {label}
      </dt>
      <dd className="mt-2 break-all text-sm font-medium">
        {value ?? t("status.notAvailable")}
      </dd>
    </div>
  );
}
