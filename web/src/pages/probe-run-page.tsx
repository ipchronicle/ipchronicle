import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  CheckCircle2,
  Clock3,
  FileJson2,
  RefreshCw,
  Route,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";

import { getNodeNetwork, type PublicAddress } from "@/api/network";
import { listNodes, type Node } from "@/api/nodes";
import { getProbeRun, type ProbeExecution, type ProbeRun } from "@/api/probes";
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
import { formatTime, ProbeStatusBadge } from "@/pages/node-probe-page";

type ViewState =
  | { kind: "loading" }
  | {
      kind: "success";
      run: ProbeRun;
      node?: Node;
      publicAddresses: PublicAddress[];
    }
  | { kind: "not-found" }
  | { kind: "error" };

export function ProbeRunPage() {
  const { runId = "" } = useParams();
  const { t, i18n } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(
    async (signal?: AbortSignal, initial = false, quiet = false) => {
      if (initial) setState({ kind: "loading" });
      else if (!quiet) setRefreshing(true);
      try {
        const run = await getProbeRun(runId, signal);
        const [nodes, network] = await Promise.all([
          listNodes(signal),
          getNodeNetwork(run.nodeId, signal),
        ]);
        setState({
          kind: "success",
          run,
          node: nodes.find((node) => node.id === run.nodeId),
          publicAddresses: network.publicAddresses,
        });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        const status =
          typeof error === "object" && error !== null && "status" in error
            ? error.status
            : undefined;
        setState(status === 404 ? { kind: "not-found" } : { kind: "error" });
      } finally {
        if (!quiet) setRefreshing(false);
      }
    },
    [runId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    return () => controller.abort();
  }, [load]);

  const running = state.kind === "success" && state.run.status === "running";
  useEffect(() => {
    if (!running) return;
    const timer = window.setInterval(
      () => void load(undefined, false, true),
      5000,
    );
    return () => window.clearInterval(timer);
  }, [load, running]);

  const backTarget =
    state.kind === "success" ? `/nodes/${state.run.nodeId}/probe` : "/nodes";
  const addressNames = useMemo(
    () =>
      new Map(
        state.kind === "success"
          ? state.publicAddresses.map(
              (address) => [address.id, address.address] as const,
            )
          : [],
      ),
    [state],
  );

  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0 max-w-2xl">
          <Button variant="ghost" size="sm" asChild className="mb-3 -ml-3">
            <Link to={backTarget}>
              <ArrowLeft data-icon="inline-start" aria-hidden="true" />
              {t("probeRun.back")}
            </Link>
          </Button>
          <p className="text-sm font-medium text-muted-foreground uppercase">
            {t("probeRun.section")}
          </p>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
            {t("probeRun.title")}
          </h1>
          <p className="mt-2 break-all text-sm text-muted-foreground">
            {state.kind === "success" ? state.run.id : t("probeRun.detail")}
          </p>
        </div>
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
          {t("probeRun.refresh")}
        </Button>
      </div>

      <div className="mt-8 space-y-4" aria-live="polite">
        {state.kind === "loading" ? <RunSkeleton /> : null}
        {state.kind === "not-found" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("probeRun.notFound")}</AlertTitle>
          </Alert>
        ) : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("probeRun.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => void load(undefined, true)}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("probeRun.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <>
            {state.run.status === "partial" ? (
              <Alert>
                <TriangleAlert aria-hidden="true" />
                <AlertTitle>{t("probeRun.partial.title")}</AlertTitle>
                <AlertDescription>
                  {t("probeRun.partial.detail")}
                </AlertDescription>
              </Alert>
            ) : null}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Clock3 aria-hidden="true" className="size-4" />
                  {t("probeRun.summary.title")}
                </CardTitle>
                <CardDescription>
                  {state.node?.name ?? state.run.nodeId}
                </CardDescription>
                <CardAction>
                  <ProbeStatusBadge status={state.run.status} />
                </CardAction>
              </CardHeader>
              <CardContent>
                <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
                  <SummaryValue
                    label={t("probeRun.summary.trigger")}
                    value={t(`probe.trigger.${state.run.trigger}`)}
                  />
                  <SummaryValue
                    label={t("probeRun.summary.startedAt")}
                    value={formatTime(
                      state.run.startedAt,
                      i18n.resolvedLanguage,
                      t("probe.notAvailable"),
                    )}
                  />
                  <SummaryValue
                    label={t("probeRun.summary.completedAt")}
                    value={formatTime(
                      state.run.completedAt,
                      i18n.resolvedLanguage,
                      t("probe.notAvailable"),
                    )}
                  />
                  <SummaryValue
                    label={t("probeRun.summary.progress")}
                    value={t("probe.runs.progress", {
                      completed: state.run.executions.filter(
                        (execution) =>
                          execution.status !== "pending" &&
                          execution.status !== "running",
                      ).length,
                      total: state.run.expectedExecutions,
                    })}
                  />
                </dl>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Route aria-hidden="true" className="size-4" />
                  {t("probeRun.executions.title")}
                </CardTitle>
                <CardDescription>
                  {t("probeRun.executions.detail")}
                </CardDescription>
                <CardAction>
                  <Badge variant="secondary">
                    {state.run.executions.length}
                  </Badge>
                </CardAction>
              </CardHeader>
              <CardContent className="space-y-3">
                {state.run.executions.map((execution) => (
                  <ExecutionRow
                    key={execution.id}
                    run={state.run}
                    execution={execution}
                    name={
                      addressNames.get(execution.egressId) ??
                      t("probeRun.executions.addressUnavailable")
                    }
                  />
                ))}
              </CardContent>
            </Card>
          </>
        ) : null}
      </div>
    </main>
  );
}

function ExecutionRow({
  run,
  execution,
  name,
}: {
  run: ProbeRun;
  execution: ProbeExecution;
  name: string;
}) {
  const { t, i18n } = useTranslation();
  return (
    <div className="rounded-md border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="break-words font-medium">{name}</p>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("probeRun.executions.sequence", { value: execution.sequence })}
          </p>
        </div>
        <ProbeStatusBadge status={execution.status} />
      </div>
      <dl className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <SummaryValue
          label={t("probeRun.executions.startedAt")}
          value={formatTime(
            execution.startedAt,
            i18n.resolvedLanguage,
            t("probe.notAvailable"),
          )}
        />
        <SummaryValue
          label={t("probeRun.executions.completedAt")}
          value={formatTime(
            execution.completedAt,
            i18n.resolvedLanguage,
            t("probe.notAvailable"),
          )}
        />
        <SummaryValue
          label={t("probeRun.executions.stage")}
          value={
            execution.failureStage
              ? t(`probe.failure.${execution.failureStage}`)
              : t("probe.notAvailable")
          }
        />
      </dl>
      {execution.diagnostic ? (
        <pre className="mt-4 max-h-48 overflow-auto whitespace-pre-wrap rounded-md bg-muted p-3 text-sm leading-5">
          {execution.diagnostic}
        </pre>
      ) : null}
      {execution.snapshotId ? (
        <Button variant="outline" size="sm" asChild className="mt-4">
          <Link to={`/probe-snapshots/${execution.snapshotId}?runId=${run.id}`}>
            <FileJson2 data-icon="inline-start" aria-hidden="true" />
            {t("probeRun.executions.openSnapshot")}
          </Link>
        </Button>
      ) : execution.status === "succeeded" ? (
        <p className="mt-4 flex items-center gap-2 text-sm text-muted-foreground">
          <CheckCircle2 aria-hidden="true" className="size-4" />
          {t("probeRun.executions.snapshotPending")}
        </p>
      ) : null}
    </div>
  );
}

function SummaryValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-words font-medium">{value}</dd>
    </div>
  );
}

function RunSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
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
  );
}
