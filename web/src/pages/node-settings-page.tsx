import { useEffect, useState } from "react";
import {
  CircleArrowUp,
  KeyRound,
  LoaderCircle,
  ServerCog,
  ShieldX,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router";

import { deleteNode, revokeNode, updateNode } from "@/api/nodes";
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
import { Switch } from "@/components/ui/switch";
import { formatAPIError } from "@/lib/api-error";
import {
  canRequestAgentUpdate,
  isTerminalUpdateTask,
  nodeHasAvailableUpdate,
} from "@/lib/agent-update";

type WorkingAction = "toggle" | "update" | "revoke" | "delete";

export function NodeSettingsPage() {
  const { nodeId = "" } = useParams();
  const { node, replaceNode } = useNodeDetail();
  const { state: authState } = useAuth();
  const { t } = useTranslation();
  const [updates, setUpdates] = useState<AgentUpdateState>();
  const [updateLoadFailed, setUpdateLoadFailed] = useState(false);
  const [working, setWorking] = useState<WorkingAction>();
  const [feedback, setFeedback] = useState<
    { kind: "success" | "error"; message: string } | undefined
  >();

  useEffect(() => {
    const controller = new AbortController();
    getAgentUpdateState(controller.signal)
      .then((value) => setUpdates(value))
      .catch((error: unknown) => {
        if (!(error instanceof DOMException && error.name === "AbortError")) {
          setUpdateLoadFailed(true);
        }
      });
    return () => controller.abort();
  }, []);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";
  const updateTask = updates?.tasks.find((task) => task.nodeId === node.id);
  const updateAvailable = nodeHasAvailableUpdate(node, updates);
  const activeUpdate =
    updateTask !== undefined && !isTerminalUpdateTask(updateTask.status);

  async function toggleEnabled(enabled: boolean) {
    setWorking("toggle");
    setFeedback(undefined);
    try {
      replaceNode(await updateNode(nodeId, enabled, csrfToken));
      setFeedback({ kind: "success", message: t("nodeDetail.settings.saved") });
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
      const task = item.task;
      setUpdates((current) =>
        current === undefined
          ? current
          : {
              ...current,
              tasks: [
                task,
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

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ServerCog aria-hidden="true" className="size-4" />
            {t("nodeDetail.settings.availability.title")}
          </CardTitle>
          <CardDescription>
            {t("nodeDetail.settings.availability.detail")}
          </CardDescription>
          <CardAction>
            <Switch
              checked={node.enabled}
              disabled={
                working !== undefined ||
                node.status === "revoked" ||
                node.deletionStatus !== undefined
              }
              onCheckedChange={(enabled) => void toggleEnabled(enabled)}
              aria-label={t("nodeDetail.settings.availability.toggle")}
            />
          </CardAction>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            {node.enabled
              ? t("nodeDetail.settings.availability.enabled")
              : t("nodeDetail.settings.availability.disabled")}
          </p>
        </CardContent>
      </Card>

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
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
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
              label={t("nodeDetail.settings.agent.capabilities")}
              value={String(node.capabilities.length)}
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
              <Badge variant={activeUpdate ? "secondary" : "outline"}>
                {t(`nodes.updates.status.${updateTask.status}`)}
              </Badge>
              <span className="text-xs text-muted-foreground">
                {t("nodes.updates.target", {
                  version: updateTask.targetVersion,
                })}
              </span>
            </div>
          ) : null}
          {updateAvailable && !activeUpdate ? (
            <Button
              variant="outline"
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
                <CircleArrowUp data-icon="inline-start" aria-hidden="true" />
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
        <CardContent className="flex flex-wrap gap-2">
          {node.status !== "revoked" && node.deletionStatus === undefined ? (
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button variant="destructive" disabled={working !== undefined}>
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
                  <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                  <AlertDialogAction
                    variant="destructive"
                    onClick={() => void revoke()}
                  >
                    {t("nodes.revoke.confirm")}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          ) : null}

          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="destructive"
                disabled={
                  working !== undefined || node.deletionStatus === "pending"
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
                <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
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
        </CardContent>
      </Card>
    </div>
  );
}

function SettingValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-words font-medium">{value}</dd>
    </div>
  );
}
