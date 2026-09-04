import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type FormEvent,
} from "react";
import {
  CirclePause,
  CirclePlay,
  Filter,
  LoaderCircle,
  RefreshCw,
  RotateCcw,
  Search,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  getLog,
  listLogs,
  type LogEventDetail,
  type LogEventSummary,
  type LogFilters,
  type LogLevel,
} from "@/api/logs";
import { listNodes, type Node } from "@/api/nodes";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatAPIError } from "@/lib/api-error";

const refreshIntervalMilliseconds = 5_000;

type FilterForm = {
  from: string;
  to: string;
  nodeId: string;
  level: "all" | LogLevel;
  component: string;
  eventType: string;
  publicAddress: string;
  taskId: string;
  proxyId: string;
  keyword: string;
};

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; items: LogEventSummary[]; nextCursor?: string }
  | { kind: "error"; message: string };

const emptyFilters: FilterForm = {
  from: "",
  to: "",
  nodeId: "all",
  level: "all",
  component: "",
  eventType: "",
  publicAddress: "",
  taskId: "",
  proxyId: "",
  keyword: "",
};

export function AgentLogViewer({ nodeId }: { nodeId?: string }) {
  const { i18n, t } = useTranslation();
  const [form, setForm] = useState<FilterForm>({
    ...emptyFilters,
    nodeId: nodeId ?? "all",
  });
  const [filters, setFilters] = useState<FilterForm>(form);
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [nodes, setNodes] = useState<Node[]>([]);
  const [paused, setPaused] = useState(false);
  const [refreshError, setRefreshError] = useState<string>();
  const [loadingMore, setLoadingMore] = useState(false);
  const [selected, setSelected] = useState<LogEventSummary>();
  const [detail, setDetail] = useState<
    | { kind: "loading" }
    | { kind: "success"; value: LogEventDetail }
    | { kind: "error" }
  >();

  const query = useMemo(
    () => filtersToQuery(filters, nodeId),
    [filters, nodeId],
  );

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setState({ kind: "loading" });
      try {
        const page = await listLogs({ ...query, pageSize: 100 }, signal);
        setState({
          kind: "success",
          items: page.items,
          nextCursor: page.nextCursor,
        });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setState({ kind: "error", message: formatAPIError(error, t) });
      }
    },
    [query, t],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    if (nodeId !== undefined) return;
    const controller = new AbortController();
    void listNodes(controller.signal)
      .then(setNodes)
      .catch((error: unknown) => {
        if (!(error instanceof DOMException && error.name === "AbortError")) {
          setNodes([]);
        }
      });
    return () => controller.abort();
  }, [nodeId]);

  useEffect(() => {
    if (paused || selected !== undefined || state.kind !== "success") return;
    let inFlight = false;
    let disposed = false;
    const refresh = async () => {
      if (inFlight || disposed || document.visibilityState !== "visible")
        return;
      inFlight = true;
      try {
        const newest = state.items[0]?.occurredAt;
        const page = await listLogs({
          ...query,
          from: newest ?? query.from,
          pageSize: 100,
        });
        if (disposed) return;
        setRefreshError(undefined);
        if (page.items.length === 0) return;
        setState((current) => {
          if (current.kind !== "success") return current;
          const merged = new Map(current.items.map((item) => [item.id, item]));
          for (const item of page.items) merged.set(item.id, item);
          return {
            ...current,
            items: [...merged.values()].sort(compareLogs),
          };
        });
      } catch (error) {
        if (!disposed) setRefreshError(formatAPIError(error, t));
      } finally {
        inFlight = false;
      }
    };
    const interval = window.setInterval(
      () => void refresh(),
      refreshIntervalMilliseconds,
    );
    const wake = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    document.addEventListener("visibilitychange", wake);
    window.addEventListener("focus", wake);
    return () => {
      disposed = true;
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", wake);
      window.removeEventListener("focus", wake);
    };
  }, [paused, query, selected, state, t]);

  function applyFilters(event: FormEvent) {
    event.preventDefault();
    setFilters(form);
  }

  function resetFilters() {
    const reset = { ...emptyFilters, nodeId: nodeId ?? "all" };
    setForm(reset);
    setFilters(reset);
  }

  async function loadMore() {
    if (state.kind !== "success" || state.nextCursor === undefined) return;
    setPaused(true);
    setLoadingMore(true);
    try {
      const page = await listLogs({
        ...query,
        cursor: state.nextCursor,
        pageSize: 100,
      });
      setState({
        kind: "success",
        items: [...state.items, ...page.items],
        nextCursor: page.nextCursor,
      });
    } catch (error) {
      setState({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setLoadingMore(false);
    }
  }

  async function openDetail(event: LogEventSummary) {
    setSelected(event);
    setPaused(true);
    setDetail({ kind: "loading" });
    try {
      setDetail({ kind: "success", value: await getLog(event.id) });
    } catch {
      setDetail({ kind: "error" });
    }
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Filter aria-hidden="true" className="size-4" />
            {t("logs.filters.title")}
          </CardTitle>
          <CardDescription>{t("logs.filters.detail")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={applyFilters}>
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
              <FilterInput label={t("logs.filters.from")}>
                <Input
                  type="datetime-local"
                  aria-label={t("logs.filters.from")}
                  value={form.from}
                  onChange={(event) =>
                    setForm({ ...form, from: event.target.value })
                  }
                />
              </FilterInput>
              <FilterInput label={t("logs.filters.to")}>
                <Input
                  type="datetime-local"
                  aria-label={t("logs.filters.to")}
                  value={form.to}
                  onChange={(event) =>
                    setForm({ ...form, to: event.target.value })
                  }
                />
              </FilterInput>
              {nodeId === undefined ? (
                <FilterInput label={t("logs.filters.node")}>
                  <Select
                    value={form.nodeId}
                    onValueChange={(value) =>
                      setForm({ ...form, nodeId: value })
                    }
                  >
                    <SelectTrigger
                      className="w-full"
                      aria-label={t("logs.filters.node")}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">
                        {t("logs.filters.allNodes")}
                      </SelectItem>
                      {nodes.map((node) => (
                        <SelectItem key={node.id} value={node.id}>
                          {node.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FilterInput>
              ) : null}
              <FilterInput label={t("logs.filters.level")}>
                <Select
                  value={form.level}
                  onValueChange={(value) =>
                    setForm({ ...form, level: value as FilterForm["level"] })
                  }
                >
                  <SelectTrigger
                    className="w-full"
                    aria-label={t("logs.filters.level")}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">
                      {t("logs.filters.allLevels")}
                    </SelectItem>
                    {(["error", "warn", "info", "debug"] as const).map(
                      (level) => (
                        <SelectItem key={level} value={level}>
                          {t(`logs.level.${level}`)}
                        </SelectItem>
                      ),
                    )}
                  </SelectContent>
                </Select>
              </FilterInput>
              <TextFilter
                label={t("logs.filters.component")}
                value={form.component}
                onChange={(value) => setForm({ ...form, component: value })}
              />
              <TextFilter
                label={t("logs.filters.eventType")}
                value={form.eventType}
                onChange={(value) => setForm({ ...form, eventType: value })}
              />
              <TextFilter
                label={t("logs.filters.publicAddress")}
                value={form.publicAddress}
                onChange={(value) => setForm({ ...form, publicAddress: value })}
              />
              <TextFilter
                label={t("logs.filters.taskId")}
                value={form.taskId}
                onChange={(value) => setForm({ ...form, taskId: value })}
              />
              <TextFilter
                label={t("logs.filters.proxyId")}
                value={form.proxyId}
                onChange={(value) => setForm({ ...form, proxyId: value })}
              />
              <TextFilter
                label={t("logs.filters.keyword")}
                value={form.keyword}
                onChange={(value) => setForm({ ...form, keyword: value })}
              />
            </div>
            <div className="flex flex-wrap justify-end gap-2 border-t pt-4">
              <Button type="button" variant="outline" onClick={resetFilters}>
                <RotateCcw data-icon="inline-start" aria-hidden="true" />
                {t("logs.filters.reset")}
              </Button>
              <Button type="submit">
                <Search data-icon="inline-start" aria-hidden="true" />
                {t("logs.filters.apply")}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("logs.list.title")}</CardTitle>
          <CardDescription>
            {paused ? t("logs.list.paused") : t("logs.list.live")}
          </CardDescription>
          <div className="absolute top-4 right-4">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPaused((value) => !value)}
            >
              {paused ? (
                <CirclePlay data-icon="inline-start" aria-hidden="true" />
              ) : (
                <CirclePause data-icon="inline-start" aria-hidden="true" />
              )}
              {paused ? t("logs.list.resume") : t("logs.list.pause")}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {refreshError ? (
            <Alert variant="destructive" className="mb-4">
              <TriangleAlert aria-hidden="true" />
              <AlertDescription>
                {t("logs.list.refreshFailed", { error: refreshError })}
              </AlertDescription>
            </Alert>
          ) : null}
          {state.kind === "loading" ? <LogSkeleton /> : null}
          {state.kind === "error" ? (
            <Alert variant="destructive">
              <TriangleAlert aria-hidden="true" />
              <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
                <span>{state.message}</span>
                <Button variant="outline" size="sm" onClick={() => void load()}>
                  <RefreshCw data-icon="inline-start" aria-hidden="true" />
                  {t("logs.list.retry")}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {state.kind === "success" && state.items.length === 0 ? (
            <div className="py-12 text-center text-sm text-muted-foreground">
              {t("logs.list.empty")}
            </div>
          ) : null}
          {state.kind === "success" && state.items.length > 0 ? (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("logs.list.time")}</TableHead>
                    {nodeId === undefined ? (
                      <TableHead>{t("logs.list.node")}</TableHead>
                    ) : null}
                    <TableHead>{t("logs.list.level")}</TableHead>
                    <TableHead>{t("logs.list.component")}</TableHead>
                    <TableHead className="w-full">
                      {t("logs.list.message")}
                    </TableHead>
                    <TableHead>
                      <span className="sr-only">{t("logs.list.details")}</span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {state.items.map((event) => (
                    <TableRow key={event.id}>
                      <TableCell>
                        {formatTime(event.occurredAt, i18n.resolvedLanguage)}
                      </TableCell>
                      {nodeId === undefined ? (
                        <TableCell>
                          {event.nodeName ?? t("logs.center")}
                        </TableCell>
                      ) : null}
                      <TableCell>
                        <LogLevelBadge level={event.level} />
                      </TableCell>
                      <TableCell>
                        <span className="font-mono text-xs">
                          {event.component}
                        </span>
                      </TableCell>
                      <TableCell className="max-w-xl whitespace-normal">
                        <p className="line-clamp-2">{eventMessage(event, t)}</p>
                        <p className="mt-0.5 font-mono text-xs text-muted-foreground">
                          {event.eventType}
                        </p>
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => void openDetail(event)}
                        >
                          {t("logs.list.details")}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              {state.nextCursor !== undefined ? (
                <div className="flex justify-center border-t pt-4">
                  <Button
                    variant="outline"
                    disabled={loadingMore}
                    onClick={() => void loadMore()}
                  >
                    {loadingMore ? (
                      <LoaderCircle
                        data-icon="inline-start"
                        className="animate-spin"
                        aria-hidden="true"
                      />
                    ) : null}
                    {t("logs.list.loadMore")}
                  </Button>
                </div>
              ) : null}
            </>
          ) : null}
        </CardContent>
      </Card>

      <LogDetailDialog
        summary={selected}
        detail={detail}
        onOpenChange={(open) => {
          if (!open) {
            setSelected(undefined);
            setDetail(undefined);
          }
        }}
      />
    </div>
  );
}

function LogDetailDialog({
  summary,
  detail,
  onOpenChange,
}: {
  summary?: LogEventSummary;
  detail?:
    | { kind: "loading" }
    | { kind: "success"; value: LogEventDetail }
    | { kind: "error" };
  onOpenChange: (open: boolean) => void;
}) {
  const { i18n, t } = useTranslation();
  const event = detail?.kind === "success" ? detail.value.event : summary;
  return (
    <Dialog open={summary !== undefined} onOpenChange={onOpenChange}>
      <DialogContent closeLabel={t("common.close")} className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("logs.entry.title")}</DialogTitle>
          <DialogDescription>
            {event ? eventMessage(event, t) : t("logs.entry.loading")}
          </DialogDescription>
        </DialogHeader>
        {detail?.kind === "loading" ? <LogSkeleton /> : null}
        {detail?.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertDescription>{t("logs.entry.loadFailed")}</AlertDescription>
          </Alert>
        ) : null}
        {detail?.kind === "success" ? (
          <div className="space-y-4">
            <dl className="grid gap-3 rounded-md border p-4 sm:grid-cols-2">
              <DetailField
                label={t("logs.entry.occurredAt")}
                value={formatTime(
                  detail.value.event.occurredAt,
                  i18n.resolvedLanguage,
                )}
              />
              <DetailField
                label={t("logs.entry.receivedAt")}
                value={formatTime(
                  detail.value.event.receivedAt,
                  i18n.resolvedLanguage,
                )}
              />
              <DetailField
                label={t("logs.entry.level")}
                value={t(`logs.level.${detail.value.event.level}`)}
              />
              <DetailField
                label={t("logs.entry.component")}
                value={detail.value.event.component}
                mono
              />
              <DetailField
                label={t("logs.entry.eventType")}
                value={detail.value.event.eventType}
                mono
              />
              <DetailField
                label={t("logs.entry.node")}
                value={detail.value.event.nodeName}
              />
              <DetailField
                label={t("logs.entry.publicAddress")}
                value={detail.value.event.publicAddress}
                mono
              />
              <DetailField
                label={t("logs.entry.family")}
                value={detail.value.event.family}
              />
              <DetailField
                label={t("logs.entry.taskId")}
                value={detail.value.event.taskId}
                mono
              />
              <DetailField
                label={t("logs.entry.proxyId")}
                value={detail.value.event.proxyId}
                mono
              />
              <DetailField
                label={t("logs.entry.configurationRevision")}
                value={detail.value.event.configurationRevision?.toString()}
              />
              <DetailField
                label={t("logs.entry.discoveryPath")}
                value={detail.value.discoveryPath}
                mono
              />
              <DetailField
                label={t("logs.entry.failureCategory")}
                value={
                  detail.value.event.failureCategory
                    ? t(`logs.failure.${detail.value.event.failureCategory}`)
                    : undefined
                }
              />
              <DetailField
                label={t("logs.entry.request")}
                value={[
                  detail.value.event.requestMethod,
                  detail.value.event.requestTarget,
                ]
                  .filter(Boolean)
                  .join(" ")}
                mono
              />
              <DetailField
                label={t("logs.entry.httpStatus")}
                value={detail.value.event.httpStatus?.toString()}
              />
              <DetailField
                label={t("logs.entry.duration")}
                value={
                  detail.value.event.durationMilliseconds === undefined
                    ? undefined
                    : t("logs.entry.milliseconds", {
                        count: detail.value.event.durationMilliseconds,
                      })
                }
              />
              <DetailField
                label={t("logs.entry.rateLimitHeaders")}
                value={formatRateLimitHeaders(detail.value.rateLimitHeaders)}
                mono
              />
            </dl>
            {detail.value.responseBody !== undefined ? (
              <div className="space-y-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <Label>{t("logs.entry.responseBody")}</Label>
                  <span className="text-xs text-muted-foreground">
                    {t("logs.entry.responseMeta", {
                      count: detail.value.event.responseBodyBytes,
                    })}
                    {detail.value.event.responseTruncated
                      ? ` · ${t("logs.entry.truncated")}`
                      : ""}
                  </span>
                </div>
                <pre className="max-h-80 overflow-auto rounded-md border bg-muted/40 p-3 text-xs whitespace-pre-wrap break-all">
                  {decodeResponseBody(detail.value.responseBody)}
                </pre>
              </div>
            ) : null}
          </div>
        ) : null}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function FilterInput({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

function TextFilter({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <FilterInput label={label}>
      <Input
        aria-label={label}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </FilterInput>
  );
}

function DetailField({
  label,
  value,
  mono = false,
}: {
  label: string;
  value?: string;
  mono?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={`mt-1 break-all text-sm ${mono ? "font-mono" : ""}`}>
        {value || t("logs.entry.notAvailable")}
      </dd>
    </div>
  );
}

function LogLevelBadge({ level }: { level: LogLevel }) {
  const { t } = useTranslation();
  const variant =
    level === "error"
      ? "destructive"
      : level === "warn"
        ? "warning"
        : level === "info"
          ? "info"
          : "outline";
  return <Badge variant={variant}>{t(`logs.level.${level}`)}</Badge>;
}

function LogSkeleton() {
  return (
    <div className="space-y-3" aria-busy="true">
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-10 w-full" />
    </div>
  );
}

function filtersToQuery(filters: FilterForm, fixedNodeId?: string): LogFilters {
  const query: LogFilters = {};
  const from = localDateTimeToISO(filters.from);
  const to = localDateTimeToISO(filters.to);
  if (from) query.from = from;
  if (to) query.to = to;
  const nodeId =
    fixedNodeId ?? (filters.nodeId === "all" ? undefined : filters.nodeId);
  if (nodeId) query.nodeId = nodeId;
  if (filters.level !== "all") query.level = filters.level;
  for (const [key, value] of [
    ["component", filters.component],
    ["eventType", filters.eventType],
    ["publicAddress", filters.publicAddress],
    ["taskId", filters.taskId],
    ["proxyId", filters.proxyId],
    ["keyword", filters.keyword],
  ] as const) {
    if (value.trim()) query[key] = value.trim();
  }
  return query;
}

function localDateTimeToISO(value: string) {
  if (!value) return undefined;
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
}

function compareLogs(left: LogEventSummary, right: LogEventSummary) {
  const time = right.occurredAt.localeCompare(left.occurredAt);
  return time === 0 ? right.id.localeCompare(left.id) : time;
}

function formatRateLimitHeaders(headers?: Record<string, string>) {
  if (headers === undefined) return undefined;
  return Object.entries(headers)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([name, value]) => `${name}: ${value}`)
    .join("\n");
}

function eventMessage(
  event: LogEventSummary,
  t: ReturnType<typeof useTranslation>["t"],
) {
  return t(`logs.events.${event.eventType}`, { defaultValue: event.message });
}

function formatTime(value: string, locale?: string) {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "short",
    timeStyle: "medium",
  }).format(new Date(value));
}

function decodeResponseBody(value: string) {
  try {
    const bytes = Uint8Array.from(window.atob(value), (character) =>
      character.charCodeAt(0),
    );
    return new TextDecoder().decode(bytes);
  } catch {
    return value;
  }
}
