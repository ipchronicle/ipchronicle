import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  Cable,
  CirclePlus,
  Clock3,
  Globe2,
  History,
  LoaderCircle,
  Network,
  RefreshCw,
  Route,
  Save,
  Trash2,
  TriangleAlert,
  Waypoints,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";

import {
  createNodeEgress,
  deleteNodeEgress,
  getNodeNetwork,
  updateNodeEgress,
  type NetworkEgress,
  type NetworkEgressCandidate,
  type NetworkEgressUpdate,
  type NodeNetworkState,
} from "@/api/network";
import { listNodes, type Node } from "@/api/nodes";
import { listNetworkProxies, type NetworkProxy } from "@/api/proxies";
import { useAuth } from "@/auth-context";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatAPIError } from "@/lib/api-error";

type ViewState =
  | { kind: "loading" }
  | {
      kind: "success";
      node: Node;
      network: NodeNetworkState;
      proxies: NetworkProxy[];
    }
  | { kind: "not-found" }
  | { kind: "error" };

export function NodeNetworkPage() {
  const { nodeId = "" } = useParams();
  const { t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);
  const [feedback, setFeedback] = useState<string>();

  const load = useCallback(
    async (signal?: AbortSignal, initial = false) => {
      if (initial) setState({ kind: "loading" });
      else setRefreshing(true);
      try {
        const [nodes, network, proxies] = await Promise.all([
          listNodes(signal),
          getNodeNetwork(nodeId, signal),
          listNetworkProxies(signal),
        ]);
        const node = nodes.find((item) => item.id === nodeId);
        setState(
          node
            ? { kind: "success", node, network, proxies }
            : { kind: "not-found" },
        );
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        setState({ kind: "error" });
      } finally {
        setRefreshing(false);
      }
    },
    [nodeId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    return () => controller.abort();
  }, [load]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  function replaceEgress(egress: NetworkEgress) {
    setState((current) =>
      current.kind === "success"
        ? {
            ...current,
            network: {
              ...current.network,
              egresses: current.network.egresses.map((item) =>
                item.id === egress.id ? egress : item,
              ),
            },
          }
        : current,
    );
  }

  async function updateEgress(
    egress: NetworkEgress,
    update: NetworkEgressUpdate,
  ) {
    setFeedback(undefined);
    try {
      replaceEgress(
        await updateNodeEgress(nodeId, egress.id, update, csrfToken),
      );
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    }
  }

  async function addCandidate(candidate: NetworkEgressCandidate) {
    setFeedback(undefined);
    try {
      await createNodeEgress(
        nodeId,
        {
          kind: candidate.kind,
          family: candidate.family,
          interfaceName: candidate.interfaceName,
          sourceAddress: candidate.sourceAddress,
        },
        csrfToken,
      );
      await load();
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    }
  }

  async function addProxyEgress(proxyId: string, family: "ipv4" | "ipv6") {
    setFeedback(undefined);
    try {
      await createNodeEgress(
        nodeId,
        { kind: "proxy", family, proxyId },
        csrfToken,
      );
      await load();
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    }
  }

  async function removeEgress(egress: NetworkEgress) {
    setFeedback(undefined);
    try {
      const deletion = await deleteNodeEgress(nodeId, egress.id, csrfToken);
      replaceEgress({
        ...egress,
        deletionStatus: deletion.status,
        deletionError: deletion.error,
      });
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    }
  }

  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0 max-w-2xl">
          <Button variant="ghost" size="sm" asChild className="mb-3 -ml-3">
            <Link to="/nodes">
              <ArrowLeft data-icon="inline-start" aria-hidden="true" />
              {t("network.back")}
            </Link>
          </Button>
          <p className="text-xs font-medium text-muted-foreground uppercase">
            {t("network.section")}
          </p>
          <h1 className="mt-2 truncate text-2xl font-semibold sm:text-3xl">
            {state.kind === "success" ? state.node.name : t("network.title")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("network.detail")}
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
          {t("network.refresh")}
        </Button>
      </div>

      <div className="mt-8 space-y-4" aria-live="polite">
        {state.kind === "loading" ? <NetworkSkeleton /> : null}
        {state.kind === "not-found" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("network.nodeNotFound")}</AlertTitle>
          </Alert>
        ) : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("network.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => void load(undefined, true)}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("network.retry")}
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
          <>
            <EgressCard
              egresses={state.network.egresses}
              addressStates={state.network.addressStates}
              proxies={state.proxies}
              onUpdate={updateEgress}
              onDelete={removeEgress}
            />
            <AddressHistoryCard network={state.network} />
            <ProxyEgressCard
              proxies={state.proxies}
              egresses={state.network.egresses}
              onAdd={addProxyEgress}
            />
            <CandidateCard
              candidates={state.network.candidates}
              onAdd={addCandidate}
            />
            <InventoryCards network={state.network} />
          </>
        ) : null}
      </div>
    </main>
  );
}

function EgressCard({
  egresses,
  addressStates,
  proxies,
  onUpdate,
  onDelete,
}: {
  egresses: NetworkEgress[];
  addressStates: NodeNetworkState["addressStates"];
  proxies: NetworkProxy[];
  onUpdate: (
    egress: NetworkEgress,
    update: NetworkEgressUpdate,
  ) => Promise<void>;
  onDelete: (egress: NetworkEgress) => Promise<void>;
}) {
  const { t } = useTranslation();

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Globe2 aria-hidden="true" className="size-4" />
          {t("network.egresses.title")}
        </CardTitle>
        <CardDescription>{t("network.egresses.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">{egresses.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        {egresses.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("network.egresses.empty")}
          </p>
        ) : (
          egresses.map((egress) => (
            <EgressItem
              key={egress.id}
              egress={egress}
              addressState={addressStates.find(
                (item) => item.egressId === egress.id,
              )}
              proxies={proxies}
              onUpdate={onUpdate}
              onDelete={onDelete}
            />
          ))
        )}
      </CardContent>
    </Card>
  );
}

function EgressItem({
  egress,
  addressState,
  proxies,
  onUpdate,
  onDelete,
}: {
  egress: NetworkEgress;
  addressState: NodeNetworkState["addressStates"][number] | undefined;
  proxies: NetworkProxy[];
  onUpdate: (
    egress: NetworkEgress,
    update: NetworkEgressUpdate,
  ) => Promise<void>;
  onDelete: (egress: NetworkEgress) => Promise<void>;
}) {
  const { i18n, t } = useTranslation();
  const [interval, setInterval] = useState(
    String(egress.lightweightIntervalSeconds),
  );
  const [working, setWorking] = useState(false);

  useEffect(() => {
    setInterval(String(egress.lightweightIntervalSeconds));
  }, [egress.lightweightIntervalSeconds]);

  async function update(update: NetworkEgressUpdate) {
    setWorking(true);
    try {
      await onUpdate(egress, update);
    } finally {
      setWorking(false);
    }
  }

  const common = {
    enabled: egress.enabled,
    lightweightIntervalSeconds: egress.lightweightIntervalSeconds,
    probeOnAddressChange: egress.probeOnAddressChange,
  };
  const deletionPending = egress.deletionStatus === "pending";
  const deletionFailed = egress.deletionStatus === "failed";
  const deletionActive = deletionPending || deletionFailed;

  return (
    <div className="space-y-4 rounded-md border p-4">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <p className="font-medium">{egressLabel(egress, proxies, t)}</p>
            <Badge variant={egress.available ? "outline" : "destructive"}>
              {egress.available
                ? t("network.egresses.available")
                : t("network.egresses.unavailable")}
            </Badge>
            {egress.automatic ? (
              <Badge variant="secondary">
                {t("network.egresses.automatic")}
              </Badge>
            ) : null}
            {deletionActive ? (
              <Badge variant="destructive">
                <Trash2 aria-hidden="true" />
                {deletionFailed
                  ? t("network.egresses.deletionFailed")
                  : t("network.egresses.deletionPending")}
              </Badge>
            ) : null}
          </div>
          <p className="mt-1 break-all text-xs text-muted-foreground">
            {t(`network.family.${egress.family}`)} · {egress.id}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {!deletionPending ? (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  disabled={working}
                  aria-label={t(
                    deletionFailed
                      ? "network.egresses.retryDeletion"
                      : "network.egresses.delete",
                  )}
                >
                  {deletionFailed ? (
                    <RefreshCw aria-hidden="true" />
                  ) : (
                    <Trash2 aria-hidden="true" />
                  )}
                </Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogMedia>
                    <TriangleAlert aria-hidden="true" />
                  </AlertDialogMedia>
                  <AlertDialogTitle>
                    {t("network.egresses.deleteTitle")}
                  </AlertDialogTitle>
                  <AlertDialogDescription>
                    {t("network.egresses.deleteDetail")}
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    onClick={() => {
                      setWorking(true);
                      void onDelete(egress).finally(() => setWorking(false));
                    }}
                  >
                    {t(
                      deletionFailed
                        ? "network.egresses.retryDeletion"
                        : "network.egresses.deleteConfirm",
                    )}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : null}
          <Switch
            checked={egress.enabled}
            disabled={working || deletionActive}
            aria-label={t("network.egresses.enabledLabel", {
              name: egressLabel(egress, proxies, t),
            })}
            onCheckedChange={(enabled) => void update({ ...common, enabled })}
          />
        </div>
      </div>

      {deletionFailed && egress.deletionError ? (
        <Alert variant="destructive">
          <TriangleAlert aria-hidden="true" />
          <AlertTitle>{t("network.egresses.deletionFailed")}</AlertTitle>
          <AlertDescription>{egress.deletionError}</AlertDescription>
        </Alert>
      ) : null}

      <AddressSummary state={addressState} locale={i18n.language} />

      <div className="grid items-end gap-4 border-t pt-4 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]">
        <div className="space-y-2">
          <Label htmlFor={`interval-${egress.id}`}>
            {t("network.egresses.interval")}
          </Label>
          <div className="flex gap-2">
            <Input
              id={`interval-${egress.id}`}
              type="number"
              min={1}
              max={9223372036}
              value={interval}
              disabled={working || deletionActive}
              onChange={(event) => setInterval(event.target.value)}
            />
            <Button
              size="icon"
              variant="outline"
              disabled={working || deletionActive || Number(interval) < 1}
              aria-label={t("network.egresses.saveInterval")}
              onClick={() =>
                void update({
                  ...common,
                  lightweightIntervalSeconds: Number(interval),
                })
              }
            >
              <Save aria-hidden="true" />
            </Button>
          </div>
        </div>
        <div className="flex items-center justify-between gap-4 rounded-md border px-3 py-2">
          <div>
            <Label htmlFor={`change-probe-${egress.id}`}>
              {t("network.egresses.probeOnChange")}
            </Label>
            <p className="text-xs text-muted-foreground">
              {t("network.egresses.probeOnChangeDetail")}
            </p>
          </div>
          <Switch
            id={`change-probe-${egress.id}`}
            checked={egress.probeOnAddressChange}
            disabled={working || deletionActive}
            onCheckedChange={(probeOnAddressChange) =>
              void update({ ...common, probeOnAddressChange })
            }
          />
        </div>
      </div>
    </div>
  );
}

function AddressSummary({
  state,
  locale,
}: {
  state: NodeNetworkState["addressStates"][number] | undefined;
  locale: string;
}) {
  const { t } = useTranslation();
  if (!state) {
    return (
      <div className="rounded-md bg-muted/40 px-3 py-3 text-sm text-muted-foreground">
        {t("network.observation.waiting")}
      </div>
    );
  }
  const mapping = state.proxyPath
    ? (state.publicAddress ?? t("network.observation.unknown"))
    : state.localAddress && state.publicAddress
      ? `${state.localInterface} · ${state.localAddress}${state.localAddress === state.publicAddress ? "" : ` -> ${state.publicAddress}`}`
      : (state.publicAddress ?? t("network.observation.unknown"));
  return (
    <div className="space-y-3 rounded-md bg-muted/40 px-3 py-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="break-all font-mono text-sm">{mapping}</span>
        <Badge variant={state.status === "failed" ? "destructive" : "outline"}>
          {t(`network.observation.status.${state.status}`)}
        </Badge>
        {state.proxyPath ? (
          <Badge variant="secondary">{t("network.observation.proxy")}</Badge>
        ) : null}
        {state.likelyNat ? (
          <Badge variant="destructive">{t("network.observation.nat")}</Badge>
        ) : null}
        {state.temporary ? (
          <Badge variant="secondary">
            {t("network.observation.temporary")}
          </Badge>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1">
          <Clock3 aria-hidden="true" className="size-3" />
          {new Date(state.lastCheckedAt).toLocaleString(locale)}
        </span>
        {state.failureReason ? (
          <span>{t(`network.observation.failure.${state.failureReason}`)}</span>
        ) : null}
      </div>
      {state.likelyNat ? (
        <Alert>
          <TriangleAlert aria-hidden="true" />
          <AlertDescription>
            {t("network.observation.natDetail")}
          </AlertDescription>
        </Alert>
      ) : null}
    </div>
  );
}

function AddressHistoryCard({ network }: { network: NodeNetworkState }) {
  const { i18n, t } = useTranslation();
  const items = [
    ...network.addressEvents.map((event) => ({
      type: "event" as const,
      id: event.id,
      time: event.observedAt,
      event,
    })),
    ...network.addressGaps.map((gap) => ({
      type: "gap" as const,
      id: gap.id,
      time: gap.lastObservedAt,
      gap,
    })),
  ].sort((left, right) => right.time.localeCompare(left.time));

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <History aria-hidden="true" className="size-4" />
          {t("network.addressHistory.title")}
        </CardTitle>
        <CardDescription>{t("network.addressHistory.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">{items.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        {items.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("network.addressHistory.empty")}
          </p>
        ) : (
          items.map((item) =>
            item.type === "event" ? (
              <div
                key={item.id}
                className="flex flex-wrap items-start justify-between gap-3 rounded-md border p-3"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge
                      variant={
                        item.event.kind === "check-failure"
                          ? "destructive"
                          : "outline"
                      }
                    >
                      {t(`network.addressHistory.kind.${item.event.kind}`)}
                    </Badge>
                    <span className="break-all font-mono text-xs">
                      {eventMapping(item.event, t)}
                    </span>
                  </div>
                  {item.event.failureReason ? (
                    <p className="mt-2 text-xs text-muted-foreground">
                      {t(
                        `network.observation.failure.${item.event.failureReason}`,
                      )}
                    </p>
                  ) : null}
                </div>
                <time className="text-xs whitespace-nowrap text-muted-foreground">
                  {new Date(item.event.observedAt).toLocaleString(
                    i18n.language,
                  )}
                </time>
              </div>
            ) : (
              <div
                key={item.id}
                className="flex flex-wrap items-start justify-between gap-3 rounded-md border border-destructive/40 p-3"
              >
                <div>
                  <Badge variant="destructive">
                    {t("network.addressHistory.gap")}
                  </Badge>
                  <p className="mt-2 text-sm">
                    {t("network.addressHistory.gapDetail", {
                      count: item.gap.droppedCount,
                      first: item.gap.firstSequence,
                      last: item.gap.lastSequence,
                    })}
                  </p>
                </div>
                <time className="text-xs whitespace-nowrap text-muted-foreground">
                  {new Date(item.gap.lastObservedAt).toLocaleString(
                    i18n.language,
                  )}
                </time>
              </div>
            ),
          )
        )}
      </CardContent>
    </Card>
  );
}

function eventMapping(
  event: NodeNetworkState["addressEvents"][number],
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (event.previousAddress && event.publicAddress) {
    return `${event.previousAddress} -> ${event.publicAddress}`;
  }
  if (event.proxyPath) {
    return event.publicAddress ?? t("network.observation.unknown");
  }
  if (event.localAddress && event.publicAddress) {
    return `${event.localInterface} · ${event.localAddress}${event.localAddress === event.publicAddress ? "" : ` -> ${event.publicAddress}`}`;
  }
  return event.publicAddress ?? t("network.observation.unknown");
}

function ProxyEgressCard({
  proxies,
  egresses,
  onAdd,
}: {
  proxies: NetworkProxy[];
  egresses: NetworkEgress[];
  onAdd: (proxyId: string, family: "ipv4" | "ipv6") => Promise<void>;
}) {
  const { t } = useTranslation();
  const [proxyId, setProxyId] = useState(proxies[0]?.id ?? "");
  const [family, setFamily] = useState<"ipv4" | "ipv6">("ipv4");
  const [working, setWorking] = useState(false);
  const duplicate = egresses.some(
    (egress) =>
      egress.kind === "proxy" &&
      egress.proxyId === proxyId &&
      egress.family === family,
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Waypoints aria-hidden="true" className="size-4" />
          {t("network.proxyEgress.title")}
        </CardTitle>
        <CardDescription>{t("network.proxyEgress.detail")}</CardDescription>
      </CardHeader>
      <CardContent>
        {proxies.length === 0 ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-sm text-muted-foreground">
              {t("network.proxyEgress.empty")}
            </p>
            <Button variant="outline" size="sm" asChild>
              <Link to="/settings/network">
                {t("network.proxyEgress.openSettings")}
              </Link>
            </Button>
          </div>
        ) : (
          <div className="grid items-end gap-4 sm:grid-cols-[minmax(0,1fr)_10rem_auto]">
            <div className="space-y-2">
              <span className="text-sm font-medium">
                {t("network.proxyEgress.proxy")}
              </span>
              <Select value={proxyId} onValueChange={setProxyId}>
                <SelectTrigger
                  className="w-full"
                  aria-label={t("network.proxyEgress.proxy")}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {proxies.map((proxy) => (
                    <SelectItem key={proxy.id} value={proxy.id}>
                      {proxy.name} · {proxy.scheme.toUpperCase()}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <span className="text-sm font-medium">
                {t("network.proxyEgress.family")}
              </span>
              <Select
                value={family}
                onValueChange={(value: "ipv4" | "ipv6") => setFamily(value)}
              >
                <SelectTrigger
                  className="w-full"
                  aria-label={t("network.proxyEgress.family")}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="ipv4">IPv4</SelectItem>
                  <SelectItem value="ipv6">IPv6</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button
              disabled={working || !proxyId || duplicate}
              onClick={() => {
                setWorking(true);
                void onAdd(proxyId, family).finally(() => setWorking(false));
              }}
            >
              {working ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <CirclePlus data-icon="inline-start" aria-hidden="true" />
              )}
              {duplicate
                ? t("network.proxyEgress.configured")
                : t("network.proxyEgress.add")}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function CandidateCard({
  candidates,
  onAdd,
}: {
  candidates: NetworkEgressCandidate[];
  onAdd: (candidate: NetworkEgressCandidate) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [working, setWorking] = useState<string>();
  const visible = useMemo(
    () =>
      candidates.filter(
        (candidate) => candidate.configuredEgressId === undefined,
      ),
    [candidates],
  );

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Cable aria-hidden="true" className="size-4" />
          {t("network.candidates.title")}
        </CardTitle>
        <CardDescription>{t("network.candidates.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">{visible.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        {visible.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("network.candidates.empty")}
          </p>
        ) : (
          visible.map((candidate) => {
            const key = candidateKey(candidate);
            return (
              <div
                key={key}
                className="flex flex-wrap items-center justify-between gap-4 rounded-md border p-4"
              >
                <div className="min-w-0">
                  <p className="break-all font-medium">
                    {candidateLabel(candidate, t)}
                  </p>
                  <div className="mt-2 flex flex-wrap gap-2">
                    <Badge variant="outline">
                      {t(`network.family.${candidate.family}`)}
                    </Badge>
                    {candidate.scope ? (
                      <Badge variant="secondary">
                        {t(`network.scope.${candidate.scope}`)}
                      </Badge>
                    ) : null}
                    {candidate.temporary ? (
                      <Badge variant="destructive">
                        {t("network.candidates.temporary")}
                      </Badge>
                    ) : null}
                    {!candidate.eligible && candidate.unavailableReason ? (
                      <span className="text-xs text-muted-foreground">
                        {t(`network.reason.${candidate.unavailableReason}`)}
                      </span>
                    ) : null}
                  </div>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={!candidate.eligible || working === key}
                  onClick={() => {
                    setWorking(key);
                    void onAdd(candidate).finally(() => setWorking(undefined));
                  }}
                >
                  {working === key ? (
                    <LoaderCircle
                      data-icon="inline-start"
                      aria-hidden="true"
                      className="animate-spin"
                    />
                  ) : (
                    <CirclePlus data-icon="inline-start" aria-hidden="true" />
                  )}
                  {t("network.candidates.add")}
                </Button>
              </div>
            );
          })
        )}
      </CardContent>
    </Card>
  );
}

function InventoryCards({ network }: { network: NodeNetworkState }) {
  const { i18n, t } = useTranslation();
  const inventory = network.inventory;
  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Network aria-hidden="true" className="size-4" />
            {t("network.inventory.title")}
          </CardTitle>
          <CardDescription>{t("network.inventory.detail")}</CardDescription>
          <CardAction>
            {network.inventoryReceivedAt ? (
              <Badge variant="outline">
                {new Intl.DateTimeFormat(i18n.language, {
                  dateStyle: "short",
                  timeStyle: "medium",
                }).format(new Date(network.inventoryReceivedAt))}
              </Badge>
            ) : null}
          </CardAction>
        </CardHeader>
        <CardContent>
          {network.inventoryError ? (
            <Alert variant="destructive" className="mb-4">
              <TriangleAlert aria-hidden="true" />
              <AlertTitle>{t("network.inventory.failed")}</AlertTitle>
              <AlertDescription className="break-all">
                {network.inventoryError}
              </AlertDescription>
            </Alert>
          ) : null}
          {!inventory ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t("network.inventory.empty")}
            </p>
          ) : (
            <div className="overflow-x-auto rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("network.inventory.interface")}</TableHead>
                    <TableHead>{t("network.inventory.index")}</TableHead>
                    <TableHead>{t("network.inventory.state")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {inventory.interfaces.map((item) => (
                    <TableRow key={`${item.index}:${item.name}`}>
                      <TableCell className="font-medium">{item.name}</TableCell>
                      <TableCell>{item.index}</TableCell>
                      <TableCell>
                        <Badge variant={item.up ? "outline" : "secondary"}>
                          {item.loopback
                            ? t("network.inventory.loopback")
                            : item.up
                              ? t("network.inventory.up")
                              : t("network.inventory.down")}
                        </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {inventory ? (
        <>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Globe2 aria-hidden="true" className="size-4" />
                {t("network.addresses.title")}
              </CardTitle>
              <CardDescription>{t("network.addresses.detail")}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("network.inventory.interface")}</TableHead>
                      <TableHead>{t("network.addresses.address")}</TableHead>
                      <TableHead>{t("network.addresses.scope")}</TableHead>
                      <TableHead>{t("network.addresses.lifecycle")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {inventory.addresses.map((item) => (
                      <TableRow key={`${item.interfaceName}:${item.address}`}>
                        <TableCell>{item.interfaceName}</TableCell>
                        <TableCell className="font-mono text-xs whitespace-nowrap">
                          {item.address}/{item.prefixLength}
                        </TableCell>
                        <TableCell>
                          {t(`network.scope.${item.scope}`)}
                        </TableCell>
                        <TableCell>
                          {addressLifecycle(item, t).map((label) => (
                            <Badge
                              key={label}
                              variant="secondary"
                              className="mr-1"
                            >
                              {label}
                            </Badge>
                          ))}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Route aria-hidden="true" className="size-4" />
                {t("network.routes.title")}
              </CardTitle>
              <CardDescription>{t("network.routes.detail")}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>{t("network.inventory.interface")}</TableHead>
                      <TableHead>{t("network.routes.destination")}</TableHead>
                      <TableHead>{t("network.routes.gateway")}</TableHead>
                      <TableHead>{t("network.routes.metric")}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {inventory.routes.map((item, index) => (
                      <TableRow
                        key={`${item.family}:${item.interfaceName}:${item.destination}:${item.metric}:${index}`}
                      >
                        <TableCell>{item.interfaceName}</TableCell>
                        <TableCell className="font-mono text-xs whitespace-nowrap">
                          {item.default
                            ? t(`network.routes.default.${item.family}`)
                            : item.destination}
                        </TableCell>
                        <TableCell className="font-mono text-xs whitespace-nowrap">
                          {item.gateway ?? "-"}
                        </TableCell>
                        <TableCell>{item.metric}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        </>
      ) : null}
    </>
  );
}

function egressLabel(
  egress: NetworkEgress,
  proxies: NetworkProxy[],
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (egress.kind === "default")
    return t(`network.egresses.default.${egress.family}`);
  if (egress.kind === "interface") {
    return t("network.egresses.interface", { name: egress.interfaceName });
  }
  if (egress.kind === "proxy") {
    const proxy = proxies.find((item) => item.id === egress.proxyId);
    return t("network.egresses.proxy", {
      name: proxy?.name ?? t("network.egresses.missingProxy"),
    });
  }
  return t("network.egresses.source", {
    name: egress.interfaceName,
    address: egress.sourceAddress,
  });
}

function candidateLabel(
  candidate: NetworkEgressCandidate,
  t: ReturnType<typeof useTranslation>["t"],
) {
  return candidate.kind === "interface"
    ? t("network.candidates.interface", { name: candidate.interfaceName })
    : t("network.candidates.source", {
        name: candidate.interfaceName,
        address: candidate.sourceAddress,
      });
}

function candidateKey(candidate: NetworkEgressCandidate) {
  return `${candidate.kind}:${candidate.family}:${candidate.interfaceName}:${candidate.sourceAddress ?? ""}`;
}

function addressLifecycle(
  address: NonNullable<NodeNetworkState["inventory"]>["addresses"][number],
  t: ReturnType<typeof useTranslation>["t"],
) {
  const labels: string[] = [];
  if (address.temporary) labels.push(t("network.addresses.temporary"));
  if (address.tentative) labels.push(t("network.addresses.tentative"));
  if (address.deprecated) labels.push(t("network.addresses.deprecated"));
  if (address.duplicate) labels.push(t("network.addresses.duplicate"));
  if (labels.length === 0) labels.push(t("network.addresses.stable"));
  return labels;
}

function NetworkSkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-72 max-w-full" />
      </CardHeader>
      <CardContent className="space-y-3">
        <Skeleton className="h-20 w-full" />
        <Skeleton className="h-20 w-full" />
      </CardContent>
    </Card>
  );
}
