import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  Globe2,
  LoaderCircle,
  Network,
  Plus,
  RefreshCw,
  Server,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useParams } from "react-router";

import {
  createNodeProxyDiscoveryPath,
  deleteNodeProxyDiscoveryPath,
  getNodeNetwork,
  updatePublicAddress,
  type NodeNetworkState,
  type PublicAddress,
  type ProxyDiscoveryPath,
} from "@/api/network";
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
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { formatAPIError } from "@/lib/api-error";
import { formatTime } from "@/pages/node-probe-page";

const refreshIntervalMilliseconds = 5_000;

type ViewState =
  | { kind: "loading" }
  | {
      kind: "success";
      network: NodeNetworkState;
      proxies: NetworkProxy[];
    }
  | { kind: "error" };

export function NodeNetworkPage() {
  const { nodeId = "" } = useParams();
  const { t, i18n } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);
  const [saving, setSaving] = useState<Set<string>>(() => new Set());
  const [deleting, setDeleting] = useState<Set<string>>(() => new Set());
  const [creatingPath, setCreatingPath] = useState(false);
  const [feedback, setFeedback] = useState<string>();

  const load = useCallback(
    async (signal?: AbortSignal, initial = false, quiet = false) => {
      if (initial) setState({ kind: "loading" });
      else if (!quiet) setRefreshing(true);
      try {
        const [network, proxies] = await Promise.all([
          getNodeNetwork(nodeId, signal),
          listNetworkProxies(signal),
        ]);
        setState({ kind: "success", network, proxies });
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

  async function saveAddress(
    address: PublicAddress,
    update: Pick<PublicAddress, "probeEnabled" | "probeOnRediscovery">,
  ) {
    setSaving((current) => new Set(current).add(address.id));
    setFeedback(undefined);
    try {
      const updated = await updatePublicAddress(
        nodeId,
        address.id,
        update,
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
              proxies: current.proxies,
            }
          : current,
      );
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setSaving((current) => {
        const next = new Set(current);
        next.delete(address.id);
        return next;
      });
    }
  }

  async function createProxyPath(proxyId: string, family: "ipv4" | "ipv6") {
    setCreatingPath(true);
    setFeedback(undefined);
    try {
      const created = await createNodeProxyDiscoveryPath(
        nodeId,
        { proxyId, family },
        csrfToken,
      );
      setState((current) =>
        current.kind === "success"
          ? {
              ...current,
              network: {
                ...current.network,
                proxyDiscoveryPaths: [
                  ...current.network.proxyDiscoveryPaths,
                  created,
                ],
              },
            }
          : current,
      );
      return true;
    } catch (error) {
      setFeedback(formatAPIError(error, t));
      return false;
    } finally {
      setCreatingPath(false);
    }
  }

  async function deleteProxyPath(path: ProxyDiscoveryPath) {
    setDeleting((current) => new Set(current).add(path.id));
    setFeedback(undefined);
    try {
      const deletion = await deleteNodeProxyDiscoveryPath(
        nodeId,
        path.id,
        csrfToken,
      );
      setState((current) =>
        current.kind === "success"
          ? {
              ...current,
              network: {
                ...current.network,
                proxyDiscoveryPaths: current.network.proxyDiscoveryPaths.map(
                  (item) =>
                    item.id === path.id
                      ? {
                          ...item,
                          deletionStatus: deletion.status,
                          deletionError: deletion.error,
                        }
                      : item,
                ),
              },
            }
          : current,
      );
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    } finally {
      setDeleting((current) => {
        const next = new Set(current);
        next.delete(path.id);
        return next;
      });
    }
  }

  return (
    <div className="space-y-4" aria-live="polite">
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
            {state.kind === "success" ? (
              <Badge variant="secondary">
                {state.network.publicAddresses.length}
              </Badge>
            ) : null}
            <Button
              variant="outline"
              size="sm"
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
          </CardAction>
        </CardHeader>
      </Card>

      {feedback ? (
        <Alert variant="destructive">
          <TriangleAlert aria-hidden="true" />
          <AlertDescription>{feedback}</AlertDescription>
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
          {state.network.publicAddresses.length === 0 ? (
            <Card>
              <CardContent className="py-10 text-center">
                <Network
                  aria-hidden="true"
                  className="mx-auto size-8 text-muted-foreground"
                />
                <p className="mt-3 text-sm text-muted-foreground">
                  {t("network.publicAddresses.empty")}
                </p>
              </CardContent>
            </Card>
          ) : (
            <div className="grid gap-4 xl:grid-cols-2">
              {state.network.publicAddresses.map((address) => (
                <PublicAddressCard
                  key={address.id}
                  address={address}
                  saving={saving.has(address.id)}
                  locale={i18n.resolvedLanguage}
                  onChange={(update) => void saveAddress(address, update)}
                />
              ))}
            </div>
          )}
          <ProxyDiscoveryPathsCard
            paths={state.network.proxyDiscoveryPaths}
            proxies={state.proxies}
            creating={creatingPath}
            deleting={deleting}
            onCreate={createProxyPath}
            onDelete={(path) => void deleteProxyPath(path)}
          />
        </>
      ) : null}
    </div>
  );
}

function PublicAddressCard({
  address,
  saving,
  locale,
  onChange,
}: {
  address: PublicAddress;
  saving: boolean;
  locale: string | undefined;
  onChange: (
    update: Pick<PublicAddress, "probeEnabled" | "probeOnRediscovery">,
  ) => void;
}) {
  const { t } = useTranslation();
  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle className="min-w-0 break-all font-mono text-lg">
          {address.address}
        </CardTitle>
        <CardDescription>
          {t("network.publicAddresses.lastSeen", {
            value: formatTime(
              address.lastSeenAt,
              locale,
              t("nodes.notAvailable"),
            ),
          })}
        </CardDescription>
        <CardAction className="flex items-center gap-2">
          {saving ? (
            <LoaderCircle
              aria-label={t("network.publicAddresses.saving")}
              className="size-4 animate-spin text-muted-foreground"
            />
          ) : null}
          <Badge variant={address.available ? "default" : "outline"}>
            {address.available
              ? t("network.publicAddresses.available")
              : t("network.publicAddresses.unavailable")}
          </Badge>
          <Badge variant="secondary">{address.family.toUpperCase()}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-5">
        <dl className="grid gap-3 text-sm sm:grid-cols-2">
          <AddressValue
            label={t("network.publicAddresses.firstSeen")}
            value={formatTime(
              address.firstSeenAt,
              locale,
              t("nodes.notAvailable"),
            )}
          />
          <AddressValue
            label={t("network.publicAddresses.executionNode")}
            value={
              address.selectedNodeName ?? t("network.publicAddresses.noNode")
            }
          />
        </dl>

        {address.likelyNat || address.proxyPath ? (
          <div className="flex flex-wrap gap-2">
            {address.likelyNat ? (
              <Badge variant="secondary">
                {t("network.publicAddresses.nat")}
              </Badge>
            ) : null}
            {address.proxyPath ? (
              <Badge variant="secondary">
                {t("network.publicAddresses.proxy")}
              </Badge>
            ) : null}
          </div>
        ) : null}

        <div className="space-y-4 border-t pt-4">
          <SettingSwitch
            id={`probe-${address.id}`}
            label={t("network.publicAddresses.probeEnabled")}
            detail={t("network.publicAddresses.probeEnabledDetail")}
            checked={address.probeEnabled}
            disabled={saving}
            onCheckedChange={(probeEnabled) =>
              onChange({
                probeEnabled,
                probeOnRediscovery: address.probeOnRediscovery,
              })
            }
          />
          <SettingSwitch
            id={`rediscovery-${address.id}`}
            label={t("network.publicAddresses.probeOnRediscovery")}
            detail={t("network.publicAddresses.probeOnRediscoveryDetail")}
            checked={address.probeOnRediscovery}
            disabled={saving || !address.probeEnabled}
            onCheckedChange={(probeOnRediscovery) =>
              onChange({
                probeEnabled: address.probeEnabled,
                probeOnRediscovery,
              })
            }
          />
        </div>
      </CardContent>
    </Card>
  );
}

function ProxyDiscoveryPathsCard({
  paths,
  proxies,
  creating,
  deleting,
  onCreate,
  onDelete,
}: {
  paths: ProxyDiscoveryPath[];
  proxies: NetworkProxy[];
  creating: boolean;
  deleting: Set<string>;
  onCreate: (proxyId: string, family: "ipv4" | "ipv6") => Promise<boolean>;
  onDelete: (path: ProxyDiscoveryPath) => void;
}) {
  const { t } = useTranslation();
  const [proxyId, setProxyId] = useState("");
  const [family, setFamily] = useState<"ipv4" | "ipv6">("ipv4");

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!proxyId) return;
    if (await onCreate(proxyId, family)) setProxyId("");
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("network.proxyDiscovery.title")}</CardTitle>
        <CardDescription>{t("network.proxyDiscovery.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">{paths.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-5">
        {proxies.length === 0 ? (
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border p-4">
            <p className="text-sm text-muted-foreground">
              {t("network.proxyDiscovery.noProxies")}
            </p>
            <Button variant="outline" size="sm" asChild>
              <Link to="/settings/network">
                {t("network.proxyDiscovery.openSettings")}
              </Link>
            </Button>
          </div>
        ) : (
          <form
            className="grid items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(9rem,0.4fr)_auto]"
            onSubmit={(event) => void submit(event)}
          >
            <div className="space-y-2">
              <Label htmlFor="proxy-discovery-proxy">
                {t("network.proxyDiscovery.proxy")}
              </Label>
              <Select value={proxyId} onValueChange={setProxyId}>
                <SelectTrigger id="proxy-discovery-proxy" className="w-full">
                  <SelectValue
                    placeholder={t("network.proxyDiscovery.selectProxy")}
                  />
                </SelectTrigger>
                <SelectContent>
                  {proxies.map((proxy) => (
                    <SelectItem key={proxy.id} value={proxy.id}>
                      {proxy.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="proxy-discovery-family">
                {t("network.proxyDiscovery.family")}
              </Label>
              <Select
                value={family}
                onValueChange={(value) => setFamily(value as "ipv4" | "ipv6")}
              >
                <SelectTrigger id="proxy-discovery-family" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="ipv4">IPv4</SelectItem>
                  <SelectItem value="ipv6">IPv6</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button type="submit" disabled={!proxyId || creating}>
              {creating ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <Plus data-icon="inline-start" aria-hidden="true" />
              )}
              {t("network.proxyDiscovery.add")}
            </Button>
          </form>
        )}

        {paths.length === 0 ? (
          <p className="py-5 text-center text-sm text-muted-foreground">
            {t("network.proxyDiscovery.empty")}
          </p>
        ) : (
          <div className="divide-y rounded-md border">
            {paths.map((path) => {
              const proxyName =
                proxies.find((proxy) => proxy.id === path.proxyId)?.name ??
                path.name;
              const displayName = `${proxyName} · ${path.family.toUpperCase()}`;
              return (
                <div
                  key={path.id}
                  className="flex flex-wrap items-center justify-between gap-3 p-3"
                >
                  <div className="min-w-0">
                    <p className="break-words text-sm font-medium">
                      {displayName}
                    </p>
                    <div className="mt-1 flex flex-wrap gap-2">
                      <Badge variant="secondary">
                        {path.family.toUpperCase()}
                      </Badge>
                      <Badge variant={path.available ? "default" : "outline"}>
                        {path.available
                          ? t("network.proxyDiscovery.available")
                          : t("network.proxyDiscovery.unavailable")}
                      </Badge>
                      {path.deletionStatus ? (
                        <Badge
                          variant={
                            path.deletionStatus === "failed"
                              ? "destructive"
                              : "outline"
                          }
                        >
                          {t(
                            `network.proxyDiscovery.deletion.${path.deletionStatus}`,
                          )}
                        </Badge>
                      ) : null}
                    </div>
                    {path.deletionError ? (
                      <p className="mt-2 text-xs text-destructive">
                        {path.deletionError}
                      </p>
                    ) : null}
                  </div>
                  <DeleteProxyPathButton
                    displayName={displayName}
                    disabled={
                      deleting.has(path.id) || path.deletionStatus === "pending"
                    }
                    onDelete={() => onDelete(path)}
                  />
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function DeleteProxyPathButton({
  displayName,
  disabled,
  onDelete,
}: {
  displayName: string;
  disabled: boolean;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" size="sm" disabled={disabled}>
          <Trash2 data-icon="inline-start" aria-hidden="true" />
          {t("network.proxyDiscovery.delete.action")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <Trash2 aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle>
            {t("network.proxyDiscovery.delete.title")}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("network.proxyDiscovery.delete.detail", { name: displayName })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onDelete}>
            {t("network.proxyDiscovery.delete.confirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function SettingSwitch({
  id,
  label,
  detail,
  checked,
  disabled,
  onCheckedChange,
}: {
  id: string;
  label: string;
  detail: string;
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="min-w-0 space-y-1">
        <Label htmlFor={id}>{label}</Label>
        <p className="text-xs text-muted-foreground">{detail}</p>
      </div>
      <Switch
        id={id}
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

function AddressValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-words font-medium">
        <Server aria-hidden="true" className="mr-1 inline size-3.5" />
        {value}
      </dd>
    </div>
  );
}

function NetworkSkeleton() {
  return (
    <div className="grid gap-4 xl:grid-cols-2" aria-busy="true">
      {[0, 1].map((item) => (
        <Card key={item}>
          <CardHeader>
            <Skeleton className="h-6 w-44" />
          </CardHeader>
          <CardContent className="space-y-4">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-16 w-full" />
          </CardContent>
        </Card>
      ))}
      <Card className="xl:col-span-2">
        <CardHeader>
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-4 w-72 max-w-full" />
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            <Skeleton className="h-9 w-full" />
            <Skeleton className="h-14 w-full" />
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
