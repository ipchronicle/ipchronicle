import { useEffect, useState } from "react";
import { LoaderCircle, ScanSearch, ScrollText } from "lucide-react";
import { useTranslation } from "react-i18next";
import { getNodeNetwork, type PublicAddress } from "@/api/network";
import { updateNode, type Node } from "@/api/nodes";
import { createCompleteProbeTask } from "@/api/probes";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatAPIError } from "@/lib/api-error";

type Result = {
  id: string;
  name: string;
  status: "accepted" | "skipped" | "failed";
  detail?: string;
};
type ProbeTarget = {
  node: Node;
  addresses: PublicAddress[];
  selected: Set<string>;
  result?: Result;
};
type Props = {
  nodes: Node[];
  csrfToken: string;
  onNodeChange: (node: Node) => void;
};

export function NodeBatchActions(props: Props) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<"probe" | "logs">();
  const [targets, setTargets] = useState<Node[]>([]);
  function open(next: "probe" | "logs") {
    setTargets(props.nodes);
    setMode(next);
  }
  return (
    <>
      <Button variant="outline" onClick={() => open("probe")}>
        <ScanSearch aria-hidden="true" />
        {t("nodes.batch.probe")}
      </Button>
      <Button variant="outline" onClick={() => open("logs")}>
        <ScrollText aria-hidden="true" />
        {t("nodes.batch.logs")}
      </Button>
      {mode ? (
        <BatchDialog
          {...props}
          nodes={targets}
          mode={mode}
          onClose={() => setMode(undefined)}
        />
      ) : null}
    </>
  );
}

function BatchDialog({
  nodes,
  csrfToken,
  onNodeChange,
  mode,
  onClose,
}: Props & { mode: "probe" | "logs"; onClose: () => void }) {
  const { t } = useTranslation();
  const [targets, setTargets] = useState<ProbeTarget[]>([]);
  const [loading, setLoading] = useState(mode === "probe");
  const [working, setWorking] = useState(false);
  const [results, setResults] = useState<Result[]>();
  const [level, setLevel] = useState<Node["logLevel"]>("info");
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (mode !== "probe") return;
    const controller = new AbortController();
    async function prepare() {
      setLoading(true);
      const prepared: ProbeTarget[] = [];
      for (const node of nodes) {
        if (controller.signal.aborted) return;
        const target: ProbeTarget = {
          node,
          addresses: [],
          selected: new Set(),
        };
        const skip =
          !node.enabled ||
          node.deletionStatus !== undefined ||
          node.status === "revoked"
            ? "disabled"
            : node.status !== "online"
              ? "offline"
              : undefined;
        if (skip)
          target.result = {
            id: node.id,
            name: node.name,
            status: "skipped",
            detail: t(`nodes.batch.${skip}`),
          };
        else {
          try {
            const network = await getNodeNetwork(node.id, controller.signal);
            target.addresses = network.publicAddresses.filter(
              (address) =>
                address.available && address.selectedNodeId === node.id,
            );
            target.selected = new Set(
              target.addresses.map((address) => address.id),
            );
            if (!target.addresses.length)
              target.result = {
                id: node.id,
                name: node.name,
                status: "skipped",
                detail: t("nodes.batch.noAddresses"),
              };
          } catch (error) {
            if (controller.signal.aborted) return;
            target.result = {
              id: node.id,
              name: node.name,
              status: "failed",
              detail: formatAPIError(error, t),
            };
          }
        }
        prepared.push(target);
      }
      if (!controller.signal.aborted) {
        setTargets(prepared);
        setLoading(false);
      }
    }
    void prepare();
    return () => controller.abort();
  }, [nodes, mode, attempt, t]);

  async function submit() {
    setWorking(true);
    setResults([]);
    const completed: Result[] = [];
    for (const node of nodes) {
      let result: Result = { id: node.id, name: node.name, status: "accepted" };
      try {
        if (mode === "logs")
          onNodeChange(
            await updateNode(node.id, { logLevel: level }, csrfToken),
          );
        else {
          const target = targets.find((item) => item.node.id === node.id)!;
          if (target.result) result = target.result;
          else if (!target.selected.size)
            result = {
              ...result,
              status: "skipped",
              detail: t("nodes.batch.noSelection"),
            };
          else
            await createCompleteProbeTask(
              node.id,
              { publicAddressIds: [...target.selected] },
              csrfToken,
            );
        }
      } catch (error) {
        const code =
          typeof error === "object" && error !== null && "code" in error
            ? error.code
            : undefined;
        const skipped =
          mode === "probe" &&
          [
            "node_offline",
            "node_disabled",
            "node_revoked",
            "node_not_found",
            "probe_paused_low_memory",
            "probe_task_slot_occupied",
            "probe_already_running",
            "invalid_probe_targets",
          ].includes(String(code));
        result = {
          ...result,
          status: skipped ? "skipped" : "failed",
          detail: formatAPIError(error, t),
        };
      }
      completed.push(result);
      setResults([...completed]);
    }
    setWorking(false);
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !working) onClose();
      }}
    >
      <DialogContent closeLabel={t("common.close")} className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t(`nodes.batch.${mode}`)} · {nodes.length}
          </DialogTitle>
          <DialogDescription>
            {t(
              mode === "probe"
                ? "probe.targets.detail"
                : "nodes.batch.logsDetail",
            )}
          </DialogDescription>
        </DialogHeader>
        {loading ? (
          <p role="status" className="flex items-center gap-2">
            <LoaderCircle className="size-4 animate-spin" />
            {t("probe.targets.loading")}
          </p>
        ) : results ? (
          <BatchResults results={results} />
        ) : (
          <>
            {mode === "logs" ? (
              <>
                <Select
                  value={level}
                  onValueChange={(value) => setLevel(value as Node["logLevel"])}
                >
                  <SelectTrigger aria-label={t("nodes.batch.logs")}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(["error", "warn", "info", "debug"] as const).map(
                      (value) => (
                        <SelectItem key={value} value={value}>
                          {value}
                        </SelectItem>
                      ),
                    )}
                  </SelectContent>
                </Select>
                <ul className="max-h-72 overflow-auto">
                  {nodes.map((node) => (
                    <li key={node.id} className="break-words py-1">
                      {node.name}
                    </li>
                  ))}
                </ul>
              </>
            ) : (
              <div className="max-h-[55dvh] divide-y overflow-auto">
                {targets.map((target) => (
                  <details
                    key={target.node.id}
                    className="py-3"
                    open={targets.length === 1}
                  >
                    <summary className="cursor-pointer break-words font-medium">
                      {target.node.name} · {target.selected.size}/
                      {target.addresses.length}
                      {target.result ? (
                        <span className="ml-2 text-sm text-muted-foreground">
                          {target.result.detail}
                        </span>
                      ) : null}
                    </summary>
                    <div className="space-y-2 pt-3">
                      {target.addresses.map((address) => (
                        <label
                          key={address.id}
                          className="flex items-center gap-3 rounded-md border p-3"
                        >
                          <Checkbox
                            checked={target.selected.has(address.id)}
                            onCheckedChange={(checked) =>
                              setTargets((current) =>
                                current.map((item) => {
                                  if (item.node.id !== target.node.id)
                                    return item;
                                  const selected = new Set(item.selected);
                                  if (checked) selected.add(address.id);
                                  else selected.delete(address.id);
                                  return { ...item, selected };
                                }),
                              )
                            }
                          />
                          <span className="break-all font-mono">
                            {address.address}
                          </span>
                        </label>
                      ))}
                    </div>
                  </details>
                ))}
              </div>
            )}
          </>
        )}
        <DialogFooter>
          {mode === "probe" &&
          !results &&
          !loading &&
          targets.some((target) => target.result?.status === "failed") ? (
            <Button
              variant="outline"
              onClick={() => setAttempt((value) => value + 1)}
            >
              {t("snapshot.retry")}
            </Button>
          ) : null}
          <Button variant="outline" disabled={working} onClick={onClose}>
            {t(results ? "common.close" : "common.cancel")}
          </Button>
          {!results ? (
            <Button disabled={loading || working} onClick={() => void submit()}>
              {t(
                mode === "probe" ? "probe.targets.confirm" : "nodes.batch.save",
              )}
            </Button>
          ) : null}
          {working ? (
            <LoaderCircle
              aria-label={t("nodes.batch.working")}
              className="size-4 animate-spin"
            />
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function BatchResults({ results }: { results: Result[] }) {
  const { t } = useTranslation();
  return (
    <ul aria-live="polite" className="max-h-[55dvh] divide-y overflow-auto">
      {results.map((result) => (
        <li key={result.id} className="space-y-2 py-3">
          <div className="flex flex-wrap items-center gap-2">
            <span className="break-words font-medium">{result.name}</span>
            <Badge
              variant={result.status === "failed" ? "destructive" : "outline"}
            >
              {t(`nodes.batch.${result.status}`)}
            </Badge>
          </div>
          {result.detail ? (
            <p className="text-sm text-muted-foreground">{result.detail}</p>
          ) : null}
        </li>
      ))}
    </ul>
  );
}
