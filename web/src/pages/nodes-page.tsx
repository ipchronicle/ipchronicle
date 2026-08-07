import { useCallback, useEffect, useState } from "react";
import {
  Check,
  Clipboard,
  KeyRound,
  LoaderCircle,
  Pause,
  Play,
  RefreshCw,
  RotateCw,
  Server,
  ShieldX,
  Terminal,
  Trash2,
  TriangleAlert,
  Wifi,
  WifiOff,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  deleteNode,
  getAgentEnrollment,
  listNodes,
  revokeNode,
  rotateAgentEnrollmentKey,
  updateNode,
  updateAgentEnrollment,
  type AgentEnrollmentSettings,
  type Node,
  type NodeDeletion,
} from "@/api/nodes";
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
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { formatAPIError } from "@/lib/api-error";

type ViewState =
  | { kind: "loading" }
  | {
      kind: "success";
      nodes: Node[];
      enrollment: AgentEnrollmentSettings;
    }
  | { kind: "error" };

export function NodesPage() {
  const { t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async (signal?: AbortSignal, initial = false) => {
    if (initial) setState({ kind: "loading" });
    else setRefreshing(true);
    try {
      const [nodes, enrollment] = await Promise.all([
        listNodes(signal),
        getAgentEnrollment(signal),
      ]);
      setState({ kind: "success", nodes, enrollment });
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      if (initial) setState({ kind: "error" });
    } finally {
      if (!initial) setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    const interval = window.setInterval(() => void load(), 30_000);
    return () => {
      controller.abort();
      window.clearInterval(interval);
    };
  }, [load]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  return (
    <div className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          <p className="text-xs font-medium text-muted-foreground uppercase">
            {t("nodes.section")}
          </p>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
            {t("nodes.title")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("nodes.detail")}
          </p>
        </div>
        <Button
          variant="outline"
          onClick={() => void load()}
          disabled={refreshing || state.kind === "loading"}
        >
          <RefreshCw
            data-icon="inline-start"
            aria-hidden="true"
            className={refreshing ? "animate-spin" : undefined}
          />
          {t("nodes.refresh")}
        </Button>
      </div>

      <div className="mt-8 space-y-4" aria-live="polite">
        {state.kind === "loading" ? <NodesSkeleton /> : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("nodes.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                className="mt-3"
                variant="outline"
                size="sm"
                onClick={() => void load(undefined, true)}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("nodes.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <>
            <EnrollmentCard
              enrollment={state.enrollment}
              csrfToken={csrfToken}
              onChange={(enrollment) =>
                setState((current) =>
                  current.kind === "success"
                    ? { ...current, enrollment }
                    : current,
                )
              }
            />
            <NodeListCard
              nodes={state.nodes}
              csrfToken={csrfToken}
              onNodeChange={(node) =>
                setState((current) =>
                  current.kind === "success"
                    ? {
                        ...current,
                        nodes: current.nodes.map((item) =>
                          item.id === node.id ? node : item,
                        ),
                      }
                    : current,
                )
              }
              onDeletionQueued={(deletion) =>
                setState((current) =>
                  current.kind === "success"
                    ? {
                        ...current,
                        nodes: current.nodes.map((item) =>
                          item.id === deletion.nodeId
                            ? {
                                ...item,
                                status: "revoked",
                                enabled: false,
                                deletionStatus: deletion.status,
                                deletionError: deletion.error,
                              }
                            : item,
                        ),
                      }
                    : current,
                )
              }
            />
          </>
        ) : null}
      </div>
    </div>
  );
}

function EnrollmentCard({
  enrollment,
  csrfToken,
  onChange,
}: {
  enrollment: AgentEnrollmentSettings;
  csrfToken: string;
  onChange: (value: AgentEnrollmentSettings) => void;
}) {
  const { i18n, t } = useTranslation();
  const [working, setWorking] = useState(false);
  const [feedback, setFeedback] = useState<
    { kind: "success" | "error"; message: string } | undefined
  >();

  async function setEnabled(enabled: boolean) {
    setWorking(true);
    setFeedback(undefined);
    try {
      onChange(await updateAgentEnrollment(enabled, csrfToken));
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(false);
    }
  }

  async function rotate() {
    setWorking(true);
    setFeedback(undefined);
    try {
      onChange(await rotateAgentEnrollmentKey(csrfToken));
      setFeedback({
        kind: "success",
        message: enrollment.hasKey
          ? t("nodes.enrollment.rotated")
          : t("nodes.enrollment.generated"),
      });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(false);
    }
  }

  async function copyCommand() {
    if (enrollment.installationCommand === undefined) return;
    try {
      await navigator.clipboard.writeText(enrollment.installationCommand);
      setFeedback({
        kind: "success",
        message: t("nodes.enrollment.copied"),
      });
    } catch {
      setFeedback({ kind: "error", message: t("errors.actionFailed") });
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <KeyRound aria-hidden="true" className="size-4" />
          {t("nodes.enrollment.title")}
        </CardTitle>
        <CardDescription>{t("nodes.enrollment.detail")}</CardDescription>
        <CardAction>
          <Badge variant={enrollment.enabled ? "outline" : "secondary"}>
            {enrollment.enabled
              ? t("nodes.enrollment.enabled")
              : t("nodes.enrollment.disabled")}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-5">
        {feedback ? (
          <Alert
            variant={feedback.kind === "error" ? "destructive" : "default"}
          >
            {feedback.kind === "error" ? (
              <TriangleAlert aria-hidden="true" />
            ) : (
              <Check aria-hidden="true" />
            )}
            <AlertDescription>{feedback.message}</AlertDescription>
          </Alert>
        ) : null}

        {enrollment.hasKey ? (
          <>
            <div className="flex items-start justify-between gap-4 rounded-md border p-4">
              <div>
                <p className="text-sm font-medium">
                  {t("nodes.enrollment.allowRegistration")}
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t("nodes.enrollment.allowRegistrationDetail")}
                </p>
              </div>
              <Switch
                checked={enrollment.enabled}
                disabled={working}
                onCheckedChange={(checked) => void setEnabled(checked)}
                aria-label={t("nodes.enrollment.allowRegistration")}
              />
            </div>

            <div>
              <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                <div>
                  <p className="text-sm font-medium">
                    {t("nodes.enrollment.command")}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("nodes.enrollment.commandDetail")}
                  </p>
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={working}
                  onClick={() => void copyCommand()}
                >
                  <Clipboard data-icon="inline-start" aria-hidden="true" />
                  {t("nodes.enrollment.copy")}
                </Button>
              </div>
              <pre className="overflow-x-auto rounded-md bg-muted p-4 text-xs leading-5">
                <code>{enrollment.installationCommand}</code>
              </pre>
            </div>

            <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-4">
              <p className="text-xs text-muted-foreground">
                {t("nodes.enrollment.rotatedAt", {
                  value:
                    enrollment.rotatedAt === undefined
                      ? t("nodes.notAvailable")
                      : new Intl.DateTimeFormat(i18n.resolvedLanguage, {
                          dateStyle: "medium",
                          timeStyle: "short",
                        }).format(new Date(enrollment.rotatedAt)),
                })}
              </p>
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button variant="outline" size="sm" disabled={working}>
                    <RotateCw data-icon="inline-start" aria-hidden="true" />
                    {t("nodes.enrollment.rotate")}
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogMedia>
                      <KeyRound aria-hidden="true" />
                    </AlertDialogMedia>
                    <AlertDialogTitle>
                      {t("nodes.enrollment.rotateTitle")}
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      {t("nodes.enrollment.rotateDetail")}
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                    <AlertDialogAction onClick={() => void rotate()}>
                      {t("nodes.enrollment.rotateConfirm")}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </div>
          </>
        ) : (
          <div className="flex flex-col items-start gap-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-3">
              <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                <Terminal aria-hidden="true" className="size-4" />
              </span>
              <div>
                <p className="font-medium">{t("nodes.enrollment.empty")}</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t("nodes.enrollment.emptyDetail")}
                </p>
              </div>
            </div>
            <Button disabled={working} onClick={() => void rotate()}>
              {working ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <KeyRound data-icon="inline-start" aria-hidden="true" />
              )}
              {t("nodes.enrollment.generate")}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function NodeListCard({
  nodes,
  csrfToken,
  onNodeChange,
  onDeletionQueued,
}: {
  nodes: Node[];
  csrfToken: string;
  onNodeChange: (node: Node) => void;
  onDeletionQueued: (deletion: NodeDeletion) => void;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Server aria-hidden="true" className="size-4" />
          {t("nodes.inventory.title")}
        </CardTitle>
        <CardDescription>{t("nodes.inventory.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">
            {t("nodes.inventory.count", { count: nodes.length })}
          </Badge>
        </CardAction>
      </CardHeader>
      {nodes.length === 0 ? (
        <CardContent>
          <div className="flex flex-col items-center py-10 text-center">
            <span className="flex size-10 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <Server aria-hidden="true" className="size-5" />
            </span>
            <p className="mt-4 font-medium">{t("nodes.inventory.empty")}</p>
            <p className="mt-1 max-w-md text-sm text-muted-foreground">
              {t("nodes.inventory.emptyDetail")}
            </p>
          </div>
        </CardContent>
      ) : (
        <>
          <div className="hidden md:block">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("nodes.inventory.node")}</TableHead>
                  <TableHead>{t("nodes.inventory.status")}</TableHead>
                  <TableHead>{t("nodes.inventory.agent")}</TableHead>
                  <TableHead>{t("nodes.inventory.configuration")}</TableHead>
                  <TableHead className="text-right">
                    {t("nodes.inventory.lastSeen")}
                  </TableHead>
                  <TableHead className="w-28 text-right">
                    <span className="sr-only">{t("nodes.actions.title")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodes.map((node) => (
                  <TableRow key={node.id}>
                    <TableCell>
                      <p className="font-medium">{node.name}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {node.hostname}
                      </p>
                    </TableCell>
                    <TableCell>
                      <NodeStatusBadge node={node} />
                    </TableCell>
                    <TableCell>
                      <p>{node.agentVersion}</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {node.operatingSystem}/{node.architecture}
                      </p>
                    </TableCell>
                    <TableCell>
                      <ConfigurationStatus node={node} />
                    </TableCell>
                    <TableCell className="text-right text-muted-foreground">
                      <NodeTime value={node.lastSeenAt} />
                    </TableCell>
                    <TableCell>
                      <NodeActions
                        node={node}
                        csrfToken={csrfToken}
                        onNodeChange={onNodeChange}
                        onDeletionQueued={onDeletionQueued}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <CardContent className="divide-y p-0 md:hidden">
            {nodes.map((node) => (
              <div className="space-y-4 p-4" key={node.id}>
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate font-medium">{node.name}</p>
                    <p className="mt-1 truncate text-xs text-muted-foreground">
                      {node.hostname}
                    </p>
                  </div>
                  <NodeStatusBadge node={node} />
                </div>
                <dl className="grid grid-cols-2 gap-3 text-xs">
                  <div>
                    <dt className="text-muted-foreground">
                      {t("nodes.inventory.agent")}
                    </dt>
                    <dd className="mt-1">
                      {node.agentVersion} · {node.architecture}
                    </dd>
                  </div>
                  <div>
                    <dt className="text-muted-foreground">
                      {t("nodes.inventory.configuration")}
                    </dt>
                    <dd className="mt-1">
                      <ConfigurationStatus node={node} />
                    </dd>
                  </div>
                  <div className="col-span-2">
                    <dt className="text-muted-foreground">
                      {t("nodes.inventory.lastSeen")}
                    </dt>
                    <dd className="mt-1">
                      <NodeTime value={node.lastSeenAt} />
                    </dd>
                  </div>
                </dl>
                <NodeActions
                  node={node}
                  csrfToken={csrfToken}
                  onNodeChange={onNodeChange}
                  onDeletionQueued={onDeletionQueued}
                />
              </div>
            ))}
          </CardContent>
        </>
      )}
    </Card>
  );
}

function NodeStatusBadge({ node }: { node: Node }) {
  const { t } = useTranslation();
  const labels = {
    online: t("nodes.status.online"),
    offline: t("nodes.status.offline"),
    disabled: t("nodes.status.disabled"),
    revoked: t("nodes.status.revoked"),
  };
  if (node.deletionStatus !== undefined) {
    return (
      <Badge variant="destructive">
        <Trash2 aria-hidden="true" />
        {node.deletionStatus === "failed"
          ? t("nodes.deletion.failed")
          : t("nodes.deletion.pending")}
      </Badge>
    );
  }
  return (
    <Badge
      variant={
        node.status === "online"
          ? "outline"
          : node.status === "offline"
            ? "secondary"
            : "destructive"
      }
    >
      {node.status === "online" ? (
        <Wifi aria-hidden="true" />
      ) : (
        <WifiOff aria-hidden="true" />
      )}
      {labels[node.status]}
    </Badge>
  );
}

function NodeActions({
  node,
  csrfToken,
  onNodeChange,
  onDeletionQueued,
}: {
  node: Node;
  csrfToken: string;
  onNodeChange: (node: Node) => void;
  onDeletionQueued: (deletion: NodeDeletion) => void;
}) {
  const { t } = useTranslation();
  const [working, setWorking] = useState(false);
  const [error, setError] = useState<string>();
  const deletionPending = node.deletionStatus === "pending";

  async function toggleEnabled() {
    setWorking(true);
    setError(undefined);
    try {
      onNodeChange(await updateNode(node.id, !node.enabled, csrfToken));
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setWorking(false);
    }
  }

  async function revoke() {
    setWorking(true);
    setError(undefined);
    try {
      onNodeChange(await revokeNode(node.id, csrfToken));
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setWorking(false);
    }
  }

  async function remove() {
    setWorking(true);
    setError(undefined);
    try {
      onDeletionQueued(await deleteNode(node.id, csrfToken));
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setWorking(false);
    }
  }

  return (
    <div className="flex flex-wrap items-center justify-end gap-1">
      {node.status !== "revoked" && node.deletionStatus === undefined ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={working}
              aria-label={
                node.enabled
                  ? t("nodes.actions.disable")
                  : t("nodes.actions.enable")
              }
              onClick={() => void toggleEnabled()}
            >
              {working ? (
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              ) : node.enabled ? (
                <Pause aria-hidden="true" />
              ) : (
                <Play aria-hidden="true" />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            {node.enabled
              ? t("nodes.actions.disable")
              : t("nodes.actions.enable")}
          </TooltipContent>
        </Tooltip>
      ) : null}

      {node.status !== "revoked" && node.deletionStatus === undefined ? (
        <AlertDialog>
          <Tooltip>
            <TooltipTrigger asChild>
              <AlertDialogTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  disabled={working}
                  aria-label={t("nodes.actions.revoke")}
                >
                  <ShieldX aria-hidden="true" />
                </Button>
              </AlertDialogTrigger>
            </TooltipTrigger>
            <TooltipContent>{t("nodes.actions.revoke")}</TooltipContent>
          </Tooltip>
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
        <Tooltip>
          <TooltipTrigger asChild>
            <AlertDialogTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={working || deletionPending}
                aria-label={
                  node.deletionStatus === "failed"
                    ? t("nodes.actions.retryDeletion")
                    : t("nodes.actions.delete")
                }
              >
                <Trash2 aria-hidden="true" />
              </Button>
            </AlertDialogTrigger>
          </TooltipTrigger>
          <TooltipContent>
            {node.deletionStatus === "failed"
              ? t("nodes.actions.retryDeletion")
              : t("nodes.actions.delete")}
          </TooltipContent>
        </Tooltip>
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

      {error !== undefined ? (
        <p className="w-full text-right text-xs text-destructive" role="alert">
          {error}
        </p>
      ) : null}
      {node.deletionError !== undefined ? (
        <p className="w-full text-right text-xs text-destructive" role="alert">
          {node.deletionError}
        </p>
      ) : null}
    </div>
  );
}

function ConfigurationStatus({ node }: { node: Node }) {
  const { t } = useTranslation();
  const labels = {
    current: t("nodes.configuration.current"),
    pending: t("nodes.configuration.pending"),
    failed: t("nodes.configuration.failed"),
  };
  return (
    <span title={node.configurationError}>
      {labels[node.configurationStatus]} · {node.appliedConfigurationRevision}/
      {node.desiredConfigurationRevision}
    </span>
  );
}

function NodeTime({ value }: { value?: string }) {
  const { i18n, t } = useTranslation();
  if (value === undefined) return <>{t("nodes.notAvailable")}</>;
  return (
    <>
      {new Intl.DateTimeFormat(i18n.resolvedLanguage, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(new Date(value))}
    </>
  );
}

function NodesSkeleton() {
  return (
    <>
      {[0, 1].map((item) => (
        <Card key={item} aria-busy="true">
          <CardHeader>
            <Skeleton className="h-5 w-36" />
            <Skeleton className="h-4 w-64 max-w-full" />
          </CardHeader>
          <CardContent>
            <Skeleton className="h-24 w-full" />
          </CardContent>
        </Card>
      ))}
    </>
  );
}
