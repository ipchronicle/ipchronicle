import { useCallback, useEffect, useState } from "react";
import { Globe2, RefreshCw, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router";

import { getNodeNetwork, type NodeNetworkState } from "@/api/network";
import { NodeAddressHistory } from "@/components/node-address-history";
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

const refreshIntervalMilliseconds = 5_000;

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; network: NodeNetworkState }
  | { kind: "error" };

export function NodeChangesPage() {
  const { nodeId = "" } = useParams();
  const { t } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);

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
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        void load(undefined, false, true);
      }
    }, refreshIntervalMilliseconds);
    return () => window.clearInterval(timer);
  }, [load, state.kind]);

  return (
    <div className="space-y-4" aria-live="polite">
      <div className="flex justify-end">
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
          {t("nodeDetail.changes.refresh")}
        </Button>
      </div>
      {state.kind === "loading" ? <ChangesSkeleton /> : null}
      {state.kind === "error" ? (
        <Alert variant="destructive">
          <TriangleAlert aria-hidden="true" />
          <AlertTitle>{t("nodeDetail.changes.loadFailed")}</AlertTitle>
          <AlertDescription>
            <Button
              variant="outline"
              size="sm"
              className="mt-3"
              onClick={() => void load(undefined, true)}
            >
              <RefreshCw data-icon="inline-start" aria-hidden="true" />
              {t("nodeDetail.retry")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {state.kind === "success" ? (
        <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,2.6fr)_minmax(290px,.75fr)]">
          <NodeAddressHistory network={state.network} />
          <CurrentAddressSet network={state.network} />
        </div>
      ) : null}
    </div>
  );
}

function CurrentAddressSet({ network }: { network: NodeNetworkState }) {
  const { t } = useTranslation();
  const current = network.publicAddresses.filter(
    (address) => address.available,
  );
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Globe2 aria-hidden="true" className="size-4" />
          {t("network.addressHistory.current.title")}
        </CardTitle>
        <CardDescription>
          {t("network.addressHistory.current.detail")}
        </CardDescription>
        <CardAction>
          <Badge variant="info">{current.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        {current.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("network.addressHistory.current.empty")}
          </p>
        ) : (
          <div className="space-y-2">
            {current.map((address) => (
              <div
                key={address.id}
                className="flex min-w-0 items-center justify-between gap-3 rounded-md bg-muted/60 p-3"
              >
                <span className="min-w-0 break-all font-mono text-sm font-medium">
                  {address.address}
                </span>
                <div className="flex shrink-0 flex-wrap gap-1">
                  <Badge variant="outline">
                    {t(`network.family.${address.family}`)}
                  </Badge>
                  {address.proxyPath ? (
                    <Badge variant="info">
                      {t("network.publicAddresses.proxy")}
                    </Badge>
                  ) : null}
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function ChangesSkeleton() {
  return (
    <div
      className="grid items-start gap-4 xl:grid-cols-[minmax(0,2.6fr)_minmax(290px,.75fr)]"
      aria-busy="true"
    >
      {[0, 1].map((item) => (
        <Card key={item}>
          <CardHeader>
            <Skeleton className="h-5 w-40" />
            <Skeleton className="h-4 w-64 max-w-full" />
          </CardHeader>
          <CardContent>
            <Skeleton className="h-48 w-full" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
