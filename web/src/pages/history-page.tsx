import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  ArrowRight,
  CircleAlert,
  FileClock,
  GitCompareArrows,
  History as HistoryIcon,
  RotateCcw,
  SearchX,
  Star,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import {
  listHistoryAddressEvents,
  listHistoryFormatEvents,
  listHistoryProbeGaps,
  listHistoryProbeSnapshots,
  type AddressHistoryPage,
  type ProbeFormatEventPage,
  type ProbeHistoryGapPage,
  type ProbeSnapshotHistoryPage,
} from "@/api/history";
import { getNodeNetwork, type PublicAddress } from "@/api/network";
import { listNodes, type Node } from "@/api/nodes";
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
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatTime } from "@/pages/node-probe-page";

const pageSize = 25;
const all = "all";
const pageParameters = ["page", "gapPage", "formatPage"];

type Snapshot = ProbeSnapshotHistoryPage["items"][number];
type AddressEvent = AddressHistoryPage["events"][number];
type ProbeHistoryGap = ProbeHistoryGapPage["items"][number];
type ProbeFormatEvent = ProbeFormatEventPage["items"][number];
type HistoryTab = "reports" | "addresses";

type ViewState =
  | { kind: "loading" }
  | {
      kind: "success";
      nodes: Node[];
      publicAddresses: PublicAddress[];
      reports?: ProbeSnapshotHistoryPage;
      probeGaps?: ProbeHistoryGapPage;
      formatEvents?: ProbeFormatEventPage;
      addresses?: AddressHistoryPage;
    }
  | { kind: "error" };

export function HistoryPage() {
  const { t, i18n } = useTranslation();
  const [search, setSearch] = useSearchParams();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const tab: HistoryTab =
    search.get("tab") === "addresses" ? "addresses" : "reports";
  const queryKey = search.toString();

  const load = useCallback(
    async (signal?: AbortSignal) => {
      setState({ kind: "loading" });
      try {
        const params = new URLSearchParams(queryKey);
        const nodeId = value(params, "nodeId");
        const common = {
          nodeId,
          egressId: value(params, "egressId"),
          from: toISO(value(params, "from")),
          to: toISO(value(params, "to")),
          page: positiveInteger(params.get("page"), 1),
          pageSize,
        };
        const nodesPromise = listNodes(signal);
        const publicAddressesPromise = nodeId
          ? getNodeNetwork(nodeId, signal).then(
              (network) => network.publicAddresses,
            )
          : Promise.resolve([] as PublicAddress[]);
        if (tab === "reports") {
          const changed = value(params, "changed");
          const [nodes, publicAddresses, reports, probeGaps, formatEvents] =
            await Promise.all([
              nodesPromise,
              publicAddressesPromise,
              listHistoryProbeSnapshots(
                {
                  ...common,
                  runStatus: value(params, "runStatus") as
                    "running" | "succeeded" | "partial" | "failed" | undefined,
                  trigger: value(params, "trigger") as
                    "manual" | "schedule" | "address-change" | undefined,
                  changed:
                    changed === "true"
                      ? true
                      : changed === "false"
                        ? false
                        : undefined,
                  formatStatus: value(params, "formatStatus") as
                    "compatible" | "mismatch" | undefined,
                },
                signal,
              ),
              listHistoryProbeGaps(
                {
                  ...common,
                  page: positiveInteger(params.get("gapPage"), 1),
                },
                signal,
              ),
              listHistoryFormatEvents(
                {
                  ...common,
                  page: positiveInteger(params.get("formatPage"), 1),
                },
                signal,
              ),
            ]);
          setState({
            kind: "success",
            nodes,
            publicAddresses,
            reports,
            probeGaps,
            formatEvents,
          });
          return;
        }
        const [nodes, publicAddresses, addresses] = await Promise.all([
          nodesPromise,
          publicAddressesPromise,
          listHistoryAddressEvents(
            {
              ...common,
              gapPage: positiveInteger(params.get("gapPage"), 1),
              eventKind: value(params, "eventKind") as
                | "first-observation"
                | "address-change"
                | "check-failure"
                | "recovery"
                | undefined,
              family: value(params, "family") as "ipv4" | "ipv6" | undefined,
            },
            signal,
          ),
        ]);
        setState({ kind: "success", nodes, publicAddresses, addresses });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setState({ kind: "error" });
      }
    },
    [queryKey, tab],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const update = useCallback(
    (name: string, next?: string) => {
      const params = new URLSearchParams(search);
      if (next === undefined || next === "" || next === all)
        params.delete(name);
      else params.set(name, next);
      if (!pageParameters.includes(name)) {
        for (const parameter of pageParameters) params.delete(parameter);
      }
      if (name === "nodeId") params.delete("egressId");
      setSearch(params);
    },
    [search, setSearch],
  );

  const clearFilters = useCallback(() => {
    const params = new URLSearchParams();
    if (tab === "addresses") params.set("tab", "addresses");
    setSearch(params);
  }, [setSearch, tab]);

  const hasFilters = useMemo(
    () =>
      [
        "nodeId",
        "egressId",
        "from",
        "to",
        "runStatus",
        "trigger",
        "changed",
        "formatStatus",
        "eventKind",
        "family",
      ].some((key) => search.has(key)),
    [search],
  );

  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="max-w-2xl">
        <p className="text-xs font-medium text-muted-foreground uppercase">
          {t("history.section")}
        </p>
        <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
          {t("history.title")}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {t("history.detail")}
        </p>
      </div>

      <Tabs
        className="mt-8"
        value={tab}
        onValueChange={(next) => update("tab", next)}
      >
        <TabsList aria-label={t("history.tabs.label")}>
          <TabsTrigger value="reports">
            <FileClock aria-hidden="true" />
            {t("history.tabs.reports")}
          </TabsTrigger>
          <TabsTrigger value="addresses">
            <HistoryIcon aria-hidden="true" />
            {t("history.tabs.addresses")}
          </TabsTrigger>
        </TabsList>

        <Card className="mt-4">
          <CardHeader>
            <CardTitle>{t("history.filters.title")}</CardTitle>
            <CardDescription>{t("history.filters.detail")}</CardDescription>
            {hasFilters ? (
              <CardAction>
                <Button variant="ghost" size="sm" onClick={clearFilters}>
                  <RotateCcw data-icon="inline-start" aria-hidden="true" />
                  {t("history.filters.clear")}
                </Button>
              </CardAction>
            ) : null}
          </CardHeader>
          <CardContent>
            <FilterGrid
              tab={tab}
              search={search}
              nodes={state.kind === "success" ? state.nodes : []}
              publicAddresses={
                state.kind === "success" ? state.publicAddresses : []
              }
              update={update}
            />
          </CardContent>
        </Card>

        <div className="mt-4" aria-live="polite">
          {state.kind === "loading" ? <HistorySkeleton /> : null}
          {state.kind === "error" ? (
            <Alert variant="destructive">
              <TriangleAlert aria-hidden="true" />
              <AlertTitle>{t("history.loadFailed")}</AlertTitle>
              <AlertDescription>
                <Button
                  variant="outline"
                  size="sm"
                  className="mt-3"
                  onClick={() => void load()}
                >
                  {t("history.retry")}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {tab === "reports" && state.kind === "success" && state.reports ? (
            <ReportHistory
              page={state.reports}
              gaps={state.probeGaps ?? { items: [], total: 0 }}
              formatEvents={state.formatEvents ?? { items: [], total: 0 }}
              currentPage={positiveInteger(search.get("page"), 1)}
              currentGapPage={positiveInteger(search.get("gapPage"), 1)}
              currentFormatPage={positiveInteger(search.get("formatPage"), 1)}
              hasFilters={hasFilters}
              update={update}
              clearFilters={clearFilters}
              language={i18n.resolvedLanguage}
            />
          ) : null}
          {tab === "addresses" &&
          state.kind === "success" &&
          state.addresses ? (
            <AddressHistory
              page={state.addresses}
              currentPage={positiveInteger(search.get("page"), 1)}
              currentGapPage={positiveInteger(search.get("gapPage"), 1)}
              hasFilters={hasFilters}
              update={update}
              clearFilters={clearFilters}
              language={i18n.resolvedLanguage}
            />
          ) : null}
        </div>
      </Tabs>
    </main>
  );
}

function FilterGrid({
  tab,
  search,
  nodes,
  publicAddresses,
  update,
}: {
  tab: HistoryTab;
  search: URLSearchParams;
  nodes: Node[];
  publicAddresses: PublicAddress[];
  update: (name: string, value?: string) => void;
}) {
  const { t } = useTranslation();
  const nodeId = value(search, "nodeId");
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <FilterSelect
        label={t("history.filters.node")}
        value={nodeId ?? all}
        onChange={(next) => update("nodeId", next)}
        options={[
          [all, t("history.filters.allNodes")],
          ...nodes.map((node) => [node.id, node.name] as const),
        ]}
      />
      <FilterSelect
        label={t("history.filters.egress")}
        value={value(search, "egressId") ?? all}
        onChange={(next) => update("egressId", next)}
        disabled={!nodeId}
        options={[
          [all, t("history.filters.allEgresses")],
          ...publicAddresses.map(
            (address) => [address.id, address.address] as const,
          ),
        ]}
      />
      <div className="space-y-2">
        <Label htmlFor="history-from">{t("history.filters.from")}</Label>
        <Input
          id="history-from"
          type="datetime-local"
          value={value(search, "from") ?? ""}
          onChange={(event) => update("from", event.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="history-to">{t("history.filters.to")}</Label>
        <Input
          id="history-to"
          type="datetime-local"
          value={value(search, "to") ?? ""}
          onChange={(event) => update("to", event.target.value)}
        />
      </div>
      {tab === "reports" ? (
        <>
          <FilterSelect
            label={t("history.filters.runStatus")}
            value={value(search, "runStatus") ?? all}
            onChange={(next) => update("runStatus", next)}
            options={[
              [all, t("history.filters.allResults")],
              ["succeeded", t("probe.state.succeeded")],
              ["partial", t("probe.state.partial")],
              ["failed", t("probe.state.failed")],
              ["running", t("probe.state.running")],
            ]}
          />
          <FilterSelect
            label={t("history.filters.trigger")}
            value={value(search, "trigger") ?? all}
            onChange={(next) => update("trigger", next)}
            options={[
              [all, t("history.filters.allTriggers")],
              ["manual", t("probe.trigger.manual")],
              ["schedule", t("probe.trigger.schedule")],
              ["address-change", t("probe.trigger.address-change")],
            ]}
          />
          <FilterSelect
            label={t("history.filters.changes")}
            value={value(search, "changed") ?? all}
            onChange={(next) => update("changed", next)}
            options={[
              [all, t("history.filters.allChanges")],
              ["true", t("history.filters.changed")],
              ["false", t("history.filters.unchanged")],
            ]}
          />
          <FilterSelect
            label={t("history.filters.format")}
            value={value(search, "formatStatus") ?? all}
            onChange={(next) => update("formatStatus", next)}
            options={[
              [all, t("history.filters.allFormats")],
              ["compatible", t("history.format.compatible")],
              ["mismatch", t("history.format.mismatch")],
            ]}
          />
        </>
      ) : (
        <>
          <FilterSelect
            label={t("history.filters.eventKind")}
            value={value(search, "eventKind") ?? all}
            onChange={(next) => update("eventKind", next)}
            options={[
              [all, t("history.filters.allEvents")],
              [
                "first-observation",
                t("network.addressHistory.kind.first-observation"),
              ],
              [
                "address-change",
                t("network.addressHistory.kind.address-change"),
              ],
              ["check-failure", t("network.addressHistory.kind.check-failure")],
              ["recovery", t("network.addressHistory.kind.recovery")],
            ]}
          />
          <FilterSelect
            label={t("history.filters.family")}
            value={value(search, "family") ?? all}
            onChange={(next) => update("family", next)}
            options={[
              [all, t("history.filters.allFamilies")],
              ["ipv4", "IPv4"],
              ["ipv6", "IPv6"],
            ]}
          />
        </>
      )}
    </div>
  );
}

function FilterSelect({
  label,
  value: selected,
  options,
  onChange,
  disabled,
}: {
  label: string;
  value: string;
  options: ReadonlyArray<readonly [string, string]>;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const id = `filter-${label.replaceAll(" ", "-")}`;
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <Select value={selected} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger id={id} className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map(([optionValue, text]) => (
            <SelectItem key={optionValue} value={optionValue}>
              {text}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function ReportHistory({
  page,
  gaps,
  formatEvents,
  currentPage,
  currentGapPage,
  currentFormatPage,
  hasFilters,
  update,
  clearFilters,
  language,
}: {
  page: ProbeSnapshotHistoryPage;
  gaps: ProbeHistoryGapPage;
  formatEvents: ProbeFormatEventPage;
  currentPage: number;
  currentGapPage: number;
  currentFormatPage: number;
  hasFilters: boolean;
  update: (name: string, value?: string) => void;
  clearFilters: () => void;
  language?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("history.reports.title")}</CardTitle>
          <CardDescription>
            {t("history.reports.count", { count: page.total })}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {page.items.length === 0 ? (
            <EmptyHistory filtered={hasFilters} clear={clearFilters} />
          ) : (
            <>
              <div className="hidden md:block">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("history.columns.owner")}</TableHead>
                      <TableHead>{t("history.columns.result")}</TableHead>
                      <TableHead>
                        {t("history.columns.interpretation")}
                      </TableHead>
                      <TableHead>{t("history.columns.time")}</TableHead>
                      <TableHead className="text-right">
                        {t("history.columns.actions")}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {page.items.map((snapshot) => (
                      <ReportRow
                        key={snapshot.id}
                        snapshot={snapshot}
                        language={language}
                      />
                    ))}
                  </TableBody>
                </Table>
              </div>
              <div className="space-y-3 md:hidden">
                {page.items.map((snapshot) => (
                  <ReportMobileCard
                    key={snapshot.id}
                    snapshot={snapshot}
                    language={language}
                  />
                ))}
              </div>
            </>
          )}
        </CardContent>
      </Card>
      <Pagination current={currentPage} total={page.total} update={update} />
      {gaps.total > 0 ? (
        <ProbeGapCard gaps={gaps.items} language={language} />
      ) : null}
      <Pagination
        current={currentGapPage}
        total={gaps.total}
        update={update}
        pageParameter="gapPage"
      />
      {formatEvents.total > 0 ? (
        <FormatEventCard events={formatEvents.items} language={language} />
      ) : null}
      <Pagination
        current={currentFormatPage}
        total={formatEvents.total}
        update={update}
        pageParameter="formatPage"
      />
    </div>
  );
}

function ReportRow(props: ReportItemProps) {
  const { snapshot, language } = props;
  const { t } = useTranslation();
  return (
    <TableRow>
      <TableCell>
        <Owner snapshot={snapshot} />
      </TableCell>
      <TableCell>
        <div className="flex flex-wrap gap-1">
          <Badge
            variant={
              snapshot.runStatus === "failed" ? "destructive" : "outline"
            }
          >
            {t(`probe.state.${snapshot.runStatus}`)}
          </Badge>
          <Badge variant="secondary">
            {t(`probe.trigger.${snapshot.trigger}`)}
          </Badge>
        </div>
      </TableCell>
      <TableCell>
        <InterpretationBadges snapshot={snapshot} />
      </TableCell>
      <TableCell>
        {formatTime(snapshot.observedAt, language, t("probe.notAvailable"))}
      </TableCell>
      <TableCell>
        <ReportActions {...props} />
      </TableCell>
    </TableRow>
  );
}

function ReportMobileCard(props: ReportItemProps) {
  const { snapshot, language } = props;
  const { t } = useTranslation();
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>
          <Owner snapshot={snapshot} />
        </CardTitle>
        <CardDescription>
          {formatTime(snapshot.observedAt, language, t("probe.notAvailable"))}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap gap-1">
          <Badge variant="outline">
            {t(`probe.state.${snapshot.runStatus}`)}
          </Badge>
          <Badge variant="secondary">
            {t(`probe.trigger.${snapshot.trigger}`)}
          </Badge>
          <InterpretationBadges snapshot={snapshot} />
        </div>
        <ReportActions {...props} mobile />
      </CardContent>
    </Card>
  );
}

type ReportItemProps = {
  snapshot: Snapshot;
  language?: string;
};

function ReportActions({
  snapshot,
  mobile,
}: ReportItemProps & { mobile?: boolean }) {
  const { t } = useTranslation();
  const className = mobile ? "flex flex-wrap gap-2" : "flex justify-end gap-1";
  return (
    <div className={className}>
      <Button variant="outline" size="sm" asChild>
        <Link to={`/probe-snapshots/${snapshot.id}?runId=${snapshot.runId}`}>
          {t("history.reports.open")}
        </Link>
      </Button>
      <Button variant="ghost" size="sm" asChild>
        <Link to={`/history/compare?egress=${snapshot.egressId}`}>
          <GitCompareArrows data-icon="inline-start" aria-hidden="true" />
          {t("history.reports.compare")}
        </Link>
      </Button>
    </div>
  );
}

function Owner({ snapshot }: { snapshot: Snapshot }) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-1.5 font-medium">
        {snapshot.starred ? (
          <Star
            className="size-3.5 fill-current"
            aria-label={t("history.starred")}
          />
        ) : null}
        <span className="truncate">
          {snapshot.owner.nodeName ?? shortID(snapshot.nodeId)}
        </span>
      </div>
      <div className="truncate text-xs text-muted-foreground">
        {historyEgressName(snapshot.owner.egressName, t)} · #{snapshot.sequence}
      </div>
    </div>
  );
}

function InterpretationBadges({ snapshot }: { snapshot: Snapshot }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-1">
      {snapshot.baseline ? (
        <Badge variant="outline">{t("history.reports.baseline")}</Badge>
      ) : snapshot.changeCount > 0 ? (
        <Badge>
          {t("history.reports.changeCount", { count: snapshot.changeCount })}
        </Badge>
      ) : (
        <Badge variant="outline">{t("history.reports.noChanges")}</Badge>
      )}
      {snapshot.formatStatus === "mismatch" ? (
        <Badge variant="destructive">
          {t("history.reports.formatIssues", {
            count: snapshot.formatIssueCount,
          })}
        </Badge>
      ) : null}
      {snapshot.current ? (
        <Badge variant="secondary">{t("history.reports.current")}</Badge>
      ) : null}
    </div>
  );
}

function AddressHistory({
  page,
  currentPage,
  currentGapPage,
  hasFilters,
  update,
  clearFilters,
  language,
}: {
  page: AddressHistoryPage;
  currentPage: number;
  currentGapPage: number;
  hasFilters: boolean;
  update: (name: string, value?: string) => void;
  clearFilters: () => void;
  language?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("history.addresses.title")}</CardTitle>
          <CardDescription>
            {t("history.addresses.count", { count: page.total })}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {page.events.length === 0 ? (
            <EmptyHistory filtered={hasFilters} clear={clearFilters} />
          ) : (
            <>
              <div className="hidden md:block">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("history.columns.owner")}</TableHead>
                      <TableHead>{t("history.columns.event")}</TableHead>
                      <TableHead>{t("history.columns.address")}</TableHead>
                      <TableHead>{t("history.columns.time")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {page.events.map((item) => (
                      <AddressRow
                        key={item.event.id}
                        item={item}
                        language={language}
                      />
                    ))}
                  </TableBody>
                </Table>
              </div>
              <div className="space-y-3 md:hidden">
                {page.events.map((item) => (
                  <AddressMobileCard
                    key={item.event.id}
                    item={item}
                    language={language}
                  />
                ))}
              </div>
            </>
          )}
        </CardContent>
      </Card>
      <Pagination current={currentPage} total={page.total} update={update} />
      {page.gapTotal > 0 ? (
        <AddressGapCard gaps={page.gaps} language={language} />
      ) : null}
      <Pagination
        current={currentGapPage}
        total={page.gapTotal}
        update={update}
        pageParameter="gapPage"
      />
    </div>
  );
}

function AddressRow({
  item,
  language,
}: {
  item: AddressEvent;
  language?: string;
}) {
  const { t } = useTranslation();
  return (
    <TableRow>
      <TableCell>
        <div className="font-medium">
          {item.owner.nodeName ?? shortID(item.nodeId)}
        </div>
        <div className="text-xs text-muted-foreground">
          {addressEventOwner(item, t)}
        </div>
      </TableCell>
      <TableCell>
        <AddressEventBadges item={item} />
      </TableCell>
      <TableCell>
        <AddressMapping item={item} />
      </TableCell>
      <TableCell>
        {formatTime(item.event.observedAt, language, t("probe.notAvailable"))}
      </TableCell>
    </TableRow>
  );
}

function AddressMobileCard({
  item,
  language,
}: {
  item: AddressEvent;
  language?: string;
}) {
  const { t } = useTranslation();
  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{item.owner.nodeName ?? shortID(item.nodeId)}</CardTitle>
        <CardDescription>
          {addressEventOwner(item, t)} ·{" "}
          {formatTime(item.event.observedAt, language, t("probe.notAvailable"))}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <AddressEventBadges item={item} />
        <AddressMapping item={item} />
      </CardContent>
    </Card>
  );
}

function AddressEventBadges({ item }: { item: AddressEvent }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-1">
      <Badge
        variant={
          item.event.kind === "check-failure" ? "destructive" : "outline"
        }
      >
        {t(`network.addressHistory.kind.${item.event.kind}`)}
      </Badge>
      <Badge variant="secondary">{item.event.family.toUpperCase()}</Badge>
    </div>
  );
}

function AddressMapping({ item }: { item: AddressEvent }) {
  const { t } = useTranslation();
  if (item.event.kind === "check-failure") {
    return (
      <span className="text-destructive">
        {item.event.failureReason
          ? t(`network.observation.failure.${item.event.failureReason}`)
          : t("probe.notAvailable")}
      </span>
    );
  }
  const current = item.event.publicAddress ?? t("probe.notAvailable");
  return (
    <div className="min-w-0 break-all font-mono text-xs">
      {item.event.previousAddress
        ? `${item.event.previousAddress} -> ${current}`
        : current}
    </div>
  );
}

function ProbeGapCard({
  gaps,
  language,
}: {
  gaps: ProbeHistoryGap[];
  language?: string;
}) {
  const { t } = useTranslation();
  return (
    <Card className="ring-destructive/30">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-destructive">
          <CircleAlert className="size-4" aria-hidden="true" />
          {t("history.gaps.probeTitle")}
        </CardTitle>
        <CardDescription>{t("history.gaps.detail")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {gaps.map((gap) => (
          <div key={gap.id} className="rounded-md border p-3">
            <div className="font-medium">
              {gap.owner.nodeName ?? shortID(gap.nodeId)} ·{" "}
              {historyEgressName(gap.owner.egressName, t)}
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              {t("history.gaps.probeItem", {
                count: gap.droppedCount,
                first: gap.firstSequence,
                last: gap.lastSequence,
              })}{" "}
              ·{" "}
              {formatTime(
                gap.lastObservedAt,
                language,
                t("probe.notAvailable"),
              )}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function FormatEventCard({
  events,
  language,
}: {
  events: ProbeFormatEvent[];
  language?: string;
}) {
  const { t } = useTranslation();
  return (
    <Card className="ring-destructive/30">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-destructive">
          <TriangleAlert className="size-4" aria-hidden="true" />
          {t("history.formatEvents.title")}
        </CardTitle>
        <CardDescription>{t("history.formatEvents.detail")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {events.map((event) => (
          <div
            key={event.id}
            className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-3"
          >
            <div>
              <div className="font-medium">
                {event.owner.nodeName ?? shortID(event.nodeId)} ·{" "}
                {historyEgressName(event.owner.egressName, t)}
              </div>
              <div className="mt-1 text-xs text-muted-foreground">
                {t(`history.formatEvents.kind.${event.kind}`)} ·{" "}
                {t("history.formatEvents.issueCount", {
                  count: event.issues.length,
                })}{" "}
                ·{" "}
                {formatTime(
                  event.observedAt,
                  language,
                  t("probe.notAvailable"),
                )}
              </div>
            </div>
            <Button variant="outline" size="sm" asChild>
              <Link to={`/probe-snapshots/${event.snapshotId}`}>
                {t("history.reports.open")}
              </Link>
            </Button>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function AddressGapCard({
  gaps,
  language,
}: {
  gaps: AddressHistoryPage["gaps"];
  language?: string;
}) {
  const { t } = useTranslation();
  return (
    <Card className="ring-destructive/30">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-destructive">
          <CircleAlert className="size-4" aria-hidden="true" />
          {t("history.gaps.addressTitle")}
        </CardTitle>
        <CardDescription>{t("history.gaps.detail")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {gaps.map((item) => (
          <div key={item.gap.id} className="rounded-md border p-3">
            <div className="font-medium">
              {item.owner.nodeName ?? shortID(item.nodeId)}
              <Badge variant="secondary" className="ml-2">
                {t("history.gaps.nodeLevel")}
              </Badge>
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              {t("history.gaps.addressItem", {
                count: item.gap.droppedCount,
                first: item.gap.firstSequence,
                last: item.gap.lastSequence,
              })}{" "}
              ·{" "}
              {formatTime(
                item.gap.lastObservedAt,
                language,
                t("probe.notAvailable"),
              )}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function addressEventOwner(
  item: AddressEvent,
  t: ReturnType<typeof useTranslation>["t"],
) {
  return (
    item.owner.egressName ??
    item.event.publicAddress ??
    item.event.previousAddress ??
    t("probe.notAvailable")
  );
}

function Pagination({
  current,
  total,
  update,
  pageParameter = "page",
}: {
  current: number;
  total: number;
  update: (name: string, value?: string) => void;
  pageParameter?: string;
}) {
  const { t } = useTranslation();
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (total <= pageSize) return null;
  return (
    <Card size="sm">
      <CardContent className="flex items-center justify-between gap-3">
        <Button
          variant="outline"
          size="sm"
          disabled={current <= 1}
          onClick={() => update(pageParameter, String(current - 1))}
        >
          <ArrowLeft data-icon="inline-start" aria-hidden="true" />
          {t("history.pagination.previous")}
        </Button>
        <span className="text-xs text-muted-foreground">
          {t("history.pagination.page", { current, total: pages })}
        </span>
        <Button
          variant="outline"
          size="sm"
          disabled={current >= pages}
          onClick={() => update(pageParameter, String(current + 1))}
        >
          {t("history.pagination.next")}
          <ArrowRight data-icon="inline-end" aria-hidden="true" />
        </Button>
      </CardContent>
    </Card>
  );
}

function EmptyHistory({
  filtered,
  clear,
}: {
  filtered: boolean;
  clear: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-48 flex-col items-center justify-center text-center">
      <SearchX className="size-8 text-muted-foreground" aria-hidden="true" />
      <p className="mt-3 font-medium">
        {t(filtered ? "history.empty.filtered" : "history.empty.none")}
      </p>
      {filtered ? (
        <Button variant="outline" size="sm" className="mt-4" onClick={clear}>
          {t("history.filters.clear")}
        </Button>
      ) : null}
    </div>
  );
}

function HistorySkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-64 max-w-full" />
      </CardHeader>
      <CardContent className="space-y-3">
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className="h-14 w-full" />
        ))}
      </CardContent>
    </Card>
  );
}

function value(search: URLSearchParams, key: string) {
  const result = search.get(key);
  return result === null || result === "" ? undefined : result;
}

function positiveInteger(raw: string | null, fallback: number) {
  const parsed = Number(raw);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function toISO(raw?: string) {
  if (!raw) return undefined;
  const date = new Date(raw);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}

function shortID(id: string) {
  return id.slice(0, 8);
}

function historyEgressName(
  name: string | undefined,
  t: ReturnType<typeof useTranslation>["t"],
) {
  return name ?? t("history.addressUnavailable");
}
