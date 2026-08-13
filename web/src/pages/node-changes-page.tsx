import { useCallback, useEffect, useState } from "react";
import { RefreshCw, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router";

import { getNodeNetwork, type NodeNetworkState } from "@/api/network";
import { NodeAddressHistory } from "@/components/node-address-history";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

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
    async (signal?: AbortSignal, initial = false) => {
      if (initial) setState({ kind: "loading" });
      else setRefreshing(true);
      try {
        setState({
          kind: "success",
          network: await getNodeNetwork(nodeId, signal),
        });
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
      {state.kind === "loading" ? (
        <Card aria-busy="true">
          <CardHeader>
            <Skeleton className="h-5 w-40" />
          </CardHeader>
          <CardContent>
            <Skeleton className="h-40 w-full" />
          </CardContent>
        </Card>
      ) : null}
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
        <NodeAddressHistory network={state.network} />
      ) : null}
    </div>
  );
}
