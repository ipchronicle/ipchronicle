import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  Check,
  Clipboard,
  Download,
  FileJson2,
  GitCompareArrows,
  Star,
  TriangleAlert,
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
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatAPIError } from "@/lib/api-error";
import {
  presentProbeField,
  presentProbeFieldGroup,
} from "@/lib/probe-field-label";
import { formatTime } from "@/pages/node-probe-page";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; snapshot: ProbeSnapshot; raw: string }
  | { kind: "not-found" }
  | { kind: "error" };

type KnownField = ProbeSnapshot["fields"][number];

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
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0 max-w-2xl">
          <Button variant="ghost" size="sm" asChild className="mb-3 -ml-3">
            <Link to={runId ? `/probe-runs/${runId}` : "/history"}>
              <ArrowLeft data-icon="inline-start" aria-hidden="true" />
              {t("snapshot.back")}
            </Link>
          </Button>
          <p className="text-xs font-medium text-muted-foreground uppercase">
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
  const groups = useMemo(() => groupFields(snapshot.fields), [snapshot.fields]);
  const fieldsByPath = useMemo(
    () => new Map(snapshot.fields.map((field) => [field.path, field])),
    [snapshot.fields],
  );
  const [view, setView] = useState<"structured" | "raw">("structured");
  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t("snapshot.summary.title")}</CardTitle>
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
          {snapshot.previousSnapshotId ? (
            <CardAction>
              <Button variant="outline" size="sm" asChild>
                <Link
                  to={`/history/compare?before=${snapshot.previousSnapshotId}&after=${snapshot.id}`}
                >
                  <GitCompareArrows
                    data-icon="inline-start"
                    aria-hidden="true"
                  />
                  {t("snapshot.comparePrevious")}
                </Link>
              </Button>
            </CardAction>
          ) : null}
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
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
            <Badge variant="outline">{t("snapshot.summary.noChanges")}</Badge>
          )}
          {snapshot.formatIssues.length > 0 ? (
            <Badge variant="destructive">
              {t("snapshot.summary.formatIssues", {
                count: snapshot.formatIssues.length,
              })}
            </Badge>
          ) : (
            <Badge variant="secondary">
              {t("snapshot.summary.compatible")}
            </Badge>
          )}
        </CardContent>
      </Card>

      {snapshot.formatIssues.length > 0 ? (
        <Card className="ring-destructive/30">
          <CardHeader>
            <CardTitle className="text-destructive">
              {t("snapshot.format.title")}
            </CardTitle>
            <CardDescription>{t("snapshot.format.detail")}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {snapshot.formatIssues.map((issue, index) => (
              <FormatIssueView
                key={`${issue.path}-${issue.kind}-${index}`}
                issue={issue}
                knownField={fieldsByPath.get(issue.path)}
              />
            ))}
          </CardContent>
        </Card>
      ) : null}

      <Tabs value={view} onValueChange={(next) => setView(next as typeof view)}>
        <TabsList aria-label={t("snapshot.views.label")}>
          <TabsTrigger value="structured">
            {t("snapshot.views.structured")}
          </TabsTrigger>
          <TabsTrigger value="raw">{t("snapshot.views.raw")}</TabsTrigger>
        </TabsList>
        {view === "structured" ? (
          <div className="space-y-4 pt-2">
            {groups.map(([group, fields]) => (
              <Card key={group}>
                <FieldGroupHeader group={group} fieldCount={fields.length} />
                <CardContent>
                  <div className="grid gap-3 md:grid-cols-2">
                    {fields.map((field) => (
                      <KnownFieldView key={field.id} field={field} />
                    ))}
                  </div>
                </CardContent>
              </Card>
            ))}
            {snapshot.changes.length > 0 ? (
              <Card>
                <CardHeader>
                  <CardTitle>{t("snapshot.changes.title")}</CardTitle>
                  <CardDescription>
                    {t("snapshot.changes.detail")}
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {snapshot.changes.map((change) => (
                    <div key={change.fieldId} className="rounded-md border p-3">
                      <FieldIdentity field={change} />
                      <div className="mt-3 grid gap-3 md:grid-cols-2">
                        <ChangedValue
                          label={t("comparison.before")}
                          value={change.before}
                        />
                        <ChangedValue
                          label={t("comparison.after")}
                          value={change.after}
                        />
                      </div>
                    </div>
                  ))}
                </CardContent>
              </Card>
            ) : null}
          </div>
        ) : (
          <div className="pt-2">
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
                  className={`max-h-[70svh] rounded-md bg-muted p-4 font-mono text-xs leading-5 ${wrap ? "overflow-y-auto whitespace-pre-wrap break-words" : "overflow-auto whitespace-pre"}`}
                >
                  {display}
                </pre>
              </CardContent>
            </Card>
          </div>
        )}
      </Tabs>
    </>
  );
}

function KnownFieldView({ field }: { field: KnownField }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0 rounded-md border p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <FieldIdentity field={field} />
        <Badge
          variant={field.status === "available" ? "outline" : "destructive"}
        >
          {t(`snapshot.fieldStatus.${field.status}`)}
        </Badge>
      </div>
      <div className="mt-3 break-all font-mono text-xs">
        {field.status === "available" ? field.value : t("snapshot.unavailable")}
      </div>
      {field.status === "incompatible" ? (
        <div className="mt-2 text-xs text-muted-foreground">
          {t("snapshot.actualType", {
            actual: field.actualType,
            expected: field.expectedTypes.join(", "),
          })}
        </div>
      ) : null}
    </div>
  );
}

function FieldGroupHeader({
  group,
  fieldCount,
}: {
  group: string;
  fieldCount: number;
}) {
  const { t } = useTranslation();
  const presentation = presentProbeFieldGroup(group, t);
  return (
    <CardHeader>
      <CardTitle>{presentation.name}</CardTitle>
      <CardDescription>
        {presentation.description}{" "}
        {t("snapshot.structured.fieldCount", { count: fieldCount })}
      </CardDescription>
    </CardHeader>
  );
}

function FieldIdentity({ field }: { field: ProbeFieldIdentity }) {
  const { t } = useTranslation();
  const presentation = presentProbeField(field, t);
  return (
    <div className="min-w-0">
      <div className="font-medium">{presentation.name}</div>
      <div className="mt-1 text-xs text-muted-foreground">
        {presentation.description}
      </div>
      <code className="mt-1 block break-all text-xs text-muted-foreground">
        {field.path}
      </code>
    </div>
  );
}

type ProbeFieldIdentity = Pick<KnownField, "group" | "path">;
type FormatIssue = ProbeSnapshot["formatIssues"][number];

function FormatIssueView({
  issue,
  knownField,
}: {
  issue: FormatIssue;
  knownField?: KnownField;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-start justify-between gap-3 rounded-md border p-3">
      {knownField ? (
        <FieldIdentity field={knownField} />
      ) : (
        <code className="break-all text-xs">{issue.path}</code>
      )}
      <div className="flex flex-wrap gap-1">
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

function ChangedValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md bg-muted p-3">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-2 break-all font-mono text-xs">{value}</div>
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

function groupFields(fields: KnownField[]) {
  const groups = new Map<string, KnownField[]>();
  for (const field of fields) {
    const current = groups.get(field.group) ?? [];
    current.push(field);
    groups.set(field.group, current);
  }
  return [...groups.entries()];
}

function decodeProbeResult(value: string) {
  const binary = window.atob(value);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
}
