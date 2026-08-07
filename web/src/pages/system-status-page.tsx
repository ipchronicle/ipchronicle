import { type ReactNode, useCallback, useEffect, useState } from "react";
import {
  Check,
  Database,
  ExternalLink,
  RefreshCw,
  Server,
  ShieldAlert,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";

import { getSystemStatus } from "@/api/system";
import { useAuth } from "@/auth-context";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

type SystemStatus = Awaited<ReturnType<typeof getSystemStatus>>;

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; status: SystemStatus; checkedAt: Date }
  | { kind: "error"; checkedAt: Date };

export function SystemStatusPage() {
  const { i18n, t } = useTranslation();
  const { state: authState } = useAuth();
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
  const account =
    authState.status === "authenticated" ? authState.session.account : null;

  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="max-w-2xl">
        <p className="text-xs font-medium text-muted-foreground uppercase">
          {t("status.section")}
        </p>
        <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
          {t("status.title")}
        </h1>
      </div>

      <div className="mt-8 space-y-3">
        {account?.usesDefaultCredentials ? (
          <Alert variant="destructive">
            <ShieldAlert aria-hidden="true" />
            <AlertTitle>{t("status.defaultCredentialsTitle")}</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>{t("status.defaultCredentialsDetail")}</span>
              <Button asChild variant="outline" size="sm">
                <Link to="/settings/account">
                  {t("status.openAccount")}
                  <ExternalLink data-icon="inline-end" aria-hidden="true" />
                </Link>
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
      </div>

      <Card
        className="mt-6 gap-0"
        aria-live="polite"
        aria-busy={state.kind === "loading"}
      >
        <CardHeader className="pb-6">
          <div className="flex min-h-24 flex-col justify-between gap-6 sm:flex-row sm:items-center">
            <StatusSummary state={state} />
            {state.kind === "error" ? (
              <Button variant="outline" onClick={() => loadStatus()}>
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("status.retry")}
              </Button>
            ) : null}
          </div>
        </CardHeader>
        <CardContent className="border-t px-0">
          <dl className="grid sm:grid-cols-2 lg:grid-cols-4">
            <StatusField
              icon={<Server aria-hidden="true" />}
              label={t("status.service")}
              value={
                state.kind === "success" ? state.status.service : undefined
              }
            />
            <StatusField
              icon={<Server aria-hidden="true" />}
              label={t("status.version")}
              value={
                state.kind === "success" ? state.status.version : undefined
              }
            />
            <StatusField
              icon={<Database aria-hidden="true" />}
              label={t("status.configSchema")}
              value={
                state.kind === "success"
                  ? String(state.status.configSchemaVersion)
                  : undefined
              }
            />
            <StatusField
              icon={<Database aria-hidden="true" />}
              label={t("status.historySchema")}
              value={
                state.kind === "success"
                  ? String(state.status.historySchemaVersion)
                  : undefined
              }
            />
            <StatusField
              icon={<ShieldAlert aria-hidden="true" />}
              label={t("status.transport")}
              value={
                state.kind === "success"
                  ? state.status.transportSecurity.toUpperCase()
                  : undefined
              }
            />
            <StatusField
              icon={<Server aria-hidden="true" />}
              label={t("status.externalOrigin")}
              value={
                state.kind === "success"
                  ? t(
                      state.status.externalOriginConfigured
                        ? "status.configured"
                        : "status.notConfigured",
                    )
                  : undefined
              }
            />
            <StatusField
              icon={<Server aria-hidden="true" />}
              label={t("status.trustedProxy")}
              value={
                state.kind === "success"
                  ? t(
                      state.status.trustedProxyConfigured
                        ? "status.configured"
                        : "status.notConfigured",
                    )
                  : undefined
              }
            />
            <StatusField
              icon={<RefreshCw aria-hidden="true" />}
              label={t("status.checkedAt")}
              value={checkedAt}
            />
          </dl>
        </CardContent>
      </Card>
    </div>
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

function StatusField({
  icon,
  label,
  value,
}: {
  icon: ReactNode;
  label: string;
  value?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 border-b px-4 py-5 last:border-b-0 sm:border-r sm:px-5 sm:nth-[2n]:border-r-0 sm:nth-[n+7]:border-b-0 lg:nth-[2n]:border-r lg:nth-[4n]:border-r-0 lg:nth-[n+5]:border-b-0">
      <dt className="flex items-center gap-2 text-xs font-medium text-muted-foreground [&_svg]:size-3.5">
        {icon}
        {label}
      </dt>
      <dd className="mt-2 break-all text-sm font-medium">
        {value ?? t("status.notAvailable")}
      </dd>
    </div>
  );
}
