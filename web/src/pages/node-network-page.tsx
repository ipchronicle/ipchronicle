import { useCallback, useEffect, useState } from "react";
import {
  Eye,
  Globe2,
  LoaderCircle,
  Play,
  RefreshCw,
  Route,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";

import {
  getNodeNetwork,
  updatePublicAddress,
  type NodeNetworkState,
  type PublicAddress,
} from "@/api/network";
import {
  createNetworkProxy,
  deleteNetworkProxy,
  updateNetworkProxy,
  type NetworkProxyCreate,
  type NetworkProxyUpdate,
} from "@/api/proxies";
import type { ProbeTask } from "@/api/probes";
import { useAuth } from "@/auth-context";
import { CompleteProbeDialog } from "@/components/complete-probe-dialog";
import { useNodeDetail } from "@/components/node-detail-layout";
import { NodeNetworkProxies } from "@/components/node-network-proxies";
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
import { Switch } from "@/components/ui/switch";
import { formatAPIError } from "@/lib/api-error";
import { publicAddressAvailability } from "@/lib/public-address";
import { formatTime } from "@/pages/node-probe-page";

const refreshIntervalMilliseconds = 5_000;

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; network: NodeNetworkState }
  | { kind: "error" };

type Feedback = { kind: "success" | "error"; message: string };

export function NodeNetworkPage() {
  const { nodeId = "" } = useParams();
  const { node } = useNodeDetail();
  const { t, i18n } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);
  const [saving, setSaving] = useState<Set<string>>(() => new Set());
  const [feedback, setFeedback] = useState<Feedback>();

  const load = useCallback(
    async (signal?: AbortSignal, initial = false, quiet = false) => {
      if (initial) setState({ kind: "loading" });
      else if (!quiet) setRefreshing(true);
      try {
        setState({
          kind: "success",
          network: await getNodeNetwork(nodeId, signal),
        });
      } catch (error) {
        if (error instanceof DOMException && error.name === "AbortError")
          return;
        if (!quiet) setState({ kind: "error" });
      } finally {
        if (!quiet) setRefreshing(false);
      }
    },
    [nodeId],
  );

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    if (state.kind !== "success") return;
    let active = true;
    let controller: AbortController | undefined;
    const refresh = () => {
      if (!active || document.visibilityState !== "visible") return;
      controller?.abort();
      controller = new AbortController();
      void load(controller.signal, false, true);
    };
    const timer = window.setInterval(refresh, refreshIntervalMilliseconds);
    document.addEventListener("visibilitychange", refresh);
    window.addEventListener("focus", refresh);
    return () => {
      active = false;
      controller?.abort();
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", refresh);
      window.removeEventListener("focus", refresh);
    };
  }, [load, state.kind]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function saveAddress(address: PublicAddress, probeEnabled: boolean) {
    setSaving((current) => new Set(current).add(address.id));
    setFeedback(undefined);
    try {
      const updated = await updatePublicAddress(
        nodeId,
        address.id,
        { probeEnabled },
        csrfToken,
      );
      setState((current) =>
        current.kind === "success"
          ? {
              kind: "success",
              network: {
                ...current.network,
                publicAddresses: current.network.publicAddresses.map((item) =>
                  item.id === updated.id ? updated : item,
                ),
              },
            }
          : current,
      );
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setSaving((current) => {
        const next = new Set(current);
        next.delete(address.id);
        return next;
      });
    }
  }

  function probeCreated(_task: ProbeTask) {
    setFeedback({ kind: "success", message: t("probe.task.created") });
  }

  async function createProxy(input: NetworkProxyCreate) {
    setFeedback(undefined);
    try {
      const created = await createNetworkProxy(nodeId, input, csrfToken);
      setState((current) =>
        current.kind === "success"
          ? {
              ...current,
              network: {
                ...current.network,
                networkProxies: [...current.network.networkProxies, created],
              },
            }
          : current,
      );
      return true;
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
      return false;
    }
  }

  async function updateProxy(proxyId: string, input: NetworkProxyUpdate) {
    setFeedback(undefined);
    try {
      const updated = await updateNetworkProxy(
        nodeId,
        proxyId,
        input,
        csrfToken,
      );
      setState((current) =>
        current.kind === "success"
          ? {
              ...current,
              network: {
                ...current.network,
                networkProxies: current.network.networkProxies.map((proxy) =>
                  proxy.id === updated.id ? updated : proxy,
                ),
              },
            }
          : current,
      );
      return true;
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
      return false;
    }
  }

  async function deleteProxy(proxyId: string) {
    setFeedback(undefined);
    try {
      const deleted = await deleteNetworkProxy(nodeId, proxyId, csrfToken);
      setState((current) =>
        current.kind === "success"
          ? {
              ...current,
              network: {
                ...current.network,
                networkProxies: current.network.networkProxies.map((proxy) =>
                  proxy.id === deleted.id ? deleted : proxy,
                ),
              },
            }
          : current,
      );
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    }
  }

  return (
    <div className="space-y-4" aria-live="polite">
      {feedback ? (
        <Alert variant={feedback.kind === "error" ? "destructive" : "default"}>
          {feedback.kind === "error" ? (
            <TriangleAlert aria-hidden="true" />
          ) : null}
          <AlertDescription>{feedback.message}</AlertDescription>
        </Alert>
      ) : null}

      {state.kind === "loading" ? <NetworkSkeleton /> : null}
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

      {state.kind === "success" ? (
        <>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Globe2 aria-hidden="true" className="size-4" />
                {t("network.publicAddresses.title")}
              </CardTitle>
              <CardDescription>
                {t("network.publicAddresses.detail")}
              </CardDescription>
              <CardAction className="flex items-center gap-2">
                <Badge variant="info">
                  {state.network.publicAddresses.length}
                </Badge>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={refreshing}
                  onClick={() => void load()}
                >
                  <RefreshCw
                    data-icon="inline-start"
                    aria-hidden="true"
                    className={refreshing ? "animate-spin" : undefined}
                  />
                  {t("network.refresh")}
                </Button>
              </CardAction>
            </CardHeader>
            <CardContent>
              {state.network.publicAddresses.length === 0 ? (
                <div className="py-10 text-center text-sm text-muted-foreground">
                  <Route aria-hidden="true" className="mx-auto mb-3 size-8" />
                  {t("network.publicAddresses.empty")}
                </div>
              ) : (
                <div className="overflow-hidden rounded-lg border">
                  {state.network.publicAddresses.map((address) => (
                    <PublicAddressRow
                      key={address.id}
                      nodeId={nodeId}
                      address={address}
                      locale={i18n.resolvedLanguage}
                      saving={saving.has(address.id)}
                      probeDisabled={
                        node.status !== "online" ||
                        !node.enabled ||
                        node.deletionStatus !== undefined
                      }
                      csrfToken={csrfToken}
                      onChange={(enabled) => void saveAddress(address, enabled)}
                      onProbeCreated={probeCreated}
                    />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          <NodeNetworkProxies
            proxies={state.network.networkProxies}
            onCreate={createProxy}
            onUpdate={updateProxy}
            onDelete={deleteProxy}
          />
        </>
      ) : null}
    </div>
  );
}

function PublicAddressRow({
  nodeId,
  address,
  locale,
  saving,
  probeDisabled,
  csrfToken,
  onChange,
  onProbeCreated,
}: {
  nodeId: string;
  address: PublicAddress;
  locale: string | undefined;
  saving: boolean;
  probeDisabled: boolean;
  csrfToken: string;
  onChange: (enabled: boolean) => void;
  onProbeCreated: (task: ProbeTask) => void;
}) {
  const { t } = useTranslation();
  const availability = publicAddressAvailability(address);
  return (
    <div className="grid min-w-0 gap-4 border-t p-4 first:border-t-0 lg:grid-cols-[minmax(240px,1.4fr)_minmax(130px,.65fr)_minmax(150px,.75fr)_auto] lg:items-center">
      <div className="min-w-0">
        <p className="break-all font-mono text-base font-semibold">
          {address.address}
        </p>
        <div className="mt-2 flex flex-wrap gap-1.5">
          <Badge variant="outline">
            {t(`network.family.${address.family}`)}
          </Badge>
          <Badge variant={availability === "available" ? "success" : "warning"}>
            {t(`network.publicAddresses.status.${availability}`)}
          </Badge>
          {address.likelyNat ? (
            <Badge variant="warning">{t("network.publicAddresses.nat")}</Badge>
          ) : null}
          {address.proxyPath ? (
            <Badge variant="info">{t("network.publicAddresses.proxy")}</Badge>
          ) : null}
        </div>
      </div>

      <AddressCell
        label={t("network.publicAddresses.path")}
        value={
          address.proxyPath
            ? t("network.publicAddresses.proxy")
            : t("network.publicAddresses.direct")
        }
      />
      <AddressCell
        label={t("network.publicAddresses.latestReport")}
        value={
          address.latestSnapshotAt
            ? formatTime(
                address.latestSnapshotAt,
                locale,
                t("nodes.notAvailable"),
              )
            : t("network.publicAddresses.noReport")
        }
        success={address.latestSnapshotAt !== undefined}
      />

      <div className="flex flex-wrap items-center gap-2 lg:justify-end">
        {saving ? (
          <LoaderCircle
            aria-label={t("network.publicAddresses.saving")}
            className="size-4 animate-spin text-muted-foreground"
          />
        ) : null}
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">
            {t("network.publicAddresses.probeShort")}
          </span>
          <Switch
            checked={address.probeEnabled}
            disabled={saving}
            onCheckedChange={onChange}
            aria-label={t("network.publicAddresses.probeEnabled")}
          />
        </div>
        {address.latestSnapshotId ? (
          <Button variant="outline" size="sm" asChild>
            <Link to={`/probe-snapshots/${address.latestSnapshotId}`}>
              <Eye data-icon="inline-start" aria-hidden="true" />
              {t("network.publicAddresses.openReport")}
            </Link>
          </Button>
        ) : (
          <Button variant="outline" size="sm" disabled>
            <Eye data-icon="inline-start" aria-hidden="true" />
            {t("network.publicAddresses.openReport")}
          </Button>
        )}
        <CompleteProbeDialog
          nodeId={nodeId}
          csrfToken={csrfToken}
          initialPublicAddressId={address.id}
          onCreated={onProbeCreated}
        >
          <Button size="sm" disabled={probeDisabled || !address.available}>
            <Play data-icon="inline-start" aria-hidden="true" />
            {t("network.publicAddresses.probeNow")}
          </Button>
        </CompleteProbeDialog>
      </div>
    </div>
  );
}

function AddressCell({
  label,
  value,
  success = false,
}: {
  label: string;
  value: string;
  success?: boolean;
}) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p
        className={
          success
            ? "mt-1 break-words text-sm font-medium text-emerald-700 dark:text-emerald-300"
            : "mt-1 break-words text-sm font-medium"
        }
      >
        {value}
      </p>
    </div>
  );
}

function NetworkSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      {[0, 1].map((item) => (
        <Card key={item}>
          <CardHeader>
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-72 max-w-full" />
          </CardHeader>
          <CardContent>
            <Skeleton className="h-32 w-full" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
