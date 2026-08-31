import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  CircleArrowUp,
  Clipboard,
  KeyRound,
  LoaderCircle,
  PackageX,
  Radar,
  Save,
  ServerCog,
  ShieldX,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router";

import { deleteNode, revokeNode, updateNode } from "@/api/nodes";
import {
  getNodeProbe,
  updateNodeProbeSettings,
  type NodeProbeState,
} from "@/api/probes";
import {
  createAgentUpdateTasks,
  getAgentUpdateState,
  type AgentUpdateState,
} from "@/api/updates";
import { useAuth } from "@/auth-context";
import { useNodeDetail } from "@/components/node-detail-layout";
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
import { Alert, AlertDescription } from "@/components/ui/alert";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatAPIError } from "@/lib/api-error";
import {
  canRequestAgentUpdate,
  isTerminalUpdateTask,
  nodeHasAvailableUpdate,
} from "@/lib/agent-update";
import { agentUninstallCommand } from "@/lib/agent-installer";
import { formatTime } from "@/pages/node-probe-page";

type WorkingAction = "basic" | "probe" | "update" | "revoke" | "delete";
type UninstallMode = "preserve" | "purge";

const preservingUninstallCommand = agentUninstallCommand("preserve");
const purgingUninstallCommand = agentUninstallCommand("purge");

export function NodeSettingsPage() {
  const { nodeId = "" } = useParams();
  const { node, replaceNode } = useNodeDetail();
  const { state: authState } = useAuth();
  const { t, i18n } = useTranslation();
  const [updates, setUpdates] = useState<AgentUpdateState>();
  const [updateLoadFailed, setUpdateLoadFailed] = useState(false);
  const [probe, setProbe] = useState<NodeProbeState>();
  const [probeLoadFailed, setProbeLoadFailed] = useState(false);
  const [name, setName] = useState(node.name);
  const [enabled, setEnabled] = useState(node.enabled);
  const [basicDirty, setBasicDirty] = useState(false);
  const [probeOnNewAddress, setProbeOnNewAddress] = useState(true);
  const [lowMemoryOverride, setLowMemoryOverride] = useState(false);
  const [working, setWorking] = useState<WorkingAction>();
  const [copiedUninstall, setCopiedUninstall] = useState<UninstallMode>();
  const copyFeedbackTimeoutRef = useRef<number | undefined>(undefined);
  const [feedback, setFeedback] = useState<
    { kind: "success" | "error"; message: string } | undefined
  >();

  useEffect(() => {
    if (basicDirty) return;
    setName(node.name);
    setEnabled(node.enabled);
  }, [basicDirty, node.enabled, node.name]);

  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      getAgentUpdateState(controller.signal)
        .then((value) => setUpdates(value))
        .catch((error: unknown) => {
          if (!(error instanceof DOMException && error.name === "AbortError")) {
            setUpdateLoadFailed(true);
          }
        }),
      getNodeProbe(nodeId, controller.signal)
        .then((value) => {
          setProbe(value);
          setProbeOnNewAddress(value.probeOnNewAddress);
          setLowMemoryOverride(value.lowMemoryOverride);
        })
        .catch((error: unknown) => {
          if (!(error instanceof DOMException && error.name === "AbortError")) {
            setProbeLoadFailed(true);
          }
        }),
    ]);
    return () => controller.abort();
  }, [nodeId]);

  useEffect(
    () => () => {
      if (copyFeedbackTimeoutRef.current !== undefined) {
        window.clearTimeout(copyFeedbackTimeoutRef.current);
      }
    },
    [],
  );

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";
  const updateTask = updates?.tasks.find((task) => task.nodeId === node.id);
  const updateAvailable = nodeHasAvailableUpdate(node, updates);
  const activeUpdate =
    updateTask !== undefined && !isTerminalUpdateTask(updateTask.status);
  const nodeLocked =
    node.status === "revoked" || node.deletionStatus !== undefined;

  async function saveBasic(event: FormEvent) {
    event.preventDefault();
    setWorking("basic");
    setFeedback(undefined);
    try {
      const updated = await updateNode(
        nodeId,
        { name: name.trim(), enabled },
        csrfToken,
      );
      replaceNode(updated);
      setName(updated.name);
      setEnabled(updated.enabled);
      setBasicDirty(false);
      setFeedback({
        kind: "success",
        message: t("nodeDetail.settings.basic.saved"),
      });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(undefined);
    }
  }

  async function saveProbeSettings() {
    if (!probe) return;
    setWorking("probe");
    setFeedback(undefined);
    try {
      const updated = await updateNodeProbeSettings(
        nodeId,
        {
          schedule: probe.schedule,
          probeOnNewAddress,
          lowMemoryOverride,
        },
        csrfToken,
      );
      setProbe(updated);
      setProbeOnNewAddress(updated.probeOnNewAddress);
      setLowMemoryOverride(updated.lowMemoryOverride);
      setFeedback({
        kind: "success",
        message: t("nodeDetail.settings.discovery.saved"),
      });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(undefined);
    }
  }

  async function requestUpdate() {
    const targetVersion = updates?.availableRelease?.version;
    if (!targetVersion) return;
    setWorking("update");
    setFeedback(undefined);
    try {
      const result = await createAgentUpdateTasks(
        [nodeId],
        targetVersion,
        csrfToken,
      );
      const item = result.items[0];
      if (!item?.accepted || item.task === undefined) {
        setFeedback({
          kind: "error",
          message: t(`nodes.updates.errors.${item?.error ?? "unknown"}`),
        });
        return;
      }
      setUpdates((current) =>
        current === undefined
          ? current
          : {
              ...current,
              tasks: [
                item.task!,
                ...current.tasks.filter((task) => task.nodeId !== nodeId),
              ],
            },
      );
      setFeedback({
        kind: "success",
        message: t("nodeDetail.settings.agent.updateAccepted"),
      });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(undefined);
    }
  }

  async function revoke() {
    setWorking("revoke");
    setFeedback(undefined);
    try {
      replaceNode(await revokeNode(nodeId, csrfToken));
      setFeedback({
        kind: "success",
        message: t("nodeDetail.settings.danger.revoked"),
      });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(undefined);
    }
  }

  async function remove() {
    setWorking("delete");
    setFeedback(undefined);
    try {
      const deletion = await deleteNode(nodeId, csrfToken);
      replaceNode({
        ...node,
        status: "revoked",
        enabled: false,
        deletionStatus: deletion.status,
        deletionError: deletion.error,
      });
      setFeedback({
        kind: "success",
        message: t("nodeDetail.settings.danger.deletionQueued"),
      });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(undefined);
    }
  }

  async function copyUninstallCommand(mode: UninstallMode) {
    const command =
      mode === "purge" ? purgingUninstallCommand : preservingUninstallCommand;
    try {
      await navigator.clipboard.writeText(command);
      setCopiedUninstall(mode);
      if (copyFeedbackTimeoutRef.current !== undefined) {
        window.clearTimeout(copyFeedbackTimeoutRef.current);
      }
      copyFeedbackTimeoutRef.current = window.setTimeout(() => {
        setCopiedUninstall(undefined);
        copyFeedbackTimeoutRef.current = undefined;
      }, 2_000);
    } catch {
      setCopiedUninstall(undefined);
      setFeedback({
        kind: "error",
        message: t("nodeDetail.settings.removal.copyFailed"),
      });
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

      <div className="grid min-w-0 items-start gap-4 xl:grid-cols-[minmax(0,2.5fr)_minmax(300px,.8fr)]">
        <div className="min-w-0 space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <ServerCog aria-hidden="true" className="size-4" />
                {t("nodeDetail.settings.basic.title")}
              </CardTitle>
              <CardDescription>
                {t("nodeDetail.settings.basic.detail")}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form className="space-y-5" onSubmit={saveBasic}>
                <div className="space-y-2">
                  <Label htmlFor="node-name">
                    {t("nodeDetail.settings.basic.name")}
                  </Label>
                  <Input
                    id="node-name"
                    required
                    minLength={1}
                    maxLength={128}
                    disabled={working !== undefined || nodeLocked}
                    value={name}
                    onChange={(event) => {
                      setName(event.target.value);
                      setBasicDirty(true);
                    }}
                  />
                </div>
                <SettingRow
                  title={t("nodeDetail.settings.availability.title")}
                  detail={t("nodeDetail.settings.availability.detail")}
                  control={
                    <Switch
                      checked={enabled}
                      disabled={working !== undefined || nodeLocked}
                      onCheckedChange={(value) => {
                        setEnabled(value);
                        setBasicDirty(true);
                      }}
                      aria-label={t("nodeDetail.settings.availability.toggle")}
                    />
                  }
                />
                <div className="flex justify-end">
                  <Button
                    type="submit"
                    disabled={
                      working !== undefined ||
                      nodeLocked ||
                      name.trim().length === 0
                    }
                  >
                    {working === "basic" ? (
                      <LoaderCircle
                        data-icon="inline-start"
                        aria-hidden="true"
                        className="animate-spin"
                      />
                    ) : (
                      <Save data-icon="inline-start" aria-hidden="true" />
                    )}
                    {t("nodeDetail.settings.basic.save")}
                  </Button>
                </div>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Radar aria-hidden="true" className="size-4" />
                {t("nodeDetail.settings.discovery.title")}
              </CardTitle>
              <CardDescription>
                {t("nodeDetail.settings.discovery.detail")}
              </CardDescription>
              {probe ? (
                <CardAction>
                  <Badge variant="success">
                    {t("nodeDetail.settings.discovery.running")}
                  </Badge>
                </CardAction>
              ) : null}
            </CardHeader>
            <CardContent className="space-y-5">
              {probeLoadFailed ? (
                <Alert variant="destructive">
                  <TriangleAlert aria-hidden="true" />
                  <AlertDescription>
                    {t("nodeDetail.settings.discovery.loadFailed")}
                  </AlertDescription>
                </Alert>
              ) : null}
              {!probe && !probeLoadFailed ? (
                <div className="space-y-3" aria-busy="true">
                  <Skeleton className="h-16 w-full" />
                  <Skeleton className="h-16 w-full" />
                </div>
              ) : null}
              {probe ? (
                <>
                  <SettingRow
                    title={t("probe.settings.probeOnNewAddress")}
                    detail={t("probe.settings.probeOnNewAddressDetail")}
                    control={
                      <Switch
                        checked={probeOnNewAddress}
                        disabled={working !== undefined || nodeLocked}
                        onCheckedChange={setProbeOnNewAddress}
                        aria-label={t("probe.settings.probeOnNewAddress")}
                      />
                    }
                  />
                  <SettingRow
                    title={t("probe.settings.memoryOverride")}
                    detail={t("probe.settings.memoryOverrideDetail")}
                    control={
                      <Switch
                        checked={lowMemoryOverride}
                        disabled={working !== undefined || nodeLocked}
                        onCheckedChange={setLowMemoryOverride}
                        aria-label={t("probe.settings.memoryOverride")}
                      />
                    }
                  />
                  <div className="flex justify-end">
                    <Button
                      type="button"
                      disabled={working !== undefined || nodeLocked}
                      onClick={() => void saveProbeSettings()}
                    >
                      {working === "probe" ? (
                        <LoaderCircle
                          data-icon="inline-start"
                          aria-hidden="true"
                          className="animate-spin"
                        />
                      ) : (
                        <Save data-icon="inline-start" aria-hidden="true" />
                      )}
                      {t("nodeDetail.settings.discovery.save")}
                    </Button>
                  </div>
                </>
              ) : null}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <PackageX aria-hidden="true" className="size-4" />
                {t("nodeDetail.settings.removal.title")}
              </CardTitle>
              <CardDescription>
                {t("nodeDetail.settings.removal.detail")}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <UninstallCommand
                title={t("nodeDetail.settings.removal.preserve.title")}
                detail={t("nodeDetail.settings.removal.preserve.detail")}
                action={t("nodeDetail.settings.removal.preserve.action")}
                command={preservingUninstallCommand}
                copied={copiedUninstall === "preserve"}
                copiedLabel={t("nodeDetail.settings.removal.copied")}
                onCopy={() => void copyUninstallCommand("preserve")}
              />
              <UninstallCommand
                title={t("nodeDetail.settings.removal.purge.title")}
                detail={t("nodeDetail.settings.removal.purge.detail")}
                action={t("nodeDetail.settings.removal.purge.action")}
                command={purgingUninstallCommand}
                copied={copiedUninstall === "purge"}
                copiedLabel={t("nodeDetail.settings.removal.copied")}
                destructive
                onCopy={() => void copyUninstallCommand("purge")}
              />
            </CardContent>
          </Card>
        </div>

        <aside className="min-w-0 space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <KeyRound aria-hidden="true" className="size-4" />
                {t("nodeDetail.settings.agent.title")}
              </CardTitle>
              <CardDescription>
                {t("nodeDetail.settings.agent.detail")}
              </CardDescription>
              <CardAction>
                <Badge variant="outline">{node.agentVersion}</Badge>
              </CardAction>
            </CardHeader>
            <CardContent className="space-y-4">
              <dl className="space-y-3">
                <SettingValue
                  label={t("nodeDetail.settings.agent.registered")}
                  value={formatTime(
                    node.registeredAt,
                    i18n.resolvedLanguage,
                    t("nodes.notAvailable"),
                  )}
                />
                <SettingValue
                  label={t("nodeDetail.settings.agent.platform")}
                  value={`${node.operatingSystem}/${node.architecture}`}
                />
                <SettingValue
                  label={t("nodeDetail.settings.agent.source")}
                  value={
                    node.sourceRevision?.slice(0, 12) ?? t("nodes.notAvailable")
                  }
                />
                <SettingValue
                  label={t("nodeDetail.settings.agent.credential")}
                  value={
                    node.status === "revoked"
                      ? t("nodeDetail.settings.agent.revoked")
                      : t("nodeDetail.settings.agent.valid")
                  }
                  success={node.status !== "revoked"}
                />
              </dl>
              {updateLoadFailed ? (
                <Alert>
                  <TriangleAlert aria-hidden="true" />
                  <AlertDescription>
                    {t("nodes.updates.loadFailedDetail")}
                  </AlertDescription>
                </Alert>
              ) : null}
              {updateTask ? (
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant={activeUpdate ? "info" : "outline"}>
                    {t(`nodes.updates.status.${updateTask.status}`)}
                  </Badge>
                  <span className="text-sm text-muted-foreground">
                    {t("nodes.updates.target", {
                      version: updateTask.targetVersion,
                    })}
                  </span>
                </div>
              ) : null}
              {updateAvailable && !activeUpdate ? (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={
                    working !== undefined ||
                    !canRequestAgentUpdate(node, updateTask, updates)
                  }
                  onClick={() => void requestUpdate()}
                >
                  {working === "update" ? (
                    <LoaderCircle
                      data-icon="inline-start"
                      aria-hidden="true"
                      className="animate-spin"
                    />
                  ) : (
                    <CircleArrowUp
                      data-icon="inline-start"
                      aria-hidden="true"
                    />
                  )}
                  {t("nodes.updates.updateAction")}
                </Button>
              ) : null}
              {!updateAvailable && !activeUpdate && !updateLoadFailed ? (
                <p className="text-sm text-muted-foreground">
                  {t("nodeDetail.settings.agent.current")}
                </p>
              ) : null}
            </CardContent>
          </Card>

          <Card className="ring-destructive/30">
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-destructive">
                <ShieldX aria-hidden="true" className="size-4" />
                {t("nodeDetail.settings.danger.title")}
              </CardTitle>
              <CardDescription>
                {t("nodeDetail.settings.danger.detail")}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {node.status !== "revoked" &&
              node.deletionStatus === undefined ? (
                <DangerAction
                  title={t("nodeDetail.settings.danger.revokeTitle")}
                  detail={t("nodeDetail.settings.danger.revokeDetail")}
                >
                  <AlertDialog>
                    <AlertDialogTrigger asChild>
                      <Button
                        variant="destructive"
                        size="sm"
                        disabled={working !== undefined}
                      >
                        <ShieldX data-icon="inline-start" aria-hidden="true" />
                        {t("nodes.actions.revoke")}
                      </Button>
                    </AlertDialogTrigger>
                    <AlertDialogContent>
                      <AlertDialogHeader>
                        <AlertDialogMedia>
                          <ShieldX aria-hidden="true" />
                        </AlertDialogMedia>
                        <AlertDialogTitle>
                          {t("nodes.revoke.title", { name: node.name })}
                        </AlertDialogTitle>
                        <AlertDialogDescription>
                          {t("nodes.revoke.detail")}
                        </AlertDialogDescription>
                      </AlertDialogHeader>
                      <AlertDialogFooter>
                        <AlertDialogCancel>
                          {t("common.cancel")}
                        </AlertDialogCancel>
                        <AlertDialogAction
                          variant="destructive"
                          onClick={() => void revoke()}
                        >
                          {t("nodes.revoke.confirm")}
                        </AlertDialogAction>
                      </AlertDialogFooter>
                    </AlertDialogContent>
                  </AlertDialog>
                </DangerAction>
              ) : null}

              <DangerAction
                title={t("nodeDetail.settings.danger.deleteTitle")}
                detail={t("nodeDetail.settings.danger.deleteDetail")}
              >
                <AlertDialog>
                  <AlertDialogTrigger asChild>
                    <Button
                      variant="destructive"
                      size="sm"
                      disabled={
                        working !== undefined ||
                        node.deletionStatus === "pending"
                      }
                    >
                      <Trash2 data-icon="inline-start" aria-hidden="true" />
                      {node.deletionStatus === "failed"
                        ? t("nodes.actions.retryDeletion")
                        : t("nodes.actions.delete")}
                    </Button>
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogMedia>
                        <Trash2 aria-hidden="true" />
                      </AlertDialogMedia>
                      <AlertDialogTitle>
                        {t("nodes.deletion.title", { name: node.name })}
                      </AlertDialogTitle>
                      <AlertDialogDescription>
                        {t("nodes.deletion.detail")}
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>
                        {t("common.cancel")}
                      </AlertDialogCancel>
                      <AlertDialogAction
                        variant="destructive"
                        onClick={() => void remove()}
                      >
                        {node.deletionStatus === "failed"
                          ? t("nodes.actions.retryDeletion")
                          : t("nodes.deletion.confirm")}
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </DangerAction>
            </CardContent>
          </Card>
        </aside>
      </div>
    </div>
  );
}

function SettingRow({
  title,
  detail,
  control,
}: {
  title: string;
  detail: string;
  control: React.ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-5 border-t pt-5">
      <div className="min-w-0">
        <p className="text-sm font-medium">{title}</p>
        <p className="mt-1 max-w-2xl text-sm leading-6 text-muted-foreground">
          {detail}
        </p>
      </div>
      <div className="shrink-0">{control}</div>
    </div>
  );
}

function SettingValue({
  label,
  value,
  success = false,
}: {
  label: string;
  value: string;
  success?: boolean;
}) {
  return (
    <div className="flex items-start justify-between gap-4 border-t pt-3 first:border-t-0 first:pt-0">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd
        className={
          success
            ? "break-all text-right text-sm font-medium text-emerald-700 dark:text-emerald-300"
            : "break-all text-right text-sm font-medium"
        }
      >
        {value}
      </dd>
    </div>
  );
}

function DangerAction({
  title,
  detail,
  children,
}: {
  title: string;
  detail: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-3 border-t pt-4 first:border-t-0 first:pt-0">
      <div>
        <p className="text-sm font-medium">{title}</p>
        <p className="mt-1 text-sm text-muted-foreground">{detail}</p>
      </div>
      {children}
    </div>
  );
}

function UninstallCommand({
  title,
  detail,
  action,
  command,
  copied,
  copiedLabel,
  destructive = false,
  onCopy,
}: {
  title: string;
  detail: string;
  action: string;
  command: string;
  copied: boolean;
  copiedLabel: string;
  destructive?: boolean;
  onCopy: () => void;
}) {
  return (
    <section
      className={
        destructive
          ? "space-y-3 rounded-md border border-destructive/40 p-4"
          : "space-y-3 rounded-md border p-4"
      }
    >
      <div>
        <h3
          className={
            destructive
              ? "text-sm font-medium text-destructive"
              : "text-sm font-medium"
          }
        >
          {title}
        </h3>
        <p className="mt-1 text-sm text-muted-foreground">{detail}</p>
      </div>
      <pre className="overflow-x-auto rounded-md bg-muted p-3 text-sm leading-5">
        <code>{command}</code>
      </pre>
      <Tooltip open={copied}>
        <TooltipTrigger asChild>
          <Button variant="outline" size="sm" onClick={onCopy}>
            <Clipboard data-icon="inline-start" aria-hidden="true" />
            {action}
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top" sideOffset={6}>
          {copiedLabel}
        </TooltipContent>
      </Tooltip>
    </section>
  );
}
