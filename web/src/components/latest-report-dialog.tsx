import { useEffect, useMemo, useState } from "react";
import {
  ArrowUpRight,
  FileText,
  LoaderCircle,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { getNodeNetwork } from "@/api/network";
import { getProbeSnapshot, type ProbeSnapshot } from "@/api/probes";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { NavigationSourceLink } from "@/lib/navigation-context";
import { formatTime } from "@/pages/node-probe-page";
import { SemanticProbeReport } from "@/pages/probe-snapshot-page";

type Target = {
  nodeId: string;
  nodeName: string;
  addressId: string;
  address: string;
};
type ViewState =
  | { kind: "loading" }
  | { kind: "empty" }
  | { kind: "error" }
  | { kind: "success"; snapshot: ProbeSnapshot };

export function LatestReportDialog(props: Target) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="outline"
          size="sm"
          aria-label={t("nodes.reportPreview.actionFor", {
            address: props.address,
          })}
        >
          <FileText data-icon="inline-start" aria-hidden="true" />
          {t("nodes.reportPreview.action")}
        </Button>
      </DialogTrigger>
      {open ? (
        <DialogContent
          closeLabel={t("common.close")}
          className="sm:max-w-6xl"
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
        >
          <DialogHeader className="min-w-0 pr-8">
            <DialogTitle className="break-all">
              {t("nodes.reportPreview.title", { address: props.address })}
            </DialogTitle>
            <DialogDescription className="break-words">
              {props.nodeName}
            </DialogDescription>
          </DialogHeader>
          <LatestReportContent
            nodeId={props.nodeId}
            addressId={props.addressId}
          />
        </DialogContent>
      ) : null}
    </Dialog>
  );
}

function LatestReportContent({
  nodeId,
  addressId,
}: Pick<Target, "nodeId" | "addressId">) {
  const { t, i18n } = useTranslation();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      setState({ kind: "loading" });
      try {
        const network = await getNodeNetwork(nodeId, controller.signal);
        const snapshotId = network.publicAddresses.find(
          (item) => item.id === addressId,
        )?.latestSnapshotId;
        if (!snapshotId) {
          if (!controller.signal.aborted) setState({ kind: "empty" });
          return;
        }
        const snapshot = await getProbeSnapshot(snapshotId, controller.signal);
        if (!controller.signal.aborted) setState({ kind: "success", snapshot });
      } catch {
        if (!controller.signal.aborted) setState({ kind: "error" });
      }
    }
    void load();
    return () => controller.abort();
  }, [nodeId, addressId, attempt]);
  const fields = useMemo(
    () =>
      new Map(
        state.kind === "success"
          ? state.snapshot.fields.map((field) => [field.path, field])
          : [],
      ),
    [state],
  );

  if (state.kind === "loading")
    return (
      <div
        role="status"
        className="flex items-center gap-2 py-8 text-muted-foreground"
      >
        <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
        {t("nodes.reportPreview.loading")}
      </div>
    );
  if (state.kind === "empty")
    return (
      <p role="status" className="py-8 text-muted-foreground">
        {t("nodes.reportPreview.empty")}
      </p>
    );
  if (state.kind === "error")
    return (
      <Alert variant="destructive">
        <TriangleAlert aria-hidden="true" />
        <AlertDescription>
          {t("snapshot.loadFailed")}
          <Button
            variant="outline"
            size="sm"
            onClick={() => setAttempt((value) => value + 1)}
          >
            {t("snapshot.retry")}
          </Button>
        </AlertDescription>
      </Alert>
    );
  return (
    <div className="min-w-0 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-muted-foreground">
          {formatTime(
            state.snapshot.observedAt,
            i18n.resolvedLanguage,
            t("probe.notAvailable"),
          )}
        </p>
        <Button variant="outline" size="sm" asChild>
          <NavigationSourceLink to={`/probe-snapshots/${state.snapshot.id}`}>
            <ArrowUpRight data-icon="inline-start" aria-hidden="true" />
            {t("nodes.reportPreview.open")}
          </NavigationSourceLink>
        </Button>
      </div>
      <SemanticProbeReport fields={fields} />
    </div>
  );
}
