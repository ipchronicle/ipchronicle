import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Check,
  CircleArrowUp,
  Clipboard,
  Globe2,
  KeyRound,
  LoaderCircle,
  Pause,
  Play,
  RadioTower,
  RefreshCw,
  RotateCw,
  ScanSearch,
  Server,
  Search,
  Terminal,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "react-router";

import {
  getAgentEnrollment,
  listNodes,
  rotateAgentEnrollmentKey,
  updateNode,
  updateAgentEnrollment,
  type AgentEnrollmentSettings,
  type Node,
} from "@/api/nodes";
import { createCompleteProbeTask } from "@/api/probes";
import {
  createAgentUpdateTasks,
  getAgentUpdateState,
  type AgentUpdateBatchResult,
  type AgentUpdateState,
  type AgentUpdateTask,
} from "@/api/updates";
import { useAuth } from "@/auth-context";
import { NodeStatusBadge } from "@/components/node-status-badge";
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
import { formatAPIError } from "@/lib/api-error";
import {
  canRequestAgentUpdate,
  isTerminalUpdateTask,
  nodeHasAvailableUpdate,
} from "@/lib/agent-update";

const nodeRefreshIntervalMilliseconds = 3_000;

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
  const nodeMutationRevisionRef = useRef(0);

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

  const hasActiveUpdate =
    state.kind === "success" &&
    state.updates?.tasks.some((task) => !isTerminalUpdateTask(task.status));

  useEffect(() => {
    if (state.kind !== "success") return;

    let disposed = false;
    let inFlight = false;
    let controller: AbortController | undefined;
    const refresh = async () => {
      if (disposed || inFlight || document.visibilityState !== "visible") {
        return;
      }
      inFlight = true;
      const nodeMutationRevision = nodeMutationRevisionRef.current;
      controller = new AbortController();
      try {
        const [nodes, updates] = await Promise.all([
          listNodes(controller.signal),
          hasActiveUpdate
            ? getAgentUpdateState(controller.signal).catch((error: unknown) => {
                if (
                  error instanceof DOMException &&
                  error.name === "AbortError"
                ) {
                  throw error;
                }
                return undefined;
              })
            : Promise.resolve(undefined),
        ]);
        if (disposed) return;
        setState((current) =>
          current.kind === "success"
            ? {
                ...current,
                nodes:
                  nodeMutationRevision === nodeMutationRevisionRef.current
                    ? nodes
                    : current.nodes,
                updates: updates ?? current.updates,
                updateLoadFailed:
                  hasActiveUpdate && updates === undefined
                    ? true
                    : current.updateLoadFailed,
              }
            : current,
        );
      } catch (error) {
        if (!(error instanceof DOMException && error.name === "AbortError")) {
          return;
        }
      } finally {
        inFlight = false;
        controller = undefined;
      }
    };
    const wake = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    const interval = window.setInterval(
      () => void refresh(),
      nodeRefreshIntervalMilliseconds,
    );
    document.addEventListener("visibilitychange", wake);
    window.addEventListener("focus", wake);
    return () => {
      disposed = true;
      controller?.abort();
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", wake);
      window.removeEventListener("focus", wake);
    };
  }, [hasActiveUpdate, state.kind]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  return (
    <div className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
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
              onNodeChange={(node) => {
                nodeMutationRevisionRef.current += 1;
                setState((current) =>
                  current.kind === "success"
                    ? {
                        ...current,
                        nodes: current.nodes.map((item) =>
                          item.id === node.id ? node : item,
                        ),
                      }
                    : current,
                );
              }}
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
  onUpdateTasksCreated,
}: {
  nodes: Node[];
  updates?: AgentUpdateState;
  updateLoadFailed: boolean;
  csrfToken: string;
  onNodeChange: (node: Node) => void;
  onUpdateTasksCreated: (result: AgentUpdateBatchResult) => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
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
        ...node.publicAddresses.map((address) => address.address),
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
  const showUpdateControls = updates?.availableRelease !== undefined;

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
          <CardContent className="space-y-4">
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
              {showUpdateControls ? (
                <>
                  <label className="flex min-h-8 items-center gap-2 text-sm">
                    <Switch
                      size="sm"
                      checked={updatesOnly}
                      onCheckedChange={setUpdatesOnly}
                      aria-label={t("nodes.updates.filter")}
                    />
                    {t("nodes.updates.filter")}
                  </label>
                  <Button
                    onClick={() => void requestUpdates([...selectedNodeIds])}
                    disabled={updating || selectedNodeIds.size === 0}
                  >
                    {updating ? (
                      <LoaderCircle
                        className="animate-spin"
                        data-icon="inline-start"
                        aria-hidden="true"
                      />
                    ) : (
                      <CircleArrowUp
                        data-icon="inline-start"
                        aria-hidden="true"
                      />
                    )}
                    {t("nodes.updates.updateSelected", {
                      count: selectedNodeIds.size,
                    })}
                  </Button>
                </>
              ) : null}
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
            <div className="mx-6 mb-6 hidden overflow-hidden rounded-lg border xl:block">
              <Table>
                <TableHeader className="bg-muted/50">
                  <TableRow>
                    {showUpdateControls ? (
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
                    ) : null}
                    <TableHead className="w-[26%]">
                      {t("nodes.inventory.node")}
                    </TableHead>
                    <TableHead>{t("nodes.inventory.status")}</TableHead>
                    <TableHead>
                      {t("nodes.inventory.publicAddresses")}
                    </TableHead>
                    <TableHead>{t("nodes.inventory.agent")}</TableHead>
                    <TableHead>{t("nodes.inventory.configuration")}</TableHead>
                    <TableHead className="text-right">
                      {t("nodes.inventory.lastSeen")}
                    </TableHead>
                    <TableHead className="w-[22rem] text-right">
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
                      <TableRow
                        key={node.id}
                        className="cursor-pointer"
                        data-state={
                          selectedNodeIds.has(node.id) ? "selected" : undefined
                        }
                        onClick={(event) => {
                          if (!isNodeRowNavigationTarget(event.target)) return;
                          void navigate(`/nodes/${node.id}`);
                        }}
                      >
                        {showUpdateControls ? (
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
                        ) : null}
                        <TableCell>
                          <Link
                            to={`/nodes/${node.id}`}
                            className="inline-block max-w-72 truncate font-medium underline-offset-4 hover:underline"
                          >
                            {node.name}
                          </Link>
                          <p className="mt-1 max-w-72 truncate text-xs text-muted-foreground">
                            {node.hostname}
                          </p>
                        </TableCell>
                        <TableCell>
                          <NodeStatusBadge node={node} />
                        </TableCell>
                        <TableCell>
                          <NodePublicAddresses
                            addresses={node.publicAddresses}
                          />
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
                          <NodeQuickActions
                            node={node}
                            csrfToken={csrfToken}
                            onNodeChange={onNodeChange}
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
            <CardContent className="mx-4 mb-4 divide-y overflow-hidden rounded-lg border p-0 sm:mx-6 sm:mb-6 xl:hidden">
              {visibleNodes.map((node) => {
                const task = tasksByNode.get(node.id);
                const updateAvailable = nodeHasAvailableUpdate(node, updates);
                const canUpdate = canRequestAgentUpdate(node, task, updates);
                return (
                  <div
                    className="cursor-pointer space-y-4 p-4 transition-colors hover:bg-muted/50"
                    key={node.id}
                    onClick={(event) => {
                      if (!isNodeRowNavigationTarget(event.target)) return;
                      void navigate(`/nodes/${node.id}`);
                    }}
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="flex min-w-0 items-start gap-3">
                        {showUpdateControls ? (
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
                        ) : null}
                        <div className="min-w-0">
                          <Link
                            to={`/nodes/${node.id}`}
                            className="block truncate font-medium underline-offset-4 hover:underline"
                          >
                            {node.name}
                          </Link>
                          <p className="mt-1 truncate text-xs text-muted-foreground">
                            {node.hostname}
                          </p>
                        </div>
                      </div>
                      <NodeStatusBadge node={node} />
                    </div>
                    <dl className="grid grid-cols-2 gap-3 text-xs">
                      <div className="col-span-2">
                        <dt className="text-muted-foreground">
                          {t("nodes.inventory.publicAddresses")}
                        </dt>
                        <dd className="mt-2">
                          <NodePublicAddresses
                            addresses={node.publicAddresses}
                          />
                        </dd>
                      </div>
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
                    <NodeQuickActions
                      node={node}
                      csrfToken={csrfToken}
                      onNodeChange={onNodeChange}
                      updateAvailable={updateAvailable}
                      updateTask={task}
                      updating={updating}
                      onUpdate={() => void requestUpdates([node.id])}
                      stacked
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

function NodePublicAddresses({
  addresses,
}: {
  addresses: Node["publicAddresses"];
}) {
  const { t } = useTranslation();
  if (addresses.length === 0) {
    return (
      <span className="text-xs text-muted-foreground">
        {t("nodes.inventory.noPublicAddresses")}
      </span>
    );
  }
  return (
    <div className="space-y-2">
      {addresses.map((address) => (
        <div key={address.id} className="flex flex-wrap items-center gap-1.5">
          <Globe2
            aria-hidden="true"
            className="size-3.5 text-muted-foreground"
          />
          <span className="break-all font-mono text-xs">{address.address}</span>
          <Badge variant="secondary">{address.family.toUpperCase()}</Badge>
          <Badge
            variant={address.probeEnabled ? "outline" : "secondary"}
            className={
              address.probeEnabled
                ? "border-emerald-600/40 text-emerald-700 dark:text-emerald-400"
                : undefined
            }
          >
            {address.probeEnabled
              ? t("nodes.inventory.probeEnabled")
              : t("nodes.inventory.probeDisabled")}
          </Badge>
          {!address.available ? (
            <Badge variant="outline">
              {t("nodes.inventory.addressUnavailable")}
            </Badge>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function isNodeRowNavigationTarget(target: EventTarget | null) {
  return !(
    target instanceof Element &&
    target.closest("a, button, input, [role='checkbox'], [role='group']")
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

function NodeQuickActions({
  node,
  csrfToken,
  onNodeChange,
  updateAvailable,
  updateTask,
  updating,
  onUpdate,
  stacked = false,
}: {
  node: Node;
  csrfToken: string;
  onNodeChange: (node: Node) => void;
  updateAvailable: boolean;
  updateTask?: AgentUpdateTask;
  updating: boolean;
  onUpdate: () => void;
  stacked?: boolean;
}) {
  const { t } = useTranslation();
  const [working, setWorking] = useState<"toggle" | "probe">();
  const [feedback, setFeedback] = useState<
    { kind: "success" | "error"; message: string } | undefined
  >();
  const activeUpdate =
    updateTask !== undefined && !isTerminalUpdateTask(updateTask.status);
  const updateDisabledReason = !node.enabled
    ? t("nodes.updates.disabledReason")
    : node.status !== "online"
      ? t("nodes.updates.offlineReason")
      : undefined;
  const buttonClassName = stacked
    ? "h-auto min-h-8 w-full min-w-0 justify-start whitespace-normal py-1.5 text-left"
    : undefined;
  const probeUnavailable =
    !node.enabled ||
    node.status !== "online" ||
    node.deletionStatus !== undefined ||
    !node.capabilities.includes("complete-probe-v1");

  async function toggleEnabled() {
    setWorking("toggle");
    setFeedback(undefined);
    try {
      onNodeChange(await updateNode(node.id, !node.enabled, csrfToken));
    } catch (cause) {
      setFeedback({ kind: "error", message: formatAPIError(cause, t) });
    } finally {
      setWorking(undefined);
    }
  }

  async function runProbe() {
    setWorking("probe");
    setFeedback(undefined);
    try {
      await createCompleteProbeTask(node.id, csrfToken);
      setFeedback({ kind: "success", message: t("probe.task.created") });
    } catch (cause) {
      setFeedback({ kind: "error", message: formatAPIError(cause, t) });
    } finally {
      setWorking(undefined);
    }
  }

  return (
    <div
      className={
        stacked
          ? "grid grid-cols-1 gap-2 border-t pt-4 sm:grid-cols-2"
          : "flex flex-wrap items-center justify-end gap-1.5"
      }
      role="group"
      aria-label={t("nodes.actions.group", { name: node.name })}
    >
      <Button
        variant="outline"
        size="sm"
        className={buttonClassName}
        disabled={working !== undefined || probeUnavailable}
        onClick={() => void runProbe()}
      >
        {working === "probe" ? (
          <LoaderCircle className="animate-spin" aria-hidden="true" />
        ) : (
          <ScanSearch aria-hidden="true" />
        )}
        {t("nodes.quickActions.runProbe")}
      </Button>

      {updateAvailable && !activeUpdate ? (
        <Button
          variant="outline"
          size="sm"
          className={buttonClassName}
          disabled={updating || updateDisabledReason !== undefined}
          title={updateDisabledReason}
          onClick={onUpdate}
        >
          {updating ? (
            <LoaderCircle className="animate-spin" aria-hidden="true" />
          ) : (
            <CircleArrowUp aria-hidden="true" />
          )}
          {t("nodes.updates.updateAction")}
        </Button>
      ) : null}

      {node.status !== "revoked" && node.deletionStatus === undefined ? (
        <Button
          variant="outline"
          size="sm"
          className={buttonClassName}
          disabled={working !== undefined}
          onClick={() => void toggleEnabled()}
        >
          {working === "toggle" ? (
            <LoaderCircle className="animate-spin" aria-hidden="true" />
          ) : node.enabled ? (
            <Pause aria-hidden="true" />
          ) : (
            <Play aria-hidden="true" />
          )}
          {node.enabled
            ? t("nodes.actions.disable")
            : t("nodes.actions.enable")}
        </Button>
      ) : null}

      {feedback ? (
        <p
          className={`text-xs ${stacked ? "sm:col-span-2" : "w-full text-right"} ${
            feedback.kind === "error"
              ? "text-destructive"
              : "text-muted-foreground"
          }`}
          role={feedback.kind === "error" ? "alert" : "status"}
        >
          {feedback.message}
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
