import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { flushSync } from "react-dom";
import {
  ArrowLeft,
  Check,
  Clipboard,
  Download,
  FileJson2,
  Gauge,
  GitCompareArrows,
  Globe2,
  ImageDown,
  LoaderCircle,
  Mail,
  Shield,
  Star,
  Tags,
  TriangleAlert,
  Tv,
  WrapText,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams, useSearchParams } from "react-router";

import { setProbeSnapshotStarred } from "@/api/history";
import { getProbeSnapshot, type ProbeSnapshot } from "@/api/probes";
import { useAuth } from "@/auth-context";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatAPIError } from "@/lib/api-error";
import { presentProbeField } from "@/lib/probe-field-label";
import { cn } from "@/lib/utils";
import { formatTime } from "@/pages/node-probe-page";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; snapshot: ProbeSnapshot; raw: string }
  | { kind: "not-found" }
  | { kind: "error" };

type KnownField = ProbeSnapshot["fields"][number];

const reportExportWidth = 1200;

export function ProbeSnapshotPage() {
  const { snapshotId = "" } = useParams();
  const [search] = useSearchParams();
  const { t, i18n } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [wrap, setWrap] = useState(true);
  const [copied, setCopied] = useState(false);
  const [starring, setStarring] = useState(false);
  const [feedback, setFeedback] = useState<string>();

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setState({ kind: "loading" });
      try {
        const snapshot = await getProbeSnapshot(snapshotId, signal);
        setState({
          kind: "success",
          snapshot,
          raw: decodeProbeResult(snapshot.rawResult),
        });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        const status =
          typeof error === "object" && error !== null && "status" in error
            ? error.status
            : undefined;
        setState(status === 404 ? { kind: "not-found" } : { kind: "error" });
      }
    },
    [snapshotId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const display = useMemo(() => {
    if (state.kind !== "success") return "";
    try {
      return JSON.stringify(JSON.parse(state.raw), null, 2);
    } catch {
      return state.raw;
    }
  }, [state]);
  const runId = search.get("runId");
  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function copy() {
    if (state.kind !== "success") return;
    try {
      await navigator.clipboard.writeText(state.raw);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  }

  function download() {
    if (state.kind !== "success") return;
    const url = URL.createObjectURL(
      new Blob([state.raw], { type: "application/json" }),
    );
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `ipchronicle-${state.snapshot.id}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  }

  async function toggleStar() {
    if (state.kind !== "success") return;
    setStarring(true);
    setFeedback(undefined);
    try {
      const snapshot = await setProbeSnapshotStarred(
        state.snapshot.id,
        !state.snapshot.starred,
        csrfToken,
      );
      setState((current) =>
        current.kind === "success"
          ? { ...current, snapshot, raw: current.raw }
          : current,
      );
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setStarring(false);
    }
  }

  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0 max-w-2xl">
          <Button variant="ghost" size="sm" asChild className="mb-3 -ml-3">
            <Link to={runId ? `/probe-runs/${runId}` : "/history"}>
              <ArrowLeft data-icon="inline-start" aria-hidden="true" />
              {t("snapshot.back")}
            </Link>
          </Button>
          <p className="text-sm font-medium text-muted-foreground uppercase">
            {t("snapshot.section")}
          </p>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
            {t("snapshot.title")}
          </h1>
          <p className="mt-2 break-all text-sm text-muted-foreground">
            {snapshotId}
          </p>
        </div>
        {state.kind === "success" ? (
          <Button
            variant={state.snapshot.starred ? "secondary" : "outline"}
            onClick={() => void toggleStar()}
            disabled={starring}
          >
            <Star
              data-icon="inline-start"
              aria-hidden="true"
              className={state.snapshot.starred ? "fill-current" : undefined}
            />
            {t(state.snapshot.starred ? "snapshot.unstar" : "snapshot.star")}
          </Button>
        ) : null}
      </div>

      <div className="mt-8 space-y-4" aria-live="polite">
        {state.kind === "loading" ? <SnapshotSkeleton /> : null}
        {state.kind === "not-found" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("snapshot.notFound")}</AlertTitle>
          </Alert>
        ) : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("snapshot.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => void load()}
              >
                {t("snapshot.retry")}
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
          <SnapshotResult
            snapshot={state.snapshot}
            display={display}
            wrap={wrap}
            copied={copied}
            language={i18n.resolvedLanguage}
            setWrap={setWrap}
            copy={copy}
            download={download}
          />
        ) : null}
      </div>
    </main>
  );
}

function SnapshotResult({
  snapshot,
  display,
  wrap,
  copied,
  language,
  setWrap,
  copy,
  download,
}: {
  snapshot: ProbeSnapshot;
  display: string;
  wrap: boolean;
  copied: boolean;
  language?: string;
  setWrap: (value: boolean) => void;
  copy: () => Promise<void>;
  download: () => void;
}) {
  const { t } = useTranslation();
  const fieldsByPath = useMemo(
    () => new Map(snapshot.fields.map((field) => [field.path, field])),
    [snapshot.fields],
  );
  const [view, setView] = useState<"report" | "raw" | "diagnostics">("report");
  const [pngOperation, setPNGOperation] = useState<"copy" | "export">();
  const [pngCopied, setPNGCopied] = useState(false);
  const [pngFailure, setPNGFailure] = useState<"copy" | "export">();
  const exportRef = useRef<HTMLDivElement>(null);

  async function exportPNG() {
    if (pngOperation) return;

    flushSync(() => setPNGOperation("export"));
    setPNGFailure(undefined);
    try {
      const target = exportRef.current;
      if (!target) throw new Error("PNG export target is unavailable");
      const blob = await renderReportPNG(target);
      downloadBlob(blob, `ipchronicle-${snapshot.id}.png`);
    } catch {
      setPNGFailure("export");
    } finally {
      setPNGOperation(undefined);
    }
  }

  async function copyPNG() {
    if (pngOperation) return;

    flushSync(() => setPNGOperation("copy"));
    setPNGFailure(undefined);
    setPNGCopied(false);
    try {
      const target = exportRef.current;
      if (!target || !navigator.clipboard?.write || !window.ClipboardItem) {
        throw new Error("PNG clipboard is unavailable");
      }

      const blob = renderReportPNG(target);
      await navigator.clipboard.write([
        new ClipboardItem({ "image/png": blob }),
      ]);
      setPNGCopied(true);
      window.setTimeout(() => setPNGCopied(false), 1500);
    } catch {
      setPNGFailure("copy");
    } finally {
      setPNGOperation(undefined);
    }
  }

  return (
    <>
      <SnapshotOverviewCard
        snapshot={snapshot}
        fields={fieldsByPath}
        language={language}
        showComparison
      />

      <Tabs value={view} onValueChange={(next) => setView(next as typeof view)}>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <TabsList
            className="h-auto flex-wrap"
            aria-label={t("snapshot.views.label")}
          >
            <TabsTrigger value="report">
              {t("snapshot.views.report")}
            </TabsTrigger>
            <TabsTrigger value="raw">{t("snapshot.views.raw")}</TabsTrigger>
            {snapshot.formatIssues.length > 0 ? (
              <TabsTrigger value="diagnostics">
                {t("snapshot.views.diagnostics", {
                  count: snapshot.formatIssues.length,
                })}
              </TabsTrigger>
            ) : null}
          </TabsList>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={Boolean(pngOperation)}
              onClick={() => void copyPNG()}
            >
              {pngOperation === "copy" ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : pngCopied ? (
                <Check data-icon="inline-start" aria-hidden="true" />
              ) : (
                <Clipboard data-icon="inline-start" aria-hidden="true" />
              )}
              {t(
                pngOperation === "copy"
                  ? "snapshot.pngExport.copying"
                  : pngCopied
                    ? "snapshot.pngExport.copied"
                    : "snapshot.pngExport.copy",
              )}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={Boolean(pngOperation)}
              onClick={() => void exportPNG()}
            >
              {pngOperation === "export" ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <ImageDown data-icon="inline-start" aria-hidden="true" />
              )}
              {t(
                pngOperation === "export"
                  ? "snapshot.pngExport.exporting"
                  : "snapshot.pngExport.action",
              )}
            </Button>
          </div>
        </div>
        {pngFailure ? (
          <Alert variant="destructive" className="mt-3">
            <TriangleAlert aria-hidden="true" />
            <AlertDescription>
              {t(
                pngFailure === "copy"
                  ? "snapshot.pngExport.copyFailed"
                  : "snapshot.pngExport.failed",
              )}
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="pt-2">
          {view === "report" ? (
            <SemanticProbeReport fields={fieldsByPath} />
          ) : null}
          {view === "diagnostics" ? (
            <FormatDiagnostics
              issues={snapshot.formatIssues}
              fields={fieldsByPath}
            />
          ) : null}
          {view === "raw" ? (
            <RawJSONCard
              snapshot={snapshot}
              display={display}
              wrap={wrap}
              copied={copied}
              language={language}
              setWrap={setWrap}
              copy={copy}
              download={download}
            />
          ) : null}
        </div>
      </Tabs>

      {pngOperation ? (
        <div
          ref={exportRef}
          aria-hidden="true"
          inert
          className="pointer-events-none fixed top-0 left-[-10000px] w-[1200px] bg-background p-8 text-foreground [&_[data-slot=table-container]]:overflow-visible"
        >
          <div className="mb-5 flex items-end justify-between gap-6 border-b pb-4">
            <div>
              <div className="text-xl font-semibold">IPChronicle</div>
              <div className="mt-1 text-sm text-muted-foreground">
                {t("snapshot.title")}
              </div>
            </div>
            <div className="max-w-xl break-all text-right font-mono text-sm text-muted-foreground">
              {snapshot.id}
            </div>
          </div>
          <SnapshotOverviewCard
            snapshot={snapshot}
            fields={fieldsByPath}
            language={language}
            exportMode
          />
          <div className="mt-4">
            <SemanticProbeReport fields={fieldsByPath} exportMode />
          </div>
        </div>
      ) : null}
    </>
  );
}

function SnapshotOverviewCard({
  snapshot,
  fields,
  language,
  showComparison = false,
  exportMode = false,
}: {
  snapshot: ProbeSnapshot;
  fields: FieldMap;
  language?: string;
  showComparison?: boolean;
  exportMode?: boolean;
}) {
  const { t } = useTranslation();
  const address = fieldText(fields, "Head.IP");
  const version = fieldText(fields, "Head.Version");
  const upstreamTime = fieldText(fields, "Head.Time");
  const canCompare = showComparison;

  return (
    <Card>
      <CardHeader
        className={cn(
          canCompare && "sm:grid-cols-[1fr_auto] sm:grid-rows-[auto_auto]",
        )}
      >
        <CardTitle className="break-all text-2xl">
          {address ?? t("snapshot.report.overview.unknownAddress")}
        </CardTitle>
        <CardDescription>
          {t("snapshot.report.overview.detail", {
            sequence: snapshot.sequence,
            value: formatTime(
              snapshot.observedAt,
              language,
              t("probe.notAvailable"),
            ),
          })}
        </CardDescription>
        {canCompare ? (
          <div className="col-span-full mt-2 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:mt-0 sm:justify-self-end">
            <Button variant="outline" size="sm" asChild>
              <Link to={`/history/compare?egress=${snapshot.egressId}`}>
                <GitCompareArrows data-icon="inline-start" aria-hidden="true" />
                {t("snapshot.compare")}
              </Link>
            </Button>
          </div>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap gap-2">
          {snapshot.baseline ? (
            <Badge variant="outline">{t("snapshot.summary.baseline")}</Badge>
          ) : null}
          {snapshot.changes.length > 0 ? (
            <Badge>
              {t("snapshot.summary.changes", {
                count: snapshot.changes.length,
              })}
            </Badge>
          ) : (
            <Badge
              variant="outline"
              className="border-emerald-600/40 text-emerald-700 dark:text-emerald-400"
            >
              {t("snapshot.summary.noChanges")}
            </Badge>
          )}
          {snapshot.formatIssues.length > 0 ? (
            <Badge variant="destructive">
              {t("snapshot.summary.formatIssues", {
                count: snapshot.formatIssues.length,
              })}
            </Badge>
          ) : (
            <Badge
              variant="outline"
              className="border-emerald-600/40 text-emerald-700 dark:text-emerald-400"
            >
              {t("snapshot.summary.compatible")}
            </Badge>
          )}
        </div>
        {version || upstreamTime ? (
          <div
            className={cn(
              "grid gap-3 border-t pt-4",
              exportMode ? "grid-cols-2" : "sm:grid-cols-2",
            )}
          >
            {version ? (
              <ReportFact
                label={t("snapshot.report.overview.version")}
                value={version}
              />
            ) : null}
            {upstreamTime ? (
              <ReportFact
                label={t("snapshot.report.overview.upstreamTime")}
                value={upstreamTime}
              />
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

export type ProbeReportFieldMap = Map<string, ProbeSnapshot["fields"][number]>;
type FieldMap = ProbeReportFieldMap;

const scoreProviders = [
  "IP2LOCATION",
  "SCAMALYTICS",
  "ipapi",
  "AbuseIPDB",
  "IPQS",
  "DBIP",
] as const;

const factorProviders = [
  "IP2LOCATION",
  "ipapi",
  "ipregistry",
  "IPQS",
  "SCAMALYTICS",
  "ipdata",
  "IPinfo",
  "IPWHOIS",
  "DBIP",
] as const;

const typeProviders = [
  "IPinfo",
  "ipregistry",
  "ipapi",
  "IP2LOCATION",
  "AbuseIPDB",
] as const;

const riskFactors = [
  "Proxy",
  "Tor",
  "VPN",
  "Server",
  "Abuser",
  "Robot",
] as const;

const mediaServices = [
  "TikTok",
  "DisneyPlus",
  "Netflix",
  "Youtube",
  "AmazonPrimeVideo",
  "Reddit",
  "ChatGPT",
] as const;

const mailServices = [
  "Gmail",
  "Outlook",
  "Yahoo",
  "Apple",
  "QQ",
  "MailRU",
  "AOL",
  "GMX",
  "MailCOM",
  "163",
  "Sohu",
  "Sina",
] as const;

export function SemanticProbeReport({
  fields,
  exportMode = false,
  compact = false,
}: {
  fields: FieldMap;
  exportMode?: boolean;
  compact?: boolean;
}) {
  return (
    <div className="space-y-4">
      <BasicInformationCard fields={fields} exportMode={exportMode} />
      <TypeClassificationCard fields={fields} compact={compact} />
      <RiskScoresCard fields={fields} exportMode={exportMode} />
      <RiskFactorsCard fields={fields} compact={compact} />
      <MediaServicesCard fields={fields} compact={compact} />
      <MailCard fields={fields} exportMode={exportMode} />
    </div>
  );
}

function BasicInformationCard({
  fields,
  exportMode = false,
}: {
  fields: FieldMap;
  exportMode?: boolean;
}) {
  const { t } = useTranslation();
  const location = joinDistinct([
    fieldText(fields, "Info.Region.Name"),
    fieldText(fields, "Info.City.Name"),
  ]);
  const registeredRegion = joinCodeAndName(
    fieldText(fields, "Info.RegisteredRegion.Code"),
    fieldText(fields, "Info.RegisteredRegion.Name"),
  );
  const reportRegion = joinCodeAndName(
    fieldText(fields, "Info.Region.Code"),
    location,
  );
  const facts: Array<[string, string | undefined, ReportTone, string]> = [
    [
      t("snapshot.report.basic.asn"),
      fieldText(fields, "Info.ASN"),
      "green",
      "Info.ASN",
    ],
    [
      t("snapshot.report.basic.organization"),
      fieldText(fields, "Info.Organization"),
      "green",
      "Info.Organization",
    ],
    [
      t("snapshot.report.basic.location"),
      reportRegion,
      "green",
      "Info.Region.Code Info.Region.Name Info.City.Name",
    ],
    [
      t("snapshot.report.basic.registeredRegion"),
      registeredRegion,
      "green",
      "Info.RegisteredRegion.Code Info.RegisteredRegion.Name",
    ],
    [
      t("snapshot.report.basic.timezone"),
      fieldText(fields, "Info.TimeZone"),
      "green",
      "Info.TimeZone",
    ],
    [
      t("snapshot.report.basic.type"),
      fieldText(fields, "Info.Type"),
      basicTypeTone(fieldText(fields, "Info.Type")),
      "Info.Type",
    ],
  ];
  const availableFacts = facts.filter(
    (item): item is [string, string, ReportTone, string] => Boolean(item[1]),
  );

  return (
    <ReportCard
      icon={<Globe2 aria-hidden="true" />}
      title={t("snapshot.report.basic.title")}
      detail={t("snapshot.report.basic.detail")}
    >
      {availableFacts.length > 0 ? (
        <div
          className={cn(
            "grid gap-x-8 gap-y-4",
            exportMode ? "grid-cols-3" : "sm:grid-cols-2 lg:grid-cols-3",
          )}
        >
          {availableFacts.map(([label, value, tone, path]) => (
            <ReportFact
              key={label}
              label={label}
              value={value}
              tone={tone}
              path={path}
            />
          ))}
        </div>
      ) : (
        <ReportEmpty />
      )}
    </ReportCard>
  );
}

function TypeClassificationCard({
  fields,
  compact = false,
}: {
  fields: FieldMap;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const hasValues = typeProviders.some(
    (provider) =>
      fieldText(fields, `Type.Usage.${provider}`) ||
      fieldText(fields, `Type.Company.${provider}`) ||
      fieldUnavailable(fields, `Type.Usage.${provider}`) ||
      fieldUnavailable(fields, `Type.Company.${provider}`),
  );

  return (
    <ReportCard
      icon={<Tags aria-hidden="true" />}
      title={t("snapshot.report.type.title")}
      detail={t("snapshot.report.type.detail")}
    >
      {hasValues ? (
        <Table
          className={cn(
            "table-fixed",
            compact ? "min-w-[560px]" : "min-w-[720px]",
          )}
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-32">
                {t("snapshot.report.type.database")}
              </TableHead>
              {typeProviders.map((provider) => (
                <TableHead key={provider} className="text-center">
                  {provider}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {(["Usage", "Company"] as const).map((field) => (
              <TableRow key={field}>
                <TableCell className="font-medium">
                  {t(`snapshot.report.type.${field.toLowerCase()}`)}
                </TableCell>
                {typeProviders.map((provider) => (
                  <TableCell key={provider} className="text-center">
                    <div data-report-path={`Type.${field}.${provider}`}>
                      <ClassificationValue
                        value={fieldText(fields, `Type.${field}.${provider}`)}
                        unavailable={fieldUnavailable(
                          fields,
                          `Type.${field}.${provider}`,
                        )}
                      />
                    </div>
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <ReportEmpty />
      )}
    </ReportCard>
  );
}

function RiskScoresCard({
  fields,
  exportMode = false,
}: {
  fields: FieldMap;
  exportMode?: boolean;
}) {
  const { t } = useTranslation();
  const scores = scoreProviders.flatMap((provider) => {
    const value = fieldText(fields, `Score.${provider}`);
    return value ? [{ provider, value, numeric: parseScore(value) }] : [];
  });

  return (
    <ReportCard
      icon={<Gauge aria-hidden="true" />}
      title={t("snapshot.report.scores.title")}
      detail={t("snapshot.report.scores.detail")}
    >
      {scores.length > 0 ? (
        <div className="space-y-4">
          {scores.map((score) => {
            const level = riskLevel(score.provider, score.numeric);
            return (
              <div
                key={score.provider}
                data-report-path={`Score.${score.provider}`}
                className={cn(
                  "grid min-w-0 gap-2",
                  exportMode
                    ? "grid-cols-[9rem_minmax(8rem,1fr)_7rem] items-center"
                    : "sm:grid-cols-[9rem_minmax(8rem,1fr)_7rem] sm:items-center",
                )}
              >
                <div className="font-medium">{score.provider}</div>
                <div className="h-2 overflow-hidden rounded-full bg-muted">
                  {score.numeric !== undefined ? (
                    <div
                      className={cn("h-full rounded-full", riskBarClass(level))}
                      style={{
                        width: `${Math.min(100, Math.max(0, score.numeric))}%`,
                      }}
                    />
                  ) : null}
                </div>
                <div className="flex items-center gap-2 sm:justify-end">
                  <span className="font-mono text-sm">{score.value}</span>
                  {level ? (
                    <Badge
                      variant={
                        riskTone(level) === "red" ? "destructive" : "outline"
                      }
                      className={riskLevelBadgeClass(level)}
                    >
                      {t(`snapshot.report.scores.level.${level}`)}
                    </Badge>
                  ) : null}
                </div>
              </div>
            );
          })}
          <p className="text-sm text-muted-foreground">
            {t("snapshot.report.scores.disclaimer")}
          </p>
        </div>
      ) : (
        <ReportEmpty />
      )}
    </ReportCard>
  );
}

function RiskFactorsCard({
  fields,
  compact = false,
}: {
  fields: FieldMap;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const rows = (["CountryCode", ...riskFactors] as const).map((factor) => ({
    factor,
    values: factorProviders.map((provider) => ({
      provider,
      unavailable: fieldUnavailable(fields, `Factor.${factor}.${provider}`),
      value:
        factor === "CountryCode"
          ? fieldText(fields, `Factor.CountryCode.${provider}`)
          : factorField(fields, `Factor.${factor}.${provider}`),
    })),
  }));
  const hasValues = rows.some((row) =>
    row.values.some((item) => item.value !== undefined || item.unavailable),
  );

  return (
    <ReportCard
      icon={<Shield aria-hidden="true" />}
      title={t("snapshot.report.factors.title")}
      detail={t("snapshot.report.factors.detail")}
    >
      {hasValues ? (
        <Table
          className={cn(
            "table-fixed",
            compact ? "min-w-[780px]" : "min-w-[1040px]",
          )}
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">
                {t("snapshot.report.factors.item")}
              </TableHead>
              {factorProviders.map((provider) => (
                <TableHead key={provider} className="text-center text-sm">
                  {provider}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.factor}>
                <TableCell className="font-medium">
                  {t(`snapshot.report.factors.names.${row.factor}`)}
                </TableCell>
                {row.values.map((item) => (
                  <TableCell key={item.provider} className="text-center">
                    <div
                      data-report-path={`Factor.${row.factor}.${item.provider}`}
                    >
                      {row.factor === "CountryCode" ? (
                        <ReportValueBadge
                          value={item.value as string}
                          tone="green"
                          unavailable={item.unavailable}
                        />
                      ) : (
                        <FactorMark
                          value={item.value as FactorSignal}
                          unavailable={item.unavailable}
                        />
                      )}
                    </div>
                  </TableCell>
                ))}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <ReportEmpty />
      )}
    </ReportCard>
  );
}

type FactorSignal = boolean | undefined;

function FactorMark({
  value,
  unavailable,
}: {
  value: FactorSignal;
  unavailable: boolean;
}) {
  const { t } = useTranslation();
  if (value === undefined) {
    return unavailable ? <UnavailableValue /> : null;
  }
  return (
    <Badge
      variant={value === true ? "destructive" : "outline"}
      className={cn(
        "whitespace-nowrap",
        value !== true &&
          "border-emerald-600/40 text-emerald-700 dark:text-emerald-400",
      )}
    >
      {t(value ? "snapshot.report.factors.yes" : "snapshot.report.factors.no")}
    </Badge>
  );
}

function MediaServicesCard({
  fields,
  compact = false,
}: {
  fields: FieldMap;
  compact?: boolean;
}) {
  const { t } = useTranslation();
  const services = mediaServices.flatMap((service) => {
    const statusPath = `Media.${service}.Status`;
    const regionPath = `Media.${service}.Region`;
    const typePath = `Media.${service}.Type`;
    const status = fieldText(fields, statusPath);
    const region = fieldText(fields, regionPath);
    const type = fieldText(fields, typePath);
    const statusUnavailable = fieldUnavailable(fields, statusPath);
    const regionUnavailable = fieldUnavailable(fields, regionPath);
    const typeUnavailable = fieldUnavailable(fields, typePath);
    return status ||
      region ||
      type ||
      statusUnavailable ||
      regionUnavailable ||
      typeUnavailable
      ? [
          {
            service,
            status,
            region,
            type,
            statusUnavailable,
            regionUnavailable,
            typeUnavailable,
          },
        ]
      : [];
  });
  return (
    <ReportCard
      icon={<Tv aria-hidden="true" />}
      title={t("snapshot.report.media.title")}
      detail={t("snapshot.report.media.detail")}
    >
      {services.length > 0 ? (
        <Table
          className={cn(
            "table-fixed",
            compact ? "min-w-[680px]" : "min-w-[880px]",
          )}
        >
          <TableHeader>
            <TableRow>
              <TableHead className="w-28">
                {t("snapshot.report.media.item")}
              </TableHead>
              {services.map((service) => (
                <TableHead
                  key={service.service}
                  className="text-center text-sm"
                >
                  {service.service}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell className="font-medium">
                {t("snapshot.report.media.status")}
              </TableCell>
              {services.map((service) => (
                <TableCell key={service.service} className="text-center">
                  <div data-report-path={`Media.${service.service}.Status`}>
                    <MediaStatusBadge
                      value={service.status}
                      unavailable={service.statusUnavailable}
                    />
                  </div>
                </TableCell>
              ))}
            </TableRow>
            <TableRow>
              <TableCell className="font-medium">
                {t("snapshot.report.media.region")}
              </TableCell>
              {services.map((service) => (
                <TableCell key={service.service} className="text-center">
                  <div data-report-path={`Media.${service.service}.Region`}>
                    <ReportValueBadge
                      value={service.region}
                      tone="green"
                      unavailable={service.regionUnavailable}
                    />
                  </div>
                </TableCell>
              ))}
            </TableRow>
            <TableRow>
              <TableCell className="font-medium">
                {t("snapshot.report.media.method")}
              </TableCell>
              {services.map((service) => (
                <TableCell key={service.service} className="text-center">
                  <div data-report-path={`Media.${service.service}.Type`}>
                    <MediaValueBadge
                      value={service.type}
                      unavailable={service.typeUnavailable}
                    />
                  </div>
                </TableCell>
              ))}
            </TableRow>
          </TableBody>
        </Table>
      ) : (
        <ReportEmpty />
      )}
    </ReportCard>
  );
}

function MediaValueBadge({
  value,
  unavailable,
}: {
  value?: string;
  unavailable: boolean;
}) {
  if (!value) return unavailable ? <UnavailableValue /> : null;
  return <ReportValueBadge value={value} tone={mediaTone(value)} />;
}

function MediaStatusBadge({
  value,
  unavailable,
}: {
  value?: string;
  unavailable: boolean;
}) {
  const { t } = useTranslation();
  if (!value) return unavailable ? <UnavailableValue /> : null;
  const normalized = value.trim().toLowerCase();
  const display = ["yes", "true", "available", "unlock", "unlocked"].includes(
    normalized,
  )
    ? t("snapshot.report.media.unlocked")
    : ["no", "false", "block", "blocked", "failed", "unavailable"].includes(
          normalized,
        )
      ? t("snapshot.report.media.blocked")
      : value;
  return <ReportValueBadge value={display} tone={mediaTone(value)} />;
}

function MailCard({
  fields,
  exportMode = false,
}: {
  fields: FieldMap;
  exportMode?: boolean;
}) {
  const { t } = useTranslation();
  const port25 = booleanField(fields, "Mail.Port25");
  const services = mailServices.flatMap((service) => {
    const value = booleanField(fields, `Mail.${service}`);
    return value === undefined ? [] : [{ service, value }];
  });
  const dns = ["Total", "Clean", "Marked", "Blacklisted"].flatMap((key) => {
    const value = fieldText(fields, `Mail.DNSBlacklist.${key}`);
    return value ? [{ key, value }] : [];
  });
  const hasData = port25 !== undefined || services.length > 0 || dns.length > 0;

  return (
    <ReportCard
      icon={<Mail aria-hidden="true" />}
      title={t("snapshot.report.mail.title")}
      detail={t("snapshot.report.mail.detail")}
    >
      {hasData ? (
        <div className="space-y-5">
          {port25 !== undefined ? (
            <div
              data-report-path="Mail.Port25"
              className="flex flex-wrap items-center justify-between gap-3"
            >
              <span className="font-medium">
                {t("snapshot.report.mail.port25")}
              </span>
              <ConnectivityBadge value={port25} />
            </div>
          ) : null}
          {services.length > 0 ? (
            <div>
              <div className="mb-3 text-sm font-medium">
                {t("snapshot.report.mail.services")}
              </div>
              <div className="flex flex-wrap gap-2">
                {services.map((service) => (
                  <Badge
                    key={service.service}
                    data-report-path={`Mail.${service.service}`}
                    variant={
                      service.value === false ? "destructive" : "outline"
                    }
                    className={cn(
                      service.value === true &&
                        "border-emerald-600/40 text-emerald-700 dark:text-emerald-400",
                    )}
                  >
                    {service.service} ·{" "}
                    {t(
                      service.value === "none"
                        ? "snapshot.unavailable"
                        : service.value
                          ? "snapshot.report.mail.reachable"
                          : "snapshot.report.mail.unreachable",
                    )}
                  </Badge>
                ))}
              </div>
            </div>
          ) : null}
          {dns.length > 0 ? (
            <div>
              <div className="mb-3 text-sm font-medium">
                {t("snapshot.report.mail.dns")}
              </div>
              <div
                className={cn(
                  "grid gap-x-8 gap-y-4",
                  exportMode ? "grid-cols-4" : "grid-cols-2 sm:grid-cols-4",
                )}
              >
                {dns.map((item) => (
                  <ReportFact
                    key={item.key}
                    label={t(`snapshot.report.mail.dnsFields.${item.key}`)}
                    value={item.value}
                    tone={dnsBlacklistTone(item.key)}
                    path={`Mail.DNSBlacklist.${item.key}`}
                  />
                ))}
              </div>
            </div>
          ) : null}
        </div>
      ) : (
        <ReportEmpty />
      )}
    </ReportCard>
  );
}

function ConnectivityBadge({ value }: { value: boolean | "none" }) {
  const { t } = useTranslation();
  return (
    <Badge
      variant={value === false ? "destructive" : "outline"}
      className={cn(
        value === true &&
          "border-emerald-600/40 text-emerald-700 dark:text-emerald-400",
      )}
    >
      {t(
        value === "none"
          ? "snapshot.unavailable"
          : value
            ? "snapshot.report.mail.reachable"
            : "snapshot.report.mail.unreachable",
      )}
    </Badge>
  );
}

function ReportCard({
  icon,
  title,
  detail,
  action,
  children,
}: {
  icon: React.ReactNode;
  title: string;
  detail: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <Card>
      <CardHeader
        className={cn(
          action && "sm:grid-cols-[1fr_auto] sm:grid-rows-[auto_auto]",
        )}
      >
        <CardTitle
          role="heading"
          aria-level={2}
          className="flex items-center gap-2 [&>svg]:size-4"
        >
          {icon}
          {title}
        </CardTitle>
        <CardDescription>{detail}</CardDescription>
        {action ? (
          <div className="col-span-full mt-2 sm:col-start-2 sm:row-span-2 sm:row-start-1 sm:mt-0 sm:justify-self-end">
            {action}
          </div>
        ) : null}
      </CardHeader>
      <CardContent>{children}</CardContent>
    </Card>
  );
}

type ReportTone = "neutral" | "green" | "yellow" | "red" | "cyan";

function ReportFact({
  label,
  value,
  tone = "neutral",
  path,
}: {
  label: string;
  value: string;
  tone?: ReportTone;
  path?: string;
}) {
  return (
    <div data-report-path={path} className="min-w-0">
      <div className="text-sm text-muted-foreground">{label}</div>
      <div
        className={cn(
          "mt-1 break-words text-base font-medium",
          reportToneTextClass(tone),
        )}
      >
        {value}
      </div>
    </div>
  );
}

function ReportEmpty() {
  const { t } = useTranslation();
  return (
    <div className="rounded-md bg-muted/50 px-4 py-3 text-sm text-muted-foreground">
      {t("snapshot.report.empty")}
    </div>
  );
}

function ClassificationValue({
  value,
  unavailable,
}: {
  value?: string;
  unavailable: boolean;
}) {
  if (!value) return unavailable ? <UnavailableValue /> : null;
  return <ReportValueBadge value={value} tone={classificationTone(value)} />;
}

function ReportValueBadge({
  value,
  tone = "neutral",
  unavailable = false,
}: {
  value?: string;
  tone?: ReportTone;
  unavailable?: boolean;
}) {
  if (!value) return unavailable ? <UnavailableValue /> : null;
  return (
    <Badge
      variant={tone === "red" ? "destructive" : "outline"}
      className={cn(
        "max-w-[65%] shrink-0 whitespace-normal break-words text-right",
        reportToneBadgeClass(tone),
      )}
    >
      {value}
    </Badge>
  );
}

function UnavailableValue() {
  return <span className="text-muted-foreground">—</span>;
}

function FormatDiagnostics({
  issues,
  fields,
}: {
  issues: ProbeSnapshot["formatIssues"];
  fields: FieldMap;
}) {
  const { t } = useTranslation();
  const counts = issues.reduce<Record<string, number>>((result, issue) => {
    result[issue.kind] = (result[issue.kind] ?? 0) + 1;
    return result;
  }, {});
  return (
    <Card className="ring-destructive/30">
      <CardHeader>
        <CardTitle className="text-destructive">
          {t("snapshot.format.title")}
        </CardTitle>
        <CardDescription>{t("snapshot.format.detail")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap gap-2">
          {Object.entries(counts).map(([kind, count]) => (
            <Badge key={kind} variant="destructive">
              {t(`snapshot.issueKind.${kind}`)} · {count}
            </Badge>
          ))}
        </div>
        <div className="divide-y rounded-md border">
          {issues.map((issue, index) => (
            <FormatIssueView
              key={`${issue.path}-${issue.kind}-${index}`}
              issue={issue}
              knownField={fields.get(issue.path)}
            />
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

function RawJSONCard({
  snapshot,
  display,
  wrap,
  copied,
  language,
  setWrap,
  copy,
  download,
}: {
  snapshot: ProbeSnapshot;
  display: string;
  wrap: boolean;
  copied: boolean;
  language?: string;
  setWrap: (value: boolean) => void;
  copy: () => Promise<void>;
  download: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <FileJson2 aria-hidden="true" className="size-4" />
          {t("snapshot.raw.title")}
        </CardTitle>
        <CardDescription>
          {t("snapshot.raw.detail", {
            sequence: snapshot.sequence,
            value: formatTime(
              snapshot.observedAt,
              language,
              t("probe.notAvailable"),
            ),
          })}
        </CardDescription>
        <CardAction className="flex items-center gap-1">
          {copied ? (
            <Badge variant="outline">
              <Check aria-hidden="true" />
              {t("snapshot.raw.copied")}
            </Badge>
          ) : null}
          <RawAction
            label={t("snapshot.raw.wrap")}
            active={wrap}
            onClick={() => setWrap(!wrap)}
            icon={<WrapText aria-hidden="true" />}
          />
          <RawAction
            label={t("snapshot.raw.copy")}
            onClick={() => void copy()}
            icon={<Clipboard aria-hidden="true" />}
          />
          <RawAction
            label={t("snapshot.raw.download")}
            onClick={download}
            icon={<Download aria-hidden="true" />}
          />
        </CardAction>
      </CardHeader>
      <CardContent>
        <pre
          className={`max-h-[70svh] rounded-md bg-muted p-4 font-mono text-sm leading-5 ${wrap ? "overflow-y-auto whitespace-pre-wrap break-words" : "overflow-auto whitespace-pre"}`}
        >
          {display}
        </pre>
      </CardContent>
    </Card>
  );
}

type FormatIssue = ProbeSnapshot["formatIssues"][number];

function FormatIssueView({
  issue,
  knownField,
}: {
  issue: FormatIssue;
  knownField?: KnownField;
}) {
  const { t } = useTranslation();
  const presentation = knownField
    ? presentProbeField(knownField, t)
    : undefined;
  return (
    <div className="flex flex-wrap items-start justify-between gap-3 p-3">
      <div className="min-w-0">
        {presentation ? (
          <div className="font-medium">{presentation.name}</div>
        ) : null}
        <code className="mt-1 block break-all text-sm text-muted-foreground">
          {issue.path}
        </code>
        {issue.expectedTypes.length > 0 ? (
          <div className="mt-1 text-sm text-muted-foreground">
            {t("snapshot.format.expected", {
              expected: issue.expectedTypes.join(", "),
            })}
          </div>
        ) : null}
      </div>
      <div className="flex shrink-0 flex-wrap gap-1">
        <Badge variant="destructive">
          {t(`snapshot.issueKind.${issue.kind}`)}
        </Badge>
        {issue.actualType ? (
          <Badge variant="outline">{issue.actualType}</Badge>
        ) : null}
      </div>
    </div>
  );
}

function RawAction({
  label,
  active,
  onClick,
  icon,
}: {
  label: string;
  active?: boolean;
  onClick: () => void;
  icon: React.ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant={active ? "secondary" : "ghost"}
          size="icon-sm"
          onClick={onClick}
          aria-label={label}
        >
          {icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

async function renderReportPNG(target: HTMLElement) {
  await document.fonts?.ready;
  const { toBlob } = await import("html-to-image");
  const blob = await toBlob(target, {
    backgroundColor: getComputedStyle(target).backgroundColor,
    cacheBust: true,
    height: target.scrollHeight,
    pixelRatio: 2,
    style: {
      left: "auto",
      pointerEvents: "auto",
      position: "static",
      top: "auto",
    },
    width: reportExportWidth,
  });
  if (!blob) throw new Error("PNG encoding returned no data");
  return blob;
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function SnapshotSkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-64 max-w-full" />
      </CardHeader>
      <CardContent>
        <Skeleton className="h-80 w-full" />
      </CardContent>
    </Card>
  );
}

function fieldText(fields: FieldMap, path: string) {
  const field = fields.get(path);
  if (field?.status !== "available" || !field.value) return undefined;
  const value = field.value.trim();
  if (!value || ["null", "n/a"].includes(value.toLowerCase())) return undefined;
  return value;
}

function fieldUnavailable(fields: FieldMap, path: string) {
  return fields.get(path)?.status === "unavailable";
}

function booleanField(fields: FieldMap, path: string) {
  const field = fields.get(path);
  if (field?.status === "unavailable") return "none" as const;
  if (field?.status !== "available") return undefined;
  if (field.value === "true") return true;
  if (field.value === "false") return false;
  return undefined;
}

function factorField(fields: FieldMap, path: string): FactorSignal {
  const field = fields.get(path);
  if (field?.status !== "available") return undefined;
  if (field.value === "true") return true;
  if (field.value === "false") return false;
  return undefined;
}

function joinDistinct(values: Array<string | undefined>) {
  const unique = [
    ...new Set(values.filter((value): value is string => Boolean(value))),
  ];
  return unique.length > 0 ? unique.join(" · ") : undefined;
}

function joinCodeAndName(code?: string, name?: string) {
  if (code && name) return `${name} (${code})`;
  return name ?? code;
}

function parseScore(value: string) {
  const numeric = Number.parseFloat(value.replace("%", ""));
  return Number.isFinite(numeric) ? numeric : undefined;
}

type ScoreProvider = (typeof scoreProviders)[number];
type RiskLevel =
  | "veryLow"
  | "low"
  | "medium"
  | "elevated"
  | "suspicious"
  | "high"
  | "veryHigh"
  | "risky"
  | "highRisk"
  | "block";

function riskLevel(
  provider: ScoreProvider,
  value?: number,
): RiskLevel | undefined {
  if (value === undefined) return undefined;
  switch (provider) {
    case "IP2LOCATION":
      return value < 33 ? "low" : value < 66 ? "medium" : "high";
    case "SCAMALYTICS":
      if (value < 20) return "low";
      if (value < 60) return "medium";
      return value < 90 ? "high" : "veryHigh";
    case "ipapi":
      if (value < 0.05) return "veryLow";
      if (value < 0.85) return "low";
      if (value < 3) return "elevated";
      return value < 20 ? "high" : "veryHigh";
    case "AbuseIPDB":
      return value < 25 ? "low" : value < 75 ? "high" : "block";
    case "IPQS":
      if (value < 75) return "low";
      if (value < 85) return "suspicious";
      return value < 90 ? "risky" : "highRisk";
    case "DBIP":
      if (value === 0) return "low";
      if (value === 50) return "medium";
      if (value === 100) return "high";
      return undefined;
  }
}

function riskTone(level?: RiskLevel): ReportTone {
  if (level === "veryLow" || level === "low") return "green";
  if (level === "medium" || level === "elevated" || level === "suspicious") {
    return "yellow";
  }
  if (level) return "red";
  return "neutral";
}

function riskBarClass(level?: RiskLevel) {
  const tone = riskTone(level);
  if (tone === "green") return "bg-emerald-500 dark:bg-emerald-400";
  if (tone === "yellow") return "bg-amber-500 dark:bg-amber-400";
  if (tone === "red") return "bg-destructive";
  return "bg-muted-foreground";
}

function riskLevelBadgeClass(level: RiskLevel) {
  return reportToneBadgeClass(riskTone(level));
}

function basicTypeTone(value?: string): ReportTone {
  const normalized = value?.trim().toLowerCase();
  if (normalized === "geo-consistent" || normalized === "原生ip") {
    return "green";
  }
  if (normalized === "geo-discrepant" || normalized === "广播ip") {
    return "red";
  }
  return "neutral";
}

function classificationTone(value: string): ReportTone {
  const normalized = value.trim().toLowerCase();
  if (["hosting", "cdn", "web spider", "机房", "蜘蛛"].includes(normalized)) {
    return "red";
  }
  if (["isp", "line isp", "mobile isp", "家宽", "手机"].includes(normalized)) {
    return "green";
  }
  if (
    [
      "business",
      "education",
      "government",
      "banking",
      "organization",
      "military",
      "library",
      "reserved",
      "other",
      "商业",
      "教育",
      "政府",
      "银行",
      "组织",
      "军队",
      "图书馆",
      "保留",
      "其他",
    ].includes(normalized)
  ) {
    return "yellow";
  }
  return "neutral";
}

function mediaTone(value: string): ReportTone {
  const normalized = value.trim().toLowerCase();
  if (["yes", "unlocked", "解锁", "native", "原生"].includes(normalized)) {
    return "green";
  }
  if (
    [
      "block",
      "blocked",
      "屏蔽",
      "failed",
      "失败",
      "china",
      "中国",
      "noprem.",
      "禁会员",
    ].includes(normalized)
  ) {
    return "red";
  }
  if (
    [
      "pending",
      "待支持",
      "nf.only",
      "仅自制",
      "webonly",
      "仅网页",
      "apponly",
      "仅app",
      "idc",
      "机房",
      "viadns",
      "dns",
    ].includes(normalized)
  ) {
    return "yellow";
  }
  return "neutral";
}

function dnsBlacklistTone(key: string): ReportTone {
  if (key === "Total") return "cyan";
  if (key === "Clean") return "green";
  if (key === "Marked") return "yellow";
  if (key === "Blacklisted") return "red";
  return "neutral";
}

function reportToneTextClass(tone: ReportTone) {
  if (tone === "green") return "text-emerald-700 dark:text-emerald-400";
  if (tone === "yellow") return "text-amber-700 dark:text-amber-400";
  if (tone === "red") return "text-destructive";
  if (tone === "cyan") return "text-cyan-700 dark:text-cyan-400";
  return undefined;
}

function reportToneBadgeClass(tone: ReportTone) {
  if (tone === "green") {
    return "border-emerald-600/40 text-emerald-700 dark:text-emerald-400";
  }
  if (tone === "yellow") {
    return "border-amber-600/40 text-amber-700 dark:text-amber-400";
  }
  if (tone === "cyan") {
    return "border-cyan-600/40 text-cyan-700 dark:text-cyan-400";
  }
  return undefined;
}

function decodeProbeResult(value: string) {
  const binary = window.atob(value);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
}
