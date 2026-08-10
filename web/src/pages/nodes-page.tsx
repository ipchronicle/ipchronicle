import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Check,
  CircleArrowUp,
  Clipboard,
  KeyRound,
  LoaderCircle,
  Network as NetworkIcon,
  Pause,
  Play,
  RadioTower,
  RefreshCw,
  RotateCw,
  ScanSearch,
  Server,
  Search,
  ShieldX,
  Terminal,
  Trash2,
  TriangleAlert,
  Unplug,
  Wifi,
  WifiOff,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { gt, major, valid } from "semver";

import {
  deleteNode,
  getAgentEnrollment,
  listNodes,
  revokeNode,
  rotateAgentEnrollmentKey,
  startNodeSyncSession,
  stopNodeSyncSession,
  updateNode,
  updateAgentEnrollment,
  type AgentEnrollmentSettings,
  type Node,
  type NodeDeletion,
} from "@/api/nodes";
import {
  createAgentUpdateTasks,
  getAgentUpdateState,
  type AgentUpdateBatchResult,
  type AgentUpdateState,
  type AgentUpdateTask,
} from "@/api/updates";
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
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
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
      updates?: AgentUpdateState;
      updateLoadFailed: boolean;
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
      const [nodes, enrollment, updates] = await Promise.all([
        listNodes(signal),
        getAgentEnrollment(signal),
        getAgentUpdateState(signal).catch((error: unknown) => {
          if (error instanceof DOMException && error.name === "AbortError") {
            throw error;
          }
          return undefined;
        }),
      ]);
      setState({
        kind: "success",
        nodes,
        enrollment,
        updates,
        updateLoadFailed: updates === undefined,
      });
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
    return () => controller.abort();
  }, [load]);

  const hasActiveSync =
    state.kind === "success" &&
    state.nodes.some((node) => node.syncStatus !== undefined);
  const hasActiveUpdate =
    state.kind === "success" &&
    state.updates?.tasks.some((task) => !isTerminalUpdateTask(task.status));

  useEffect(() => {
    if (state.kind !== "success") return;
    const interval = window.setInterval(
      () => void load(),
      hasActiveSync || hasActiveUpdate ? 3_000 : 30_000,
    );
    return () => window.clearInterval(interval);
  }, [hasActiveSync, hasActiveUpdate, load, state.kind]);

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
              updates={state.updates}
              updateLoadFailed={state.updateLoadFailed}
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
              onUpdateTasksCreated={(result) =>
                setState((current) => {
                  if (
                    current.kind !== "success" ||
                    current.updates === undefined
                  ) {
                    return current;
                  }
                  const replacements = new Map(
                    result.items.flatMap((item) =>
                      item.task === undefined
                        ? []
                        : ([[item.nodeId, item.task]] as const),
                    ),
                  );
                  return {
                    ...current,
                    updates: {
                      ...current.updates,
                      tasks: [
                        ...replacements.values(),
                        ...current.updates.tasks.filter(
                          (task) => !replacements.has(task.nodeId),
                        ),
                      ],
                    },
                  };
                })
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
  updates,
  updateLoadFailed,
  csrfToken,
  onNodeChange,
  onDeletionQueued,
  onUpdateTasksCreated,
}: {
  nodes: Node[];
  updates?: AgentUpdateState;
  updateLoadFailed: boolean;
  csrfToken: string;
  onNodeChange: (node: Node) => void;
  onDeletionQueued: (deletion: NodeDeletion) => void;
  onUpdateTasksCreated: (result: AgentUpdateBatchResult) => void;
}) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const [updatesOnly, setUpdatesOnly] = useState(false);
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<string>>(
    () => new Set(),
  );
  const [updating, setUpdating] = useState(false);
  const [updateFeedback, setUpdateFeedback] = useState<UpdateFeedback>();

  const tasksByNode = useMemo(
    () => new Map(updates?.tasks.map((task) => [task.nodeId, task]) ?? []),
    [updates?.tasks],
  );
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const visibleNodes = nodes.filter((node) => {
    const matchesQuery =
      normalizedQuery.length === 0 ||
      [
        node.name,
        node.hostname,
        node.agentVersion,
        node.sourceRevision ?? "",
      ].some((value) => value.toLocaleLowerCase().includes(normalizedQuery));
    return (
      matchesQuery && (!updatesOnly || nodeHasAvailableUpdate(node, updates))
    );
  });
  const selectableVisibleNodes = visibleNodes.filter((node) =>
    canRequestAgentUpdate(node, tasksByNode.get(node.id), updates),
  );
  const allVisibleSelected =
    selectableVisibleNodes.length > 0 &&
    selectableVisibleNodes.every((node) => selectedNodeIds.has(node.id));
  const someVisibleSelected = selectableVisibleNodes.some((node) =>
    selectedNodeIds.has(node.id),
  );

  useEffect(() => {
    setSelectedNodeIds((current) => {
      const next = new Set(
        [...current].filter((nodeId) => {
          const node = nodes.find((item) => item.id === nodeId);
          return (
            node !== undefined &&
            canRequestAgentUpdate(node, tasksByNode.get(node.id), updates)
          );
        }),
      );
      return next.size === current.size ? current : next;
    });
  }, [nodes, tasksByNode, updates]);

  async function requestUpdates(nodeIds: string[]) {
    const targetVersion = updates?.availableRelease?.version;
    if (targetVersion === undefined || nodeIds.length === 0) return;
    setUpdating(true);
    setUpdateFeedback(undefined);
    try {
      const result = await createAgentUpdateTasks(
        nodeIds,
        targetVersion,
        csrfToken,
      );
      onUpdateTasksCreated(result);
      const failures = result.items.flatMap((item) => {
        if (item.accepted || item.error === undefined) return [];
        return [
          {
            nodeName:
              nodes.find((node) => node.id === item.nodeId)?.name ??
              item.nodeId,
            code: item.error,
          },
        ];
      });
      const acceptedCount = result.items.filter((item) => item.accepted).length;
      setUpdateFeedback({ acceptedCount, failures });
      setSelectedNodeIds((current) => {
        const next = new Set(current);
        result.items.forEach((item) => {
          if (item.accepted) next.delete(item.nodeId);
        });
        return next;
      });
    } catch (cause) {
      setUpdateFeedback({
        acceptedCount: 0,
        failures: [{ nodeName: "", message: formatAPIError(cause, t) }],
      });
    } finally {
      setUpdating(false);
    }
  }

  function toggleVisibleSelection(checked: boolean) {
    setSelectedNodeIds((current) => {
      const next = new Set(current);
      selectableVisibleNodes.forEach((node) => {
        if (checked) next.add(node.id);
        else next.delete(node.id);
      });
      return next;
    });
  }

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
          <CardContent className="space-y-4 border-b">
            {updateLoadFailed ? (
              <Alert>
                <TriangleAlert aria-hidden="true" />
                <AlertTitle>{t("nodes.updates.loadFailed")}</AlertTitle>
                <AlertDescription>
                  {t("nodes.updates.loadFailedDetail")}
                </AlertDescription>
              </Alert>
            ) : null}
            {updateFeedback !== undefined ? (
              <AgentUpdateFeedback value={updateFeedback} />
            ) : null}
            <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
              <div className="relative min-w-0 flex-1 sm:min-w-64">
                <Search
                  aria-hidden="true"
                  className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
                />
                <Input
                  className="pl-9"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={t("nodes.inventory.searchPlaceholder")}
                  aria-label={t("nodes.inventory.search")}
                />
              </div>
              <label className="flex min-h-8 items-center gap-2 text-sm">
                <Switch
                  size="sm"
                  checked={updatesOnly}
                  disabled={updates === undefined}
                  onCheckedChange={setUpdatesOnly}
                  aria-label={t("nodes.updates.filter")}
                />
                {t("nodes.updates.filter")}
              </label>
              <Button
                onClick={() => void requestUpdates([...selectedNodeIds])}
                disabled={
                  updating ||
                  selectedNodeIds.size === 0 ||
                  updates?.availableRelease === undefined
                }
              >
                {updating ? (
                  <LoaderCircle
                    className="animate-spin"
                    data-icon="inline-start"
                    aria-hidden="true"
                  />
                ) : (
                  <CircleArrowUp data-icon="inline-start" aria-hidden="true" />
                )}
                {t("nodes.updates.updateSelected", {
                  count: selectedNodeIds.size,
                })}
              </Button>
            </div>
          </CardContent>

          {visibleNodes.length === 0 ? (
            <CardContent>
              <div className="flex flex-col items-center py-10 text-center">
                <Search
                  aria-hidden="true"
                  className="size-5 text-muted-foreground"
                />
                <p className="mt-4 font-medium">
                  {t("nodes.inventory.noMatches")}
                </p>
                <Button
                  className="mt-3"
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setQuery("");
                    setUpdatesOnly(false);
                  }}
                >
                  {t("nodes.inventory.clearFilters")}
                </Button>
              </div>
            </CardContent>
          ) : null}

          {visibleNodes.length > 0 ? (
            <div className="hidden lg:block">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <Checkbox
                        checked={
                          allVisibleSelected
                            ? true
                            : someVisibleSelected
                              ? "indeterminate"
                              : false
                        }
                        disabled={selectableVisibleNodes.length === 0}
                        onCheckedChange={(checked) =>
                          toggleVisibleSelection(checked === true)
                        }
                        aria-label={t("nodes.updates.selectAvailable")}
                      />
                    </TableHead>
                    <TableHead>{t("nodes.inventory.node")}</TableHead>
                    <TableHead>{t("nodes.inventory.status")}</TableHead>
                    <TableHead>{t("nodes.inventory.agent")}</TableHead>
                    <TableHead>{t("nodes.inventory.configuration")}</TableHead>
                    <TableHead className="text-right">
                      {t("nodes.inventory.lastSeen")}
                    </TableHead>
                    <TableHead className="w-36 text-right">
                      <span className="sr-only">
                        {t("nodes.actions.title")}
                      </span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visibleNodes.map((node) => {
                    const task = tasksByNode.get(node.id);
                    const updateAvailable = nodeHasAvailableUpdate(
                      node,
                      updates,
                    );
                    const canUpdate = canRequestAgentUpdate(
                      node,
                      task,
                      updates,
                    );
                    return (
                      <TableRow key={node.id}>
                        <TableCell>
                          <Checkbox
                            checked={selectedNodeIds.has(node.id)}
                            disabled={!canUpdate}
                            onCheckedChange={(checked) =>
                              setSelectedNodeIds((current) => {
                                const next = new Set(current);
                                if (checked === true) next.add(node.id);
                                else next.delete(node.id);
                                return next;
                              })
                            }
                            aria-label={t("nodes.updates.selectNode", {
                              name: node.name,
                            })}
                          />
                        </TableCell>
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
                          <AgentSourceRevision value={node.sourceRevision} />
                          <p className="mt-1 text-xs text-muted-foreground">
                            {node.operatingSystem}/{node.architecture}
                          </p>
                          <AgentUpdateStatus
                            task={task}
                            updateAvailable={updateAvailable}
                            targetVersion={updates?.availableRelease?.version}
                          />
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
                            updateAvailable={updateAvailable}
                            updateTask={task}
                            updating={updating}
                            onUpdate={() => void requestUpdates([node.id])}
                          />
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          ) : null}
          {visibleNodes.length > 0 ? (
            <CardContent className="divide-y p-0 lg:hidden">
              {visibleNodes.map((node) => {
                const task = tasksByNode.get(node.id);
                const updateAvailable = nodeHasAvailableUpdate(node, updates);
                const canUpdate = canRequestAgentUpdate(node, task, updates);
                return (
                  <div className="space-y-4 p-4" key={node.id}>
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex min-w-0 items-start gap-3">
                        <Checkbox
                          className="mt-1"
                          checked={selectedNodeIds.has(node.id)}
                          disabled={!canUpdate}
                          onCheckedChange={(checked) =>
                            setSelectedNodeIds((current) => {
                              const next = new Set(current);
                              if (checked === true) next.add(node.id);
                              else next.delete(node.id);
                              return next;
                            })
                          }
                          aria-label={t("nodes.updates.selectNode", {
                            name: node.name,
                          })}
                        />
                        <div className="min-w-0">
                          <p className="truncate font-medium">{node.name}</p>
                          <p className="mt-1 truncate text-xs text-muted-foreground">
                            {node.hostname}
                          </p>
                        </div>
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
                          <AgentSourceRevision value={node.sourceRevision} />
                          <AgentUpdateStatus
                            task={task}
                            updateAvailable={updateAvailable}
                            targetVersion={updates?.availableRelease?.version}
                          />
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
                      updateAvailable={updateAvailable}
                      updateTask={task}
                      updating={updating}
                      onUpdate={() => void requestUpdates([node.id])}
                    />
                  </div>
                );
              })}
            </CardContent>
          ) : null}
        </>
      )}
    </Card>
  );
}

type UpdateFeedback = {
  acceptedCount: number;
  failures: Array<{
    nodeName: string;
    code?: string;
    message?: string;
  }>;
};

function AgentUpdateFeedback({ value }: { value: UpdateFeedback }) {
  const { t } = useTranslation();
  const hasFailures = value.failures.length > 0;
  return (
    <Alert variant={hasFailures ? "destructive" : "default"}>
      {hasFailures ? (
        <TriangleAlert aria-hidden="true" />
      ) : (
        <Check aria-hidden="true" />
      )}
      <AlertTitle>
        {t(
          hasFailures
            ? value.acceptedCount > 0
              ? "nodes.updates.partial"
              : "nodes.updates.failed"
            : "nodes.updates.accepted",
          { count: value.acceptedCount },
        )}
      </AlertTitle>
      {hasFailures ? (
        <AlertDescription>
          <ul className="mt-1 list-disc space-y-1 pl-4">
            {value.failures.map((failure, index) => (
              <li key={`${failure.nodeName}-${failure.code ?? index}`}>
                {failure.nodeName === "" ? null : `${failure.nodeName}: `}
                {failure.message ??
                  t(`nodes.updates.errors.${failure.code ?? "unknown"}`)}
              </li>
            ))}
          </ul>
        </AlertDescription>
      ) : null}
    </Alert>
  );
}

function AgentSourceRevision({ value }: { value?: string }) {
  const { t } = useTranslation();
  if (value === undefined) return null;
  return (
    <p
      className="mt-1 truncate font-mono text-xs text-muted-foreground"
      title={value}
    >
      {t("nodes.inventory.sourceRevision", { value: value.slice(0, 12) })}
    </p>
  );
}

function AgentUpdateStatus({
  task,
  updateAvailable,
  targetVersion,
}: {
  task?: AgentUpdateTask;
  updateAvailable: boolean;
  targetVersion?: string;
}) {
  const { t } = useTranslation();
  if (task === undefined) {
    if (!updateAvailable || targetVersion === undefined) return null;
    return (
      <div className="mt-2">
        <Badge variant="secondary">
          <CircleArrowUp aria-hidden="true" />
          {t("nodes.updates.available", { version: targetVersion })}
        </Badge>
      </div>
    );
  }

  const failed = ["failed", "rejected", "rolled-back", "expired"].includes(
    task.status,
  );
  const active = !isTerminalUpdateTask(task.status);
  return (
    <div className="mt-2 max-w-sm space-y-1.5">
      <Badge
        variant={failed ? "destructive" : active ? "secondary" : "outline"}
      >
        {active ? (
          <LoaderCircle
            className={task.offline ? undefined : "animate-spin"}
            aria-hidden="true"
          />
        ) : task.status === "succeeded" ? (
          <Check aria-hidden="true" />
        ) : (
          <TriangleAlert aria-hidden="true" />
        )}
        {task.offline && active
          ? t("nodes.updates.status.offlineWithPhase", {
              phase: t(`nodes.updates.status.${task.status}`),
            })
          : t(`nodes.updates.status.${task.status}`)}
      </Badge>
      <p className="text-xs text-muted-foreground">
        {t("nodes.updates.target", { version: task.targetVersion })}
        {task.resultVersion === undefined
          ? null
          : ` · ${t("nodes.updates.result", { version: task.resultVersion })}`}
      </p>
      {task.failureCode !== undefined ? (
        <p className="break-words font-mono text-xs text-destructive">
          {t("nodes.updates.failureCode", { code: task.failureCode })}
        </p>
      ) : null}
      {task.diagnostic !== undefined ? (
        <p className="break-words text-xs text-destructive">
          {task.diagnostic}
        </p>
      ) : null}
    </div>
  );
}

function nodeHasAvailableUpdate(node: Node, updates?: AgentUpdateState) {
  const targetVersion = valid(updates?.availableRelease?.version ?? "");
  const currentVersion = valid(node.agentVersion);
  return (
    node.capabilities.includes("agent-update-v1") &&
    targetVersion !== null &&
    currentVersion !== null &&
    major(targetVersion) === major(currentVersion) &&
    gt(targetVersion, currentVersion)
  );
}

function canRequestAgentUpdate(
  node: Node,
  task: AgentUpdateTask | undefined,
  updates?: AgentUpdateState,
) {
  return (
    nodeHasAvailableUpdate(node, updates) &&
    node.enabled &&
    node.status === "online" &&
    node.deletionStatus === undefined &&
    (task === undefined || isTerminalUpdateTask(task.status))
  );
}

function isTerminalUpdateTask(status: AgentUpdateTask["status"]) {
  return ["succeeded", "failed", "rolled-back", "rejected", "expired"].includes(
    status,
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
  updateAvailable,
  updateTask,
  updating,
  onUpdate,
}: {
  node: Node;
  csrfToken: string;
  onNodeChange: (node: Node) => void;
  onDeletionQueued: (deletion: NodeDeletion) => void;
  updateAvailable: boolean;
  updateTask?: AgentUpdateTask;
  updating: boolean;
  onUpdate: () => void;
}) {
  const { t } = useTranslation();
  const [working, setWorking] = useState<
    "toggle" | "sync" | "revoke" | "delete"
  >();
  const [error, setError] = useState<string>();
  const deletionPending = node.deletionStatus === "pending";
  const supportsSync = node.capabilities.includes("sync-wakeup-v1");
  const activeUpdate =
    updateTask !== undefined && !isTerminalUpdateTask(updateTask.status);
  const updateDisabledReason = !node.enabled
    ? t("nodes.updates.disabledReason")
    : node.status !== "online"
      ? t("nodes.updates.offlineReason")
      : undefined;

  async function toggleEnabled() {
    setWorking("toggle");
    setError(undefined);
    try {
      onNodeChange(await updateNode(node.id, !node.enabled, csrfToken));
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setWorking(undefined);
    }
  }

  async function toggleSync() {
    setWorking("sync");
    setError(undefined);
    try {
      onNodeChange(
        await (node.syncStatus === undefined
          ? startNodeSyncSession(node.id, csrfToken)
          : stopNodeSyncSession(node.id, csrfToken)),
      );
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setWorking(undefined);
    }
  }

  async function revoke() {
    setWorking("revoke");
    setError(undefined);
    try {
      onNodeChange(await revokeNode(node.id, csrfToken));
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setWorking(undefined);
    }
  }

  async function remove() {
    setWorking("delete");
    setError(undefined);
    try {
      onDeletionQueued(await deleteNode(node.id, csrfToken));
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setWorking(undefined);
    }
  }

  return (
    <div className="flex flex-wrap items-center justify-end gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon-sm" asChild>
            <Link
              to={`/nodes/${node.id}/network`}
              aria-label={t("nodes.actions.network")}
            >
              <NetworkIcon aria-hidden="true" />
            </Link>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("nodes.actions.network")}</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon-sm" asChild>
            <Link
              to={`/nodes/${node.id}/probe`}
              aria-label={t("nodes.actions.probe")}
            >
              <ScanSearch aria-hidden="true" />
            </Link>
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("nodes.actions.probe")}</TooltipContent>
      </Tooltip>

      {updateAvailable && !activeUpdate ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex">
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={updating || updateDisabledReason !== undefined}
                aria-label={t("nodes.updates.updateNode", {
                  name: node.name,
                })}
                onClick={onUpdate}
              >
                {updating ? (
                  <LoaderCircle className="animate-spin" aria-hidden="true" />
                ) : (
                  <CircleArrowUp aria-hidden="true" />
                )}
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {updateDisabledReason ??
              t("nodes.updates.updateNode", { name: node.name })}
          </TooltipContent>
        </Tooltip>
      ) : null}

      {node.status !== "revoked" && node.deletionStatus === undefined ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              disabled={working !== undefined}
              aria-label={
                node.enabled
                  ? t("nodes.actions.disable")
                  : t("nodes.actions.enable")
              }
              onClick={() => void toggleEnabled()}
            >
              {working === "toggle" ? (
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
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex">
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={
                  working !== undefined ||
                  (node.syncStatus === undefined && !supportsSync)
                }
                aria-label={
                  node.syncStatus === undefined
                    ? t("nodes.sync.start")
                    : t("nodes.sync.stop")
                }
                onClick={() => void toggleSync()}
              >
                {working === "sync" ? (
                  <LoaderCircle className="animate-spin" aria-hidden="true" />
                ) : node.syncStatus === undefined ? (
                  <RadioTower aria-hidden="true" />
                ) : (
                  <Unplug aria-hidden="true" />
                )}
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {node.syncStatus === undefined && !supportsSync
              ? t("nodes.sync.unsupported")
              : node.syncStatus === undefined
                ? t("nodes.sync.start")
                : t("nodes.sync.stop")}
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
                  disabled={working !== undefined}
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
                disabled={working !== undefined || deletionPending}
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
    <div className="space-y-2">
      <span title={node.configurationError}>
        {labels[node.configurationStatus]} · {node.appliedConfigurationRevision}
        /{node.desiredConfigurationRevision}
      </span>
      {node.syncStatus !== undefined ? <SyncStatus node={node} /> : null}
    </div>
  );
}

function SyncStatus({ node }: { node: Node }) {
  const { t } = useTranslation();
  const labels = {
    pending: t("nodes.sync.pending"),
    connected: t("nodes.sync.connected"),
    degraded: t("nodes.sync.degraded"),
  };
  if (node.syncStatus === undefined) return null;
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant={node.syncStatus === "degraded" ? "secondary" : "outline"}>
        {node.syncStatus === "degraded" ? (
          <TriangleAlert aria-hidden="true" />
        ) : (
          <RadioTower aria-hidden="true" />
        )}
        {labels[node.syncStatus]}
      </Badge>
      <span className="text-xs text-muted-foreground">
        {t("nodes.sync.until")} <NodeTime value={node.syncExpiresAt} />
      </span>
    </div>
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
