import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  ArrowLeft,
  CheckCircle2,
  GitCompareArrows,
  Star,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router";

import {
  compareProbeSnapshots,
  listHistoryProbeGaps,
  listHistoryProbeSnapshots,
  setProbeSnapshotStarred,
  type ProbeHistoryGapPage,
  type ProbeSnapshotComparison,
  type ProbeSnapshotHistoryPage,
} from "@/api/history";
import { getProbeSnapshot, type ProbeSnapshot } from "@/api/probes";
import { useAuth } from "@/auth-context";
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
import { Slider } from "@/components/ui/slider";
import { formatAPIError } from "@/lib/api-error";
import {
  NavigationSourceLink,
  SourceAwareBackLink,
} from "@/lib/navigation-context";
import { cn } from "@/lib/utils";
import { formatTime } from "@/pages/node-probe-page";
import {
  SemanticProbeReport,
  type ProbeReportFieldMap,
} from "@/pages/probe-snapshot-page";

type TimelineSnapshot = ProbeSnapshotHistoryPage["items"][number];
type TimelineGap = ProbeHistoryGapPage["items"][number];

type TimelineState =
  | { kind: "loading" }
  | {
      kind: "success";
      egressId: string;
      snapshots: TimelineSnapshot[];
      gaps: TimelineGap[];
      details: Map<string, ProbeSnapshot>;
    }
  | { kind: "invalid" }
  | { kind: "error" };

type ComparisonState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "success"; comparison: ProbeSnapshotComparison }
  | { kind: "not-found" }
  | { kind: "mismatch" }
  | { kind: "error" };

const timelinePageSize = 100;

export function ProbeComparisonPage() {
  const { t, i18n } = useTranslation();
  const [search] = useSearchParams();
  const requestedEgress = search.get("egress") ?? "";
  const requestedBefore = search.get("before") ?? "";
  const requestedAfter = search.get("after") ?? "";
  const [timeline, setTimeline] = useState<TimelineState>({ kind: "loading" });
  const [selection, setSelection] = useState<[number, number]>([0, 1]);
  const [comparison, setComparison] = useState<ComparisonState>({
    kind: "idle",
  });

  const loadTimeline = useCallback(
    async (signal?: AbortSignal) => {
      setTimeline({ kind: "loading" });
      setComparison({ kind: "idle" });
      try {
        let egressId = requestedEgress;
        if (!egressId && requestedBefore && requestedAfter) {
          const bootstrap = await compareProbeSnapshots(
            requestedBefore,
            requestedAfter,
            signal,
          );
          egressId = bootstrap.egressId;
        }
        if (!egressId) {
          setTimeline({ kind: "invalid" });
          return;
        }

        const [snapshots, gaps] = await Promise.all([
          loadAllSnapshots(egressId, signal),
          loadAllGaps(egressId, signal),
        ]);
        snapshots.sort(compareTimelineSnapshots);
        const details = await loadAllSnapshotDetails(snapshots, signal);
        const initial = initialSelection(
          snapshots,
          requestedBefore,
          requestedAfter,
        );
        setSelection(initial);
        setTimeline({ kind: "success", egressId, snapshots, gaps, details });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setTimeline({ kind: "error" });
      }
    },
    [requestedAfter, requestedBefore, requestedEgress],
  );

  useEffect(() => {
    const controller = new AbortController();
    void loadTimeline(controller.signal);
    return () => controller.abort();
  }, [loadTimeline]);

  const resolved =
    timeline.kind === "success"
      ? resolveSelection(timeline.snapshots, selection)
      : undefined;
  const before = resolved?.before;
  const after = resolved?.after;
  const beforeDetails =
    timeline.kind === "success" && before
      ? timeline.details.get(before.id)
      : undefined;
  const afterDetails =
    timeline.kind === "success" && after
      ? timeline.details.get(after.id)
      : undefined;

  useEffect(() => {
    if (!before || !after || before.id === after.id) {
      setComparison({ kind: "idle" });
      return;
    }
    const controller = new AbortController();
    setComparison({ kind: "loading" });
    compareProbeSnapshots(before.id, after.id, controller.signal)
      .then((result) => setComparison({ kind: "success", comparison: result }))
      .catch((error) => {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        const status =
          typeof error === "object" && error !== null && "status" in error
            ? error.status
            : undefined;
        setComparison(
          status === 404
            ? { kind: "not-found" }
            : status === 409
              ? { kind: "mismatch" }
              : { kind: "error" },
        );
      });
    return () => controller.abort();
  }, [after, before]);

  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="max-w-3xl">
        <Button variant="ghost" size="sm" asChild className="mb-3 -ml-3">
          <SourceAwareBackLink fallback="/history?tab=reports">
            <ArrowLeft data-icon="inline-start" aria-hidden="true" />
            {t("common.back")}
          </SourceAwareBackLink>
        </Button>
        <p className="text-sm font-medium text-muted-foreground uppercase">
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
        {timeline.kind === "loading" ? <ComparisonSkeleton /> : null}
        {timeline.kind === "invalid" ? (
          <ComparisonError title={t("comparison.invalid")} />
        ) : null}
        {timeline.kind === "error" ? (
          <RetryError
            title={t("comparison.timeline.loadFailed")}
            retry={loadTimeline}
          />
        ) : null}
        {timeline.kind === "success" ? (
          timeline.snapshots.length < 2 ? (
            <ComparisonError title={t("comparison.timeline.insufficient")} />
          ) : (
            <>
              <ComparisonTimeline
                snapshots={timeline.snapshots}
                gaps={timeline.gaps}
                selection={selection}
                language={i18n.resolvedLanguage}
                onChange={setSelection}
                onCommit={(value) => {
                  setSelection(value);
                  const next = resolveSelection(timeline.snapshots, value);
                  if (!next) return;
                  const params = new URLSearchParams(search);
                  params.set("before", next.before.id);
                  params.set("after", next.after.id);
                  window.history.replaceState({}, "", `?${params.toString()}`);
                }}
              />
              {comparison.kind === "not-found" ? (
                <ComparisonError title={t("comparison.notFound")} />
              ) : null}
              {comparison.kind === "mismatch" ? (
                <ComparisonError title={t("comparison.egressMismatch")} />
              ) : null}
              {comparison.kind === "error" ? (
                <ComparisonError title={t("comparison.loadFailed")} />
              ) : null}
              {before && after && beforeDetails && afterDetails ? (
                <ComparisonReports
                  before={before}
                  after={after}
                  beforeDetails={beforeDetails}
                  afterDetails={afterDetails}
                  comparison={
                    comparison.kind === "success"
                      ? comparison.comparison
                      : undefined
                  }
                  comparisonLoading={comparison.kind === "loading"}
                  language={i18n.resolvedLanguage}
                  updateTimeline={setTimeline}
                />
              ) : null}
            </>
          )
        ) : null}
      </div>
    </main>
  );
}

async function loadAllSnapshots(egressId: string, signal?: AbortSignal) {
  const items: TimelineSnapshot[] = [];
  let page = 1;
  let total = 0;
  do {
    const result = await listHistoryProbeSnapshots(
      { egressId, page, pageSize: timelinePageSize },
      signal,
    );
    items.push(...result.items);
    total = result.total;
    page += 1;
    if (result.items.length === 0) break;
  } while (items.length < total);
  return items;
}

async function loadAllGaps(egressId: string, signal?: AbortSignal) {
  const items: TimelineGap[] = [];
  let page = 1;
  let total = 0;
  do {
    const result = await listHistoryProbeGaps(
      { egressId, page, pageSize: timelinePageSize },
      signal,
    );
    items.push(...result.items);
    total = result.total;
    page += 1;
    if (result.items.length === 0) break;
  } while (items.length < total);
  return items;
}

async function loadAllSnapshotDetails(
  snapshots: TimelineSnapshot[],
  signal?: AbortSignal,
) {
  const details = new Map<string, ProbeSnapshot>();
  let nextIndex = 0;
  const worker = async () => {
    while (nextIndex < snapshots.length) {
      const index = nextIndex;
      nextIndex += 1;
      const snapshot = await getProbeSnapshot(snapshots[index].id, signal);
      details.set(snapshot.id, snapshot);
    }
  };
  await Promise.all(
    Array.from({ length: Math.min(4, snapshots.length) }, () => worker()),
  );
  return details;
}

function compareTimelineSnapshots(
  left: TimelineSnapshot,
  right: TimelineSnapshot,
) {
  const time = Date.parse(left.observedAt) - Date.parse(right.observedAt);
  return (
    time || left.sequence - right.sequence || left.id.localeCompare(right.id)
  );
}

function snapshotAxisValue(snapshot: TimelineSnapshot, index: number) {
  return Date.parse(snapshot.observedAt) * 1000 + index;
}

function snapshotAxisValues(snapshots: TimelineSnapshot[]) {
  return snapshots.map((snapshot, index) => snapshotAxisValue(snapshot, index));
}

function initialSelection(
  snapshots: TimelineSnapshot[],
  beforeID: string,
  afterID: string,
): [number, number] {
  const axis = snapshotAxisValues(snapshots);
  const beforeIndex = snapshots.findIndex(
    (snapshot) => snapshot.id === beforeID,
  );
  const afterIndex = snapshots.findIndex((snapshot) => snapshot.id === afterID);
  if (beforeIndex >= 0 && afterIndex >= 0) {
    return [
      Math.min(axis[beforeIndex], axis[afterIndex]),
      Math.max(axis[beforeIndex], axis[afterIndex]),
    ];
  }
  return [axis[0] ?? 0, axis[axis.length - 1] ?? axis[0] ?? 0];
}

function resolveSelection(
  snapshots: TimelineSnapshot[],
  selection: [number, number],
) {
  if (snapshots.length === 0) return undefined;
  const axis = snapshotAxisValues(snapshots);
  const resolve = (cursor: number) => {
    let index = 0;
    for (let candidate = 0; candidate < axis.length; candidate += 1) {
      if (axis[candidate] > cursor) break;
      index = candidate;
    }
    return snapshots[index];
  };
  const before = resolve(selection[0]);
  const after = resolve(selection[1]);
  return before && after ? { before, after } : undefined;
}

function ComparisonTimeline({
  snapshots,
  gaps,
  selection,
  language,
  onChange,
  onCommit,
}: {
  snapshots: TimelineSnapshot[];
  gaps: TimelineGap[];
  selection: [number, number];
  language?: string;
  onChange: (value: [number, number]) => void;
  onCommit: (value: [number, number]) => void;
}) {
  const { t } = useTranslation();
  const resolved = resolveSelection(snapshots, selection);
  const start = resolved?.before ?? snapshots[0];
  const end = resolved?.after ?? snapshots[snapshots.length - 1];
  const axis = snapshotAxisValues(snapshots);
  const min = axis[0] ?? 0;
  const max = axis[axis.length - 1] ?? min;
  const denominator = Math.max(1, max - min);
  return (
    <Card
      size="sm"
      data-testid="comparison-timeline"
      className="sticky top-16 z-10 gap-2 bg-card/95 shadow-lg backdrop-blur supports-[backdrop-filter]:bg-card/90"
    >
      <CardHeader className="gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:grid-rows-1 sm:items-center">
        <div className="min-w-0">
          <CardTitle className="flex items-center gap-2">
            <GitCompareArrows className="size-4 shrink-0" aria-hidden="true" />
            {t("comparison.timeline.title")}
          </CardTitle>
          <CardDescription className="truncate text-sm">
            {t("comparison.timeline.owner", {
              node: snapshots[0].owner.nodeName ?? t("probe.notAvailable"),
              egress: snapshots[0].owner.egressName ?? t("probe.notAvailable"),
            })}
          </CardDescription>
        </div>
        <div className="flex flex-wrap gap-1.5 sm:justify-end">
          <Badge variant="secondary">
            {t("comparison.timeline.snapshotCount", {
              count: snapshots.length,
            })}
          </Badge>
          {gaps.length > 0 ? (
            <Badge variant="destructive">
              {t("comparison.timeline.gapCount", { count: gaps.length })}
            </Badge>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-2.5">
        <div className="grid grid-cols-2 gap-3 text-sm">
          <TimelineSelection
            label={t("comparison.start")}
            snapshot={start}
            language={language}
          />
          <TimelineSelection
            label={t("comparison.end")}
            snapshot={end}
            language={language}
            align="right"
          />
        </div>
        <div className="px-2 py-1.5">
          <div className="relative">
            <div className="pointer-events-none absolute inset-x-0 top-1/2 h-5 -translate-y-1/2">
              {snapshots.map((snapshot, index) => (
                <span
                  key={snapshot.id}
                  className={cn(
                    "absolute top-1/2 size-2 -translate-x-1/2 -translate-y-1/2 rounded-full border border-background bg-muted-foreground",
                    snapshot.starred && "bg-amber-500",
                  )}
                  style={{
                    left: `${((axis[index] - min) / denominator) * 100}%`,
                  }}
                  title={formatTime(snapshot.observedAt, language, "")}
                />
              ))}
              {gapPositions(snapshots, gaps).map((position, index) => (
                <TriangleAlert
                  key={`${position}-${index}`}
                  className="absolute top-1/2 size-3 -translate-x-1/2 -translate-y-1/2 fill-background text-destructive"
                  style={{ left: `${position}%` }}
                  aria-label={t("comparison.timeline.gap")}
                />
              ))}
            </div>
            <div className="pointer-events-none absolute inset-x-0 top-1/2 z-10 h-5 -translate-y-1/2">
              {snapshots.map((snapshot, index) => (
                <button
                  key={`${snapshot.id}-selector`}
                  type="button"
                  className="pointer-events-auto absolute top-1/2 size-5 -translate-x-1/2 -translate-y-1/2 rounded-full focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-hidden"
                  style={{
                    left: `${((axis[index] - min) / denominator) * 100}%`,
                  }}
                  aria-label={t("comparison.timeline.selectSnapshot", {
                    value: formatTime(snapshot.observedAt, language, ""),
                    sequence: snapshot.sequence,
                  })}
                  title={formatTime(snapshot.observedAt, language, "")}
                  onClick={() =>
                    onCommit(selectTimelinePoint(selection, axis[index]))
                  }
                />
              ))}
            </div>
            <Slider
              value={selection}
              min={min}
              max={max}
              step={1}
              minStepsBetweenThumbs={1}
              thumbLabels={[t("comparison.start"), t("comparison.end")]}
              onValueChange={(value) => onChange(asSelection(value))}
              onValueCommit={(value) => onCommit(asSelection(value))}
            />
          </div>
        </div>
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-muted-foreground" />
            {t("comparison.timeline.snapshot")}
          </span>
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-amber-500" />
            {t("comparison.timeline.starred")}
          </span>
          {gaps.length > 0 ? (
            <span className="flex items-center gap-1.5">
              <TriangleAlert
                className="size-3 text-destructive"
                aria-hidden="true"
              />
              {t("comparison.timeline.gap")}
            </span>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

function TimelineSelection({
  label,
  snapshot,
  language,
  align = "left",
}: {
  label: string;
  snapshot: TimelineSnapshot;
  language?: string;
  align?: "left" | "right";
}) {
  return (
    <div className={cn("min-w-0", align === "right" && "text-right")}>
      <div className="text-muted-foreground">{label}</div>
      <div
        className={cn(
          "mt-0.5 flex min-w-0 items-center gap-1.5 font-medium tabular-nums",
          align === "right" && "justify-end",
        )}
      >
        <time dateTime={snapshot.observedAt} className="truncate">
          {formatTime(snapshot.observedAt, language, "")}
        </time>
        {snapshot.starred ? (
          <Star
            className="size-3 shrink-0 fill-amber-500 text-amber-500"
            aria-hidden="true"
          />
        ) : null}
      </div>
    </div>
  );
}

function ComparisonReports({
  before,
  after,
  beforeDetails,
  afterDetails,
  comparison,
  comparisonLoading,
  language,
  updateTimeline,
}: {
  before: TimelineSnapshot;
  after: TimelineSnapshot;
  beforeDetails: ProbeSnapshot;
  afterDetails: ProbeSnapshot;
  comparison?: ProbeSnapshotComparison;
  comparisonLoading: boolean;
  language?: string;
  updateTimeline: React.Dispatch<React.SetStateAction<TimelineState>>;
}) {
  const { t } = useTranslation();
  const { state: authState } = useAuth();
  const [saving, setSaving] = useState<string>();
  const [feedback, setFeedback] = useState<string>();
  const changedPaths = useMemo(
    () =>
      new Set(
        comparison?.fields
          .filter((field) => field.changed)
          .map((field) => field.path) ?? [],
      ),
    [comparison],
  );
  const beforeFields = useMemo(
    () => new Map(beforeDetails.fields.map((field) => [field.path, field])),
    [beforeDetails],
  );
  const afterFields = useMemo(
    () => new Map(afterDetails.fields.map((field) => [field.path, field])),
    [afterDetails],
  );
  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function toggleStar(snapshot: TimelineSnapshot) {
    setSaving(snapshot.id);
    setFeedback(undefined);
    try {
      const updated = await setProbeSnapshotStarred(
        snapshot.id,
        !snapshot.starred,
        csrfToken,
      );
      updateTimeline((current) =>
        current.kind === "success"
          ? {
              ...current,
              snapshots: current.snapshots.map((item) =>
                item.id === snapshot.id
                  ? { ...item, starred: updated.starred }
                  : item,
              ),
            }
          : current,
      );
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setSaving(undefined);
    }
  }

  return (
    <div className="space-y-4">
      {changedPaths.size === 0 ? (
        <Alert>
          <CheckCircle2 aria-hidden="true" />
          <AlertTitle>
            {comparisonLoading
              ? t("comparison.comparing")
              : t("comparison.noChanges")}
          </AlertTitle>
        </Alert>
      ) : (
        <Alert>
          <GitCompareArrows aria-hidden="true" />
          <AlertTitle>
            {t("comparison.changeCount", { count: changedPaths.size })}
          </AlertTitle>
          <AlertDescription>{t("comparison.highlightDetail")}</AlertDescription>
        </Alert>
      )}
      {feedback ? (
        <Alert variant="destructive">
          <TriangleAlert aria-hidden="true" />
          <AlertDescription>{feedback}</AlertDescription>
        </Alert>
      ) : null}
      <div className="grid min-w-0 gap-6 xl:grid-cols-2">
        <ComparisonReport
          label={t("comparison.start")}
          snapshot={before}
          fields={beforeFields}
          changedPaths={changedPaths}
          language={language}
          saving={saving === before.id}
          toggleStar={() => void toggleStar(before)}
        />
        <ComparisonReport
          label={t("comparison.end")}
          snapshot={after}
          fields={afterFields}
          changedPaths={changedPaths}
          language={language}
          saving={saving === after.id}
          toggleStar={() => void toggleStar(after)}
        />
      </div>
    </div>
  );
}

function ComparisonReport({
  label,
  snapshot,
  fields,
  changedPaths,
  language,
  saving,
  toggleStar,
}: {
  label: string;
  snapshot: TimelineSnapshot;
  fields: ProbeReportFieldMap;
  changedPaths: Set<string>;
  language?: string;
  saving: boolean;
  toggleStar: () => void;
}) {
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const nodes =
      container.current?.querySelectorAll<HTMLElement>("[data-report-path]");
    nodes?.forEach((node) => {
      const paths = (node.dataset.reportPath ?? "").split(" ");
      node.toggleAttribute(
        "data-report-changed",
        paths.some((path) => changedPaths.has(path)),
      );
    });
  }, [changedPaths, fields]);
  const address = reportFieldValue(fields, "Head.IP");
  return (
    <section ref={container} aria-label={label} className="min-w-0 space-y-4">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b pb-4">
        <div className="min-w-0">
          <div className="text-sm font-medium text-muted-foreground uppercase">
            {label}
          </div>
          <NavigationSourceLink
            to={`/probe-snapshots/${snapshot.id}?runId=${snapshot.runId}`}
            className="mt-1 block font-semibold underline-offset-4 hover:underline"
          >
            {formatTime(snapshot.observedAt, language, "")}
          </NavigationSourceLink>
          <div
            data-report-path="Head.IP"
            className="mt-1 break-all text-sm text-muted-foreground"
          >
            {address}
          </div>
          <dl className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
            <ReportHeaderFact
              label={t("snapshot.report.overview.version")}
              value={reportFieldValue(fields, "Head.Version")}
              path="Head.Version"
            />
            <ReportHeaderFact
              label={t("snapshot.report.overview.upstreamTime")}
              value={reportFieldValue(fields, "Head.Time")}
              path="Head.Time"
            />
          </dl>
        </div>
        <Button
          variant={snapshot.starred ? "secondary" : "outline"}
          size="sm"
          disabled={saving}
          onClick={toggleStar}
        >
          <Star
            data-icon="inline-start"
            aria-hidden="true"
            className={snapshot.starred ? "fill-current" : undefined}
          />
          {t(snapshot.starred ? "snapshot.unstar" : "snapshot.star")}
        </Button>
      </div>
      <div className="[&_[data-report-changed]]:rounded-md [&_[data-report-changed]]:bg-amber-500/10 [&_[data-report-changed]]:ring-2 [&_[data-report-changed]]:ring-amber-500/70 [&_[data-report-changed]]:ring-offset-2 [&_[data-report-changed]]:ring-offset-background [&_[data-slot=card]]:gap-3 [&_[data-slot=card]]:py-3 [&_[data-slot=card-content]]:px-3 [&_[data-slot=card-header]]:px-3 [&_[data-slot=table-cell]]:px-1 [&_[data-slot=table-head]]:px-1 [&_table]:text-sm">
        <SemanticProbeReport fields={fields} compact />
      </div>
    </section>
  );
}

function reportFieldValue(fields: ProbeReportFieldMap, path: string) {
  const value = fields.get(path)?.value?.trim();
  return value || "—";
}

function ReportHeaderFact({
  label,
  value,
  path,
}: {
  label: string;
  value: string;
  path: string;
}) {
  if (value === "—") return null;
  return (
    <div data-report-path={path} className="flex gap-1">
      <dt>{label}:</dt>
      <dd>{value}</dd>
    </div>
  );
}

function gapPositions(snapshots: TimelineSnapshot[], gaps: TimelineGap[]) {
  if (snapshots.length < 2) return [];
  const axis = snapshotAxisValues(snapshots);
  const min = axis[0] ?? 0;
  const max = axis[axis.length - 1] ?? min;
  const denominator = Math.max(1, max - min);
  return gaps.flatMap((gap) => {
    const next = snapshots.findIndex(
      (snapshot) => snapshot.sequence > gap.lastSequence,
    );
    if (next <= 0) return [];
    const previous = axis[next - 1] ?? min;
    const current = axis[next] ?? previous;
    return [(((previous + current) / 2 - min) / denominator) * 100];
  });
}

function selectTimelinePoint(
  selection: [number, number],
  point: number,
): [number, number] {
  const startDistance = Math.abs(selection[0] - point);
  const endDistance = Math.abs(selection[1] - point);
  if (point <= selection[1] && startDistance <= endDistance) {
    return [point, selection[1]];
  }
  return [selection[0], point];
}

function asSelection(value: number[]): [number, number] {
  return [value[0] ?? 0, value[1] ?? value[0] ?? 0];
}

function RetryError({
  title,
  retry,
}: {
  title: string;
  retry: () => void | Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <Alert variant="destructive">
      <TriangleAlert aria-hidden="true" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        <Button
          variant="outline"
          size="sm"
          className="mt-3"
          onClick={() => void retry()}
        >
          {t("comparison.retry")}
        </Button>
      </AlertDescription>
    </Alert>
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
      <CardContent className="space-y-4">
        <Skeleton className="h-16 w-full" />
        <Skeleton className="h-5 w-full" />
      </CardContent>
    </Card>
  );
}
