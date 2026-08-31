import { useEffect, useState, type ReactNode } from "react";
import {
  LoaderCircle,
  RefreshCw,
  ScanSearch,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { getNodeNetwork, type PublicAddress } from "@/api/network";
import { createCompleteProbeTask, type ProbeTask } from "@/api/probes";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { formatAPIError } from "@/lib/api-error";

type AddressState =
  | { kind: "loading" }
  | { kind: "success"; addresses: PublicAddress[] }
  | { kind: "error" };

export function CompleteProbeDialog({
  nodeId,
  csrfToken,
  initialPublicAddressId,
  children,
  onCreated,
}: {
  nodeId: string;
  csrfToken: string;
  initialPublicAddressId?: string;
  children: ReactNode;
  onCreated: (task: ProbeTask) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [state, setState] = useState<AddressState>({ kind: "loading" });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string>();
  const [loadRevision, setLoadRevision] = useState(0);

  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    setState({ kind: "loading" });
    setError(undefined);
    void getNodeNetwork(nodeId, controller.signal)
      .then((network) => {
        const addresses = network.publicAddresses.filter(
          (address) => address.available && address.selectedNodeId === nodeId,
        );
        const initialAddress = addresses.find(
          (address) => address.id === initialPublicAddressId,
        );
        setSelected(
          new Set(
            initialAddress
              ? [initialAddress.id]
              : addresses
                  .filter((address) => address.probeEnabled)
                  .map((address) => address.id),
          ),
        );
        setState({ kind: "success", addresses });
      })
      .catch((cause: unknown) => {
        if (cause instanceof DOMException && cause.name === "AbortError")
          return;
        setState({ kind: "error" });
      });
    return () => controller.abort();
  }, [initialPublicAddressId, loadRevision, nodeId, open]);

  function changeOpen(next: boolean) {
    if (submitting) return;
    setOpen(next);
  }

  function toggleAddress(id: string, checked: boolean) {
    setSelected((current) => {
      const next = new Set(current);
      if (checked) next.add(id);
      else next.delete(id);
      return next;
    });
    setError(undefined);
  }

  async function submit() {
    if (selected.size === 0) return;
    setSubmitting(true);
    setError(undefined);
    try {
      const task = await createCompleteProbeTask(
        nodeId,
        { publicAddressIds: Array.from(selected) },
        csrfToken,
      );
      setOpen(false);
      onCreated(task);
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setSubmitting(false);
    }
  }

  const addresses = state.kind === "success" ? state.addresses : [];

  return (
    <AlertDialog open={open} onOpenChange={changeOpen}>
      <AlertDialogTrigger asChild>{children}</AlertDialogTrigger>
      <AlertDialogContent className="sm:max-w-lg">
        <AlertDialogHeader>
          <AlertDialogMedia>
            <ScanSearch aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle>{t("probe.targets.title")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("probe.targets.detail")}
          </AlertDialogDescription>
        </AlertDialogHeader>

        {state.kind === "loading" ? (
          <div className="space-y-2" aria-label={t("probe.targets.loading")}>
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        ) : null}

        {state.kind === "error" ? (
          <div className="space-y-3 text-center">
            <p className="text-sm text-muted-foreground">
              {t("probe.targets.loadFailed")}
            </p>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setLoadRevision((value) => value + 1)}
            >
              <RefreshCw data-icon="inline-start" aria-hidden="true" />
              {t("probe.targets.retry")}
            </Button>
          </div>
        ) : null}

        {state.kind === "success" && addresses.length === 0 ? (
          <p className="py-5 text-center text-sm text-muted-foreground">
            {t("probe.targets.empty")}
          </p>
        ) : null}

        {addresses.length > 0 ? (
          <div className="max-h-72 space-y-2 overflow-y-auto p-0.5">
            {addresses.map((address) => {
              const checkboxId = `complete-probe-target-${address.id}`;
              return (
                <label
                  key={address.id}
                  htmlFor={checkboxId}
                  className="flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors hover:bg-muted/50"
                >
                  <Checkbox
                    id={checkboxId}
                    className="mt-1"
                    checked={selected.has(address.id)}
                    disabled={submitting}
                    onCheckedChange={(checked) =>
                      toggleAddress(address.id, checked === true)
                    }
                  />
                  <span className="min-w-0 flex-1">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="break-all font-mono text-base font-medium">
                        {address.address}
                      </span>
                      <Badge variant="secondary">
                        {address.family.toUpperCase()}
                      </Badge>
                    </span>
                    {address.likelyNat || address.proxyPath ? (
                      <span className="mt-2 flex flex-wrap gap-2">
                        {address.likelyNat ? (
                          <Badge variant="outline">
                            {t("network.publicAddresses.nat")}
                          </Badge>
                        ) : null}
                        {address.proxyPath ? (
                          <Badge variant="outline">
                            {t("network.publicAddresses.proxy")}
                          </Badge>
                        ) : null}
                      </span>
                    ) : null}
                  </span>
                </label>
              );
            })}
          </div>
        ) : null}

        {state.kind === "success" && addresses.length > 0 ? (
          <p className="text-sm text-muted-foreground">
            {t("probe.targets.selected", {
              selected: selected.size,
              total: addresses.length,
            })}
          </p>
        ) : null}

        {error ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        <AlertDialogFooter>
          <AlertDialogCancel disabled={submitting}>
            {t("common.cancel")}
          </AlertDialogCancel>
          <Button
            disabled={
              submitting || state.kind !== "success" || selected.size === 0
            }
            onClick={() => void submit()}
          >
            {submitting ? (
              <LoaderCircle
                data-icon="inline-start"
                aria-hidden="true"
                className="animate-spin"
              />
            ) : (
              <ScanSearch data-icon="inline-start" aria-hidden="true" />
            )}
            {t("probe.targets.confirm")}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
