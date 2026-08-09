import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowDown,
  ArrowLeft,
  CheckCircle2,
  GitCompareArrows,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import {
  compareProbeSnapshots,
  type ProbeSnapshotComparison,
} from "@/api/history";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { presentProbeField } from "@/lib/probe-field-label";

type ComparedField = ProbeSnapshotComparison["fields"][number];
type FieldSide = ComparedField["before"];

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; comparison: ProbeSnapshotComparison }
  | { kind: "invalid" }
  | { kind: "not-found" }
  | { kind: "mismatch" }
  | { kind: "error" };

export function ProbeComparisonPage() {
  const { t } = useTranslation();
  const [search] = useSearchParams();
  const before = search.get("before") ?? "";
  const after = search.get("after") ?? "";
  const [state, setState] = useState<ViewState>(
    before && after ? { kind: "loading" } : { kind: "invalid" },
  );

  const load = useCallback(
    async (signal?: AbortSignal) => {
      if (!before || !after) {
        setState({ kind: "invalid" });
        return;
      }
      setState({ kind: "loading" });
      try {
        setState({
          kind: "success",
          comparison: await compareProbeSnapshots(before, after, signal),
        });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        const status =
          typeof error === "object" && error !== null && "status" in error
            ? error.status
            : undefined;
        setState(
          status === 404
            ? { kind: "not-found" }
            : status === 409
              ? { kind: "mismatch" }
              : { kind: "error" },
        );
      }
    },
    [after, before],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="max-w-3xl">
        <Button variant="ghost" size="sm" asChild className="mb-3 -ml-3">
          <Link to="/history">
            <ArrowLeft data-icon="inline-start" aria-hidden="true" />
            {t("comparison.back")}
          </Link>
        </Button>
        <p className="text-xs font-medium text-muted-foreground uppercase">
          {t("comparison.section")}
        </p>
        <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
          {t("comparison.title")}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {t("comparison.detail")}
        </p>
      </div>

      <div className="mt-8 space-y-4" aria-live="polite">
        {state.kind === "loading" ? <ComparisonSkeleton /> : null}
        {state.kind === "invalid" ? (
          <ComparisonError title={t("comparison.invalid")} />
        ) : null}
        {state.kind === "not-found" ? (
          <ComparisonError title={t("comparison.notFound")} />
        ) : null}
        {state.kind === "mismatch" ? (
          <ComparisonError title={t("comparison.egressMismatch")} />
        ) : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("comparison.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => void load()}
              >
                {t("comparison.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <ComparisonResult comparison={state.comparison} />
        ) : null}
      </div>
    </main>
  );
}

function ComparisonResult({
  comparison,
}: {
  comparison: ProbeSnapshotComparison;
}) {
  const { t } = useTranslation();
  const changed = useMemo(
    () => comparison.fields.filter((field) => field.changed),
    [comparison.fields],
  );
  const unchanged = useMemo(
    () => comparison.fields.filter((field) => !field.changed),
    [comparison.fields],
  );

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <GitCompareArrows className="size-4" aria-hidden="true" />
            {t("comparison.summary.title")}
          </CardTitle>
          <CardDescription>
            {t("comparison.summary.egress", { value: comparison.egressId })}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid items-center gap-3 sm:grid-cols-[1fr_auto_1fr]">
            <SnapshotReference
              label={t("comparison.before")}
              id={comparison.beforeId}
            />
            <ArrowDown
              className="mx-auto size-4 text-muted-foreground sm:-rotate-90"
              aria-hidden="true"
            />
            <SnapshotReference
              label={t("comparison.after")}
              id={comparison.afterId}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("comparison.changed.title")}</CardTitle>
          <CardDescription>
            {t("comparison.changed.count", { count: changed.length })}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {changed.length === 0 ? (
            <div className="flex min-h-32 flex-col items-center justify-center text-center">
              <CheckCircle2
                className="size-8 text-muted-foreground"
                aria-hidden="true"
              />
              <p className="mt-3 font-medium">
                {t("comparison.changed.empty")}
              </p>
            </div>
          ) : (
            <div className="space-y-3">
              {changed.map((field) => (
                <ComparedFieldRow key={field.id} field={field} />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {unchanged.length > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("comparison.unchanged.title")}</CardTitle>
            <CardDescription>
              {t("comparison.unchanged.count", { count: unchanged.length })}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <details>
              <summary className="cursor-pointer font-medium text-primary">
                {t("comparison.unchanged.show")}
              </summary>
              <div className="mt-4 space-y-3">
                {unchanged.map((field) => (
                  <ComparedFieldRow key={field.id} field={field} />
                ))}
              </div>
            </details>
          </CardContent>
        </Card>
      ) : null}
    </>
  );
}

function SnapshotReference({ label, id }: { label: string; id: string }) {
  return (
    <div className="min-w-0 rounded-md border p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <Link
        className="mt-1 block truncate font-mono text-xs text-primary underline-offset-4 hover:underline"
        to={`/probe-snapshots/${id}`}
      >
        {id}
      </Link>
    </div>
  );
}

function ComparedFieldRow({ field }: { field: ComparedField }) {
  const { t } = useTranslation();
  const presentation = presentProbeField(field, t);
  return (
    <div className="rounded-md border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="font-medium">{presentation.name}</div>
          <div className="mt-1 text-xs text-muted-foreground">
            {presentation.description}
          </div>
          <code className="mt-1 block break-all text-xs text-muted-foreground">
            {field.path}
          </code>
        </div>
        {field.changed ? <Badge>{t("comparison.changed.badge")}</Badge> : null}
      </div>
      <div className="mt-3 grid gap-3 md:grid-cols-2">
        <FieldValue side={field.before} label={t("comparison.before")} />
        <FieldValue side={field.after} label={t("comparison.after")} />
      </div>
    </div>
  );
}

function FieldValue({ side, label }: { side: FieldSide; label: string }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 rounded-md bg-muted p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">{label}</span>
        <Badge
          variant={side.status === "available" ? "outline" : "destructive"}
        >
          {t(`snapshot.fieldStatus.${side.status}`)}
        </Badge>
      </div>
      <div className="mt-2 break-all font-mono text-xs">
        {side.status === "available" ? side.value : t("snapshot.unavailable")}
      </div>
      {side.status === "incompatible" ? (
        <div className="mt-2 text-xs text-muted-foreground">
          {t("snapshot.actualType", {
            actual: side.actualType,
            expected: side.expectedTypes.join(", "),
          })}
        </div>
      ) : null}
    </div>
  );
}

function ComparisonError({ title }: { title: string }) {
  return (
    <Alert variant="destructive">
      <TriangleAlert aria-hidden="true" />
      <AlertTitle>{title}</AlertTitle>
    </Alert>
  );
}

function ComparisonSkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-72 max-w-full" />
      </CardHeader>
      <CardContent className="space-y-3">
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-32 w-full" />
      </CardContent>
    </Card>
  );
}
