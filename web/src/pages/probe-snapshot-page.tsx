import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  Check,
  Clipboard,
  Download,
  FileJson2,
  TriangleAlert,
  WrapText,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams, useSearchParams } from "react-router";

import { getProbeSnapshot, type ProbeSnapshot } from "@/api/probes";
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
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatTime } from "@/pages/node-probe-page";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; snapshot: ProbeSnapshot; raw: string }
  | { kind: "not-found" }
  | { kind: "error" };

export function ProbeSnapshotPage() {
  const { snapshotId = "" } = useParams();
  const [search] = useSearchParams();
  const { t, i18n } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [wrap, setWrap] = useState(true);
  const [copied, setCopied] = useState(false);

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

  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="min-w-0 max-w-2xl">
        <Button variant="ghost" size="sm" asChild className="mb-3 -ml-3">
          <Link to={runId ? `/probe-runs/${runId}` : "/nodes"}>
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

      <div className="mt-8 space-y-4" aria-live="polite">
        {state.kind === "loading" ? (
          <Card>
            <CardHeader>
              <Skeleton className="h-5 w-40" />
              <Skeleton className="h-4 w-64 max-w-full" />
            </CardHeader>
            <CardContent>
              <Skeleton className="h-80 w-full" />
            </CardContent>
          </Card>
        ) : null}
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
        {state.kind === "success" ? (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileJson2 aria-hidden="true" className="size-4" />
                {t("snapshot.raw.title")}
              </CardTitle>
              <CardDescription>
                {t("snapshot.raw.detail", {
                  sequence: state.snapshot.sequence,
                  value: formatTime(
                    state.snapshot.observedAt,
                    i18n.resolvedLanguage,
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
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant={wrap ? "secondary" : "ghost"}
                      size="icon-sm"
                      onClick={() => setWrap((value) => !value)}
                      aria-label={t("snapshot.raw.wrap")}
                    >
                      <WrapText aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t("snapshot.raw.wrap")}</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => void copy()}
                      aria-label={t("snapshot.raw.copy")}
                    >
                      <Clipboard aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t("snapshot.raw.copy")}</TooltipContent>
                </Tooltip>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={download}
                      aria-label={t("snapshot.raw.download")}
                    >
                      <Download aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t("snapshot.raw.download")}</TooltipContent>
                </Tooltip>
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
        ) : null}
      </div>
    </main>
  );
}

function decodeProbeResult(value: string) {
  const binary = window.atob(value);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
}
