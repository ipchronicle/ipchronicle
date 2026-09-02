import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  BellRing,
  Bot,
  Code2,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCw,
  Save,
  Send,
  Trash2,
  TriangleAlert,
  Webhook,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import { getNodeNetwork, type PublicAddress } from "@/api/network";
import { listNodes, type Node } from "@/api/nodes";
import {
  createNotificationRule,
  createNotificationSender,
  createNotificationTestDelivery,
  deleteNotificationRule,
  deleteNotificationSender,
  listNotificationDeliveries,
  listNotificationProbeFields,
  listNotificationRules,
  listNotificationSenders,
  updateNotificationRule,
  updateNotificationSender,
  type NotificationDeliveryStatus,
  type NotificationEventType,
  type NotificationProbeField,
  type NotificationRule,
  type NotificationRuleWrite,
  type NotificationSender,
  type NotificationSenderCreate,
  type NotificationSenderUpdate,
} from "@/api/notifications";
import { useAuth } from "@/auth-context";
import {
  allProbeFieldsValue,
  ProbeFieldCombobox,
} from "@/components/probe-field-combobox";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { formatAPIError } from "@/lib/api-error";
import { presentProbeField } from "@/lib/probe-field-label";

const allValue = "__all__";
const notificationEventTypes: NotificationEventType[] = [
  "all",
  "probe-field-change",
  "address-change",
  "address-check-failure",
  "address-check-recovery",
  "probe-failure",
  "probe-recovery",
  "address-gap",
  "probe-gap",
  "format-mismatch",
  "format-changed",
  "format-recovery",
];

type ViewState =
  | { kind: "loading" }
  | {
      kind: "success";
      senders: NotificationSender[];
      rules: NotificationRule[];
      nodes: Node[];
      probeFields: NotificationProbeField[];
    }
  | { kind: "error" };

type Feedback = { kind: "success" | "error"; text: string };

export function NotificationsPage() {
  const { i18n, t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [tab, setTab] = useState("senders");
  const [senderEditor, setSenderEditor] = useState<
    NotificationSender | "create"
  >();
  const [ruleEditor, setRuleEditor] = useState<NotificationRule | "create">();
  const [feedback, setFeedback] = useState<Feedback>();
  const [refreshing, setRefreshing] = useState(false);
  const [deliveryRevision, setDeliveryRevision] = useState(0);

  const load = useCallback(async (signal?: AbortSignal, initial = false) => {
    if (initial) setState({ kind: "loading" });
    else setRefreshing(true);
    try {
      const [senders, rules, nodes, probeFields] = await Promise.all([
        listNotificationSenders(signal),
        listNotificationRules(signal),
        listNodes(signal),
        listNotificationProbeFields(signal),
      ]);
      setState({ kind: "success", senders, rules, nodes, probeFields });
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setState({ kind: "error" });
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal, true);
    return () => controller.abort();
  }, [load]);

  useEffect(() => {
    setFeedback(undefined);
  }, [i18n.resolvedLanguage]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function runAction<T>(action: () => Promise<T>) {
    setFeedback(undefined);
    try {
      return await action();
    } catch (error) {
      setFeedback({ kind: "error", text: formatAPIError(error, t) });
      return undefined;
    }
  }

  async function runVoidAction(action: () => Promise<void>) {
    setFeedback(undefined);
    try {
      await action();
      return true;
    } catch (error) {
      setFeedback({ kind: "error", text: formatAPIError(error, t) });
      return false;
    }
  }

  async function createSender(input: NotificationSenderCreate) {
    const sender = await runAction(() =>
      createNotificationSender(input, csrfToken),
    );
    if (!sender) return false;
    setState((current) =>
      current.kind === "success"
        ? { ...current, senders: [...current.senders, sender] }
        : current,
    );
    setSenderEditor(undefined);
    setFeedback({ kind: "success", text: t("notifications.feedback.saved") });
    return true;
  }

  async function updateSender(
    senderId: string,
    input: NotificationSenderUpdate,
  ) {
    const sender = await runAction(() =>
      updateNotificationSender(senderId, input, csrfToken),
    );
    if (!sender) return false;
    setState((current) =>
      current.kind === "success"
        ? {
            ...current,
            senders: current.senders.map((item) =>
              item.id === sender.id ? sender : item,
            ),
          }
        : current,
    );
    setSenderEditor(undefined);
    setFeedback({ kind: "success", text: t("notifications.feedback.saved") });
    return true;
  }

  async function removeSender(senderId: string) {
    const succeeded = await runVoidAction(() =>
      deleteNotificationSender(senderId, csrfToken),
    );
    if (!succeeded) return;
    setState((current) =>
      current.kind === "success"
        ? {
            ...current,
            senders: current.senders.filter((item) => item.id !== senderId),
          }
        : current,
    );
    setFeedback({ kind: "success", text: t("notifications.feedback.deleted") });
  }

  async function testSender(senderId: string) {
    const delivery = await runAction(() =>
      createNotificationTestDelivery(senderId, csrfToken),
    );
    if (!delivery) return;
    setFeedback({
      kind: "success",
      text: t("notifications.feedback.testQueued"),
    });
    setTab("deliveries");
    setDeliveryRevision((value) => value + 1);
  }

  async function createRule(input: NotificationRuleWrite) {
    const rule = await runAction(() =>
      createNotificationRule(input, csrfToken),
    );
    if (!rule) return false;
    setState((current) =>
      current.kind === "success"
        ? { ...current, rules: [...current.rules, rule] }
        : current,
    );
    setRuleEditor(undefined);
    setFeedback({ kind: "success", text: t("notifications.feedback.saved") });
    return true;
  }

  async function updateRule(ruleId: string, input: NotificationRuleWrite) {
    const rule = await runAction(() =>
      updateNotificationRule(ruleId, input, csrfToken),
    );
    if (!rule) return false;
    setState((current) =>
      current.kind === "success"
        ? {
            ...current,
            rules: current.rules.map((item) =>
              item.id === rule.id ? rule : item,
            ),
          }
        : current,
    );
    setRuleEditor(undefined);
    setFeedback({ kind: "success", text: t("notifications.feedback.saved") });
    return true;
  }

  async function removeRule(ruleId: string) {
    const succeeded = await runVoidAction(() =>
      deleteNotificationRule(ruleId, csrfToken),
    );
    if (!succeeded) return;
    setState((current) =>
      current.kind === "success"
        ? {
            ...current,
            rules: current.rules.filter((item) => item.id !== ruleId),
          }
        : current,
    );
    setFeedback({ kind: "success", text: t("notifications.feedback.deleted") });
  }

  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          <p className="text-sm font-medium text-muted-foreground uppercase">
            {t("notifications.section")}
          </p>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
            {t("notifications.title")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("notifications.detail")}
          </p>
        </div>
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
          {t("notifications.refresh")}
        </Button>
      </div>

      <div className="mt-8 space-y-4" aria-live="polite">
        {feedback ? (
          <Alert
            variant={feedback.kind === "error" ? "destructive" : "default"}
          >
            {feedback.kind === "error" ? (
              <TriangleAlert aria-hidden="true" />
            ) : null}
            <AlertDescription>{feedback.text}</AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "loading" ? <NotificationSkeleton /> : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("notifications.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => void load(undefined, true)}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("notifications.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList aria-label={t("notifications.tabs.label")}>
              <TabsTrigger value="senders">
                {t("notifications.tabs.senders")}
              </TabsTrigger>
              <TabsTrigger value="rules">
                {t("notifications.tabs.rules")}
              </TabsTrigger>
              <TabsTrigger value="deliveries">
                {t("notifications.tabs.deliveries")}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="senders" className="space-y-4">
              <SectionActionCard
                title={t("notifications.senders.title")}
                detail={t("notifications.senders.detail")}
                action={t("notifications.senders.add")}
                onAction={() => setSenderEditor("create")}
              />
              {senderEditor ? (
                <SenderForm
                  key={senderEditor === "create" ? "create" : senderEditor.id}
                  sender={senderEditor === "create" ? undefined : senderEditor}
                  onCancel={() => setSenderEditor(undefined)}
                  onCreate={createSender}
                  onUpdate={updateSender}
                />
              ) : null}
              {state.senders.length === 0 ? (
                <EmptyCard text={t("notifications.senders.empty")} />
              ) : (
                state.senders.map((sender) => (
                  <SenderCard
                    key={sender.id}
                    sender={sender}
                    onEdit={() => setSenderEditor(sender)}
                    onTest={() => void testSender(sender.id)}
                    onDelete={() => void removeSender(sender.id)}
                  />
                ))
              )}
            </TabsContent>
            <TabsContent value="rules" className="space-y-4">
              <SectionActionCard
                title={t("notifications.rules.title")}
                detail={t("notifications.rules.detail")}
                action={t("notifications.rules.add")}
                onAction={() => setRuleEditor("create")}
              />
              {ruleEditor ? (
                <RuleForm
                  key={ruleEditor === "create" ? "create" : ruleEditor.id}
                  rule={ruleEditor === "create" ? undefined : ruleEditor}
                  senders={state.senders}
                  nodes={state.nodes}
                  probeFields={state.probeFields}
                  onCancel={() => setRuleEditor(undefined)}
                  onCreate={createRule}
                  onUpdate={updateRule}
                />
              ) : null}
              {state.rules.length === 0 ? (
                <EmptyCard text={t("notifications.rules.empty")} />
              ) : (
                state.rules.map((rule) => (
                  <RuleCard
                    key={rule.id}
                    rule={rule}
                    senders={state.senders}
                    nodes={state.nodes}
                    probeFields={state.probeFields}
                    onEdit={() => setRuleEditor(rule)}
                    onDelete={() => void removeRule(rule.id)}
                  />
                ))
              )}
            </TabsContent>
            <TabsContent value="deliveries">
              <DeliveryHistory
                senders={state.senders}
                active={tab === "deliveries"}
                revision={deliveryRevision}
              />
            </TabsContent>
          </Tabs>
        ) : null}
      </div>
    </main>
  );
}

function SectionActionCard({
  title,
  detail,
  action,
  onAction,
}: {
  title: string;
  detail: string;
  action: string;
  onAction: () => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{detail}</CardDescription>
        <CardAction>
          <Button onClick={onAction}>
            <Plus data-icon="inline-start" aria-hidden="true" />
            {action}
          </Button>
        </CardAction>
      </CardHeader>
    </Card>
  );
}

function SenderCard({
  sender,
  onEdit,
  onTest,
  onDelete,
}: {
  sender: NotificationSender;
  onEdit: () => void;
  onTest: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const Icon = senderIcon(sender.kind);
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Icon aria-hidden="true" className="size-4" />
          {sender.name}
        </CardTitle>
        <CardDescription>{senderDescription(sender, t)}</CardDescription>
        <CardAction className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={onTest}
            disabled={!sender.enabled}
          >
            <Send data-icon="inline-start" aria-hidden="true" />
            {t("notifications.senders.test")}
          </Button>
          <Button variant="outline" size="sm" onClick={onEdit}>
            <Pencil data-icon="inline-start" aria-hidden="true" />
            {t("notifications.edit")}
          </Button>
          <DeleteAction
            title={t("notifications.senders.deleteTitle")}
            detail={t("notifications.senders.deleteDetail")}
            confirm={t("notifications.delete")}
            onConfirm={onDelete}
          />
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-2">
        <Badge variant="outline">
          {t(`notifications.senderKind.${sender.kind}`)}
        </Badge>
        <Badge variant={sender.enabled ? "default" : "secondary"}>
          {sender.enabled
            ? t("notifications.enabled")
            : t("notifications.disabled")}
        </Badge>
      </CardContent>
    </Card>
  );
}

function SenderForm({
  sender,
  onCancel,
  onCreate,
  onUpdate,
}: {
  sender?: NotificationSender;
  onCancel: () => void;
  onCreate: (input: NotificationSenderCreate) => Promise<boolean>;
  onUpdate: (
    senderId: string,
    input: NotificationSenderUpdate,
  ) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(sender?.name ?? "");
  const [kind, setKind] = useState<NotificationSender["kind"]>(
    sender?.kind ?? "telegram",
  );
  const [enabled, setEnabled] = useState(sender?.enabled ?? true);
  const [chatId, setChatId] = useState(sender?.telegram?.chatId ?? "");
  const [topicId, setTopicId] = useState(
    sender?.telegram?.topicId?.toString() ?? "",
  );
  const [token, setToken] = useState("");
  const [url, setURL] = useState(sender?.webhook?.url ?? "");
  const [headers, setHeaders] = useState("");
  const [replaceHeaders, setReplaceHeaders] = useState(!sender);
  const [source, setSource] = useState(sender?.javascript?.source ?? "");
  const [working, setWorking] = useState(false);
  const [invalid, setInvalid] = useState<string>();

  async function submit(event: FormEvent) {
    event.preventDefault();
    setInvalid(undefined);
    const parsedHeaders = parseHeaders(headers);
    if (kind === "webhook" && replaceHeaders && parsedHeaders === undefined) {
      setInvalid(t("notifications.senders.invalidHeaders"));
      return;
    }
    const parsedTopicId = topicId === "" ? undefined : Number(topicId);
    if (
      kind === "telegram" &&
      parsedTopicId !== undefined &&
      (!Number.isSafeInteger(parsedTopicId) || parsedTopicId <= 0)
    ) {
      setInvalid(t("notifications.senders.invalidTopicId"));
      return;
    }
    setWorking(true);
    try {
      if (!sender) {
        const input: NotificationSenderCreate = { name, kind, enabled };
        if (kind === "telegram")
          input.telegram = {
            chatId,
            token,
            ...(parsedTopicId ? { topicId: parsedTopicId } : {}),
          };
        if (kind === "webhook")
          input.webhook = { url, headers: parsedHeaders ?? {} };
        if (kind === "javascript") input.javascript = { source };
        await onCreate(input);
        return;
      }
      const input: NotificationSenderUpdate = { name, enabled };
      if (kind === "telegram") {
        input.telegram = {
          chatId,
          ...(token ? { token } : {}),
          ...(parsedTopicId ? { topicId: parsedTopicId } : {}),
        };
      }
      if (kind === "webhook") {
        input.webhook = {
          url,
          ...(replaceHeaders ? { headers: parsedHeaders ?? {} } : {}),
        };
      }
      if (kind === "javascript") input.javascript = { source };
      await onUpdate(sender.id, input);
    } finally {
      setWorking(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {sender
            ? t("notifications.senders.edit")
            : t("notifications.senders.create")}
        </CardTitle>
        <CardDescription>
          {t("notifications.senders.formDetail")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form className="space-y-5" onSubmit={submit}>
          {invalid ? (
            <Alert variant="destructive">
              <TriangleAlert aria-hidden="true" />
              <AlertDescription>{invalid}</AlertDescription>
            </Alert>
          ) : null}
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="sender-name">{t("notifications.name")}</Label>
              <Input
                id="sender-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
                maxLength={128}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="sender-kind">
                {t("notifications.senders.kind")}
              </Label>
              {sender ? (
                <div className="flex h-8 items-center">
                  <Badge variant="outline">
                    {t(`notifications.senderKind.${kind}`)}
                  </Badge>
                </div>
              ) : (
                <Select
                  value={kind}
                  onValueChange={(value) =>
                    setKind(value as NotificationSender["kind"])
                  }
                >
                  <SelectTrigger id="sender-kind" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="telegram">
                      {t("notifications.senderKind.telegram")}
                    </SelectItem>
                    <SelectItem value="webhook">
                      {t("notifications.senderKind.webhook")}
                    </SelectItem>
                    <SelectItem value="javascript">
                      {t("notifications.senderKind.javascript")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              )}
            </div>
          </div>
          {kind === "telegram" ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="sender-chat-id">
                  {t("notifications.senders.chatId")}
                </Label>
                <Input
                  id="sender-chat-id"
                  value={chatId}
                  onChange={(event) => setChatId(event.target.value)}
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="sender-topic-id">
                  {t("notifications.senders.topicId")}
                </Label>
                <Input
                  id="sender-topic-id"
                  type="number"
                  min={1}
                  max={Number.MAX_SAFE_INTEGER}
                  step={1}
                  value={topicId}
                  onChange={(event) => setTopicId(event.target.value)}
                  placeholder={t("notifications.senders.topicIdPlaceholder")}
                />
                <p className="text-sm text-muted-foreground">
                  {t("notifications.senders.topicIdDetail")}
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="sender-token">
                  {t("notifications.senders.token")}
                </Label>
                <Input
                  id="sender-token"
                  type="password"
                  value={token}
                  onChange={(event) => setToken(event.target.value)}
                  required={!sender}
                  autoComplete="off"
                />
                {sender ? (
                  <p className="text-sm text-muted-foreground">
                    {t("notifications.senders.keepToken")}
                  </p>
                ) : null}
              </div>
            </div>
          ) : null}
          {kind === "webhook" ? (
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="sender-url">
                  {t("notifications.senders.url")}
                </Label>
                <Input
                  id="sender-url"
                  type="url"
                  value={url}
                  onChange={(event) => setURL(event.target.value)}
                  required
                />
              </div>
              {sender ? (
                <div className="flex items-center justify-between gap-4 rounded-md border p-3">
                  <div>
                    <p
                      id="replace-headers-label"
                      className="text-sm leading-none font-medium"
                    >
                      {t("notifications.senders.replaceHeaders")}
                    </p>
                    <p className="mt-1 text-sm text-muted-foreground">
                      {t("notifications.senders.configuredHeaders", {
                        value:
                          sender.webhook?.headerNames.join(", ") ||
                          t("notifications.none"),
                      })}
                    </p>
                  </div>
                  <Switch
                    id="replace-headers"
                    aria-labelledby="replace-headers-label"
                    checked={replaceHeaders}
                    onCheckedChange={setReplaceHeaders}
                  />
                </div>
              ) : null}
              {replaceHeaders ? (
                <div className="space-y-2">
                  <Label htmlFor="sender-headers">
                    {t("notifications.senders.headers")}
                  </Label>
                  <Textarea
                    id="sender-headers"
                    value={headers}
                    onChange={(event) => setHeaders(event.target.value)}
                    rows={4}
                    placeholder={t("notifications.senders.headersPlaceholder")}
                  />
                </div>
              ) : null}
            </div>
          ) : null}
          {kind === "javascript" ? (
            <div className="space-y-2">
              <Label htmlFor="sender-source">
                {t("notifications.senders.source")}
              </Label>
              <Textarea
                id="sender-source"
                className="min-h-64 font-mono text-sm"
                value={source}
                onChange={(event) => setSource(event.target.value)}
                required
                spellCheck={false}
              />
              <p className="text-sm text-muted-foreground">
                {t("notifications.senders.sourceDetail")}
              </p>
            </div>
          ) : null}
          <div className="flex items-center justify-between gap-4 rounded-md border p-3">
            <span
              id="sender-enabled-label"
              className="text-sm leading-none font-medium"
            >
              {t("notifications.enabled")}
            </span>
            <Switch
              id="sender-enabled"
              aria-labelledby="sender-enabled-label"
              checked={enabled}
              onCheckedChange={setEnabled}
            />
          </div>
          <div className="flex justify-end gap-2 border-t pt-4">
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={working}>
              {working ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <Save data-icon="inline-start" aria-hidden="true" />
              )}
              {t("notifications.save")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function RuleCard({
  rule,
  senders,
  nodes,
  probeFields,
  onEdit,
  onDelete,
}: {
  rule: NotificationRule;
  senders: NotificationSender[];
  nodes: Node[];
  probeFields: NotificationProbeField[];
  onEdit: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const sender = senders.find((item) => item.id === rule.senderId);
  const node = nodes.find((item) => item.id === rule.nodeId);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{rule.name}</CardTitle>
        <CardDescription>
          {t(`notifications.eventType.${rule.eventType}`)} ·{" "}
          {sender?.name ?? rule.senderId}
        </CardDescription>
        <CardAction className="flex gap-2">
          <Button variant="outline" size="sm" onClick={onEdit}>
            <Pencil data-icon="inline-start" aria-hidden="true" />
            {t("notifications.edit")}
          </Button>
          <DeleteAction
            title={t("notifications.rules.deleteTitle")}
            detail={t("notifications.rules.deleteDetail")}
            confirm={t("notifications.delete")}
            onConfirm={onDelete}
          />
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-2">
        <Badge variant={rule.enabled ? "default" : "secondary"}>
          {rule.enabled
            ? t("notifications.enabled")
            : t("notifications.disabled")}
        </Badge>
        {rule.fieldId ? (
          <Badge variant="outline">
            {notificationProbeFieldLabel(rule.fieldId, probeFields, t)}
          </Badge>
        ) : null}
        <Badge variant="outline">
          {node?.name ?? t("notifications.rules.allNodes")}
        </Badge>
        {rule.egressId ? (
          <Badge variant="outline">
            {rule.publicAddress ?? t("notifications.rules.addressUnavailable")}
          </Badge>
        ) : null}
      </CardContent>
    </Card>
  );
}

function RuleForm({
  rule,
  senders,
  nodes,
  probeFields,
  onCancel,
  onCreate,
  onUpdate,
}: {
  rule?: NotificationRule;
  senders: NotificationSender[];
  nodes: Node[];
  probeFields: NotificationProbeField[];
  onCancel: () => void;
  onCreate: (input: NotificationRuleWrite) => Promise<boolean>;
  onUpdate: (ruleId: string, input: NotificationRuleWrite) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(rule?.name ?? "");
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [senderId, setSenderId] = useState(
    rule?.senderId ?? senders[0]?.id ?? "",
  );
  const [eventType, setEventType] = useState<NotificationEventType>(
    rule?.eventType ?? "address-change",
  );
  const [fieldId, setFieldId] = useState(rule?.fieldId ?? allProbeFieldsValue);
  const [nodeId, setNodeId] = useState(rule?.nodeId ?? allValue);
  const [egressId, setEgressId] = useState(rule?.egressId ?? allValue);
  const [publicAddresses, setPublicAddresses] = useState<PublicAddress[]>([]);
  const [loadingEgresses, setLoadingEgresses] = useState(false);
  const [working, setWorking] = useState(false);

  useEffect(() => {
    if (nodeId === allValue) {
      setPublicAddresses([]);
      setEgressId(allValue);
      return;
    }
    const controller = new AbortController();
    setLoadingEgresses(true);
    void getNodeNetwork(nodeId, controller.signal)
      .then((network) => setPublicAddresses(network.publicAddresses))
      .catch((error) => {
        if (!(error instanceof DOMException && error.name === "AbortError"))
          setPublicAddresses([]);
      })
      .finally(() => setLoadingEgresses(false));
    return () => controller.abort();
  }, [nodeId]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const input: NotificationRuleWrite = {
      name,
      enabled,
      senderId,
      eventType,
      ...(eventType === "probe-field-change" && fieldId !== allProbeFieldsValue
        ? { fieldId }
        : {}),
      ...(nodeId !== allValue ? { nodeId } : {}),
      ...(egressId !== allValue ? { egressId } : {}),
    };
    setWorking(true);
    try {
      if (rule) await onUpdate(rule.id, input);
      else await onCreate(input);
    } finally {
      setWorking(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {rule
            ? t("notifications.rules.edit")
            : t("notifications.rules.create")}
        </CardTitle>
        <CardDescription>{t("notifications.rules.formDetail")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="space-y-5" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="rule-name">{t("notifications.name")}</Label>
              <Input
                id="rule-name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
                maxLength={128}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-sender">
                {t("notifications.rules.sender")}
              </Label>
              <Select value={senderId} onValueChange={setSenderId} required>
                <SelectTrigger id="rule-sender" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {senders.map((sender) => (
                    <SelectItem key={sender.id} value={sender.id}>
                      {sender.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-event">
                {t("notifications.rules.event")}
              </Label>
              <Select
                value={eventType}
                onValueChange={(value) =>
                  setEventType(value as NotificationEventType)
                }
              >
                <SelectTrigger id="rule-event" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {notificationEventTypes.map((type) => (
                    <SelectItem key={type} value={type}>
                      {t(`notifications.eventType.${type}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {eventType === "probe-field-change" ? (
              <div className="space-y-2">
                <Label htmlFor="rule-field">
                  {t("notifications.rules.field")}
                </Label>
                <ProbeFieldCombobox
                  id="rule-field"
                  fields={probeFields}
                  value={fieldId}
                  onValueChange={setFieldId}
                />
              </div>
            ) : null}
            <div className="space-y-2">
              <Label htmlFor="rule-node">{t("notifications.rules.node")}</Label>
              <Select value={nodeId} onValueChange={setNodeId}>
                <SelectTrigger id="rule-node" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={allValue}>
                    {t("notifications.rules.allNodes")}
                  </SelectItem>
                  {nodes.map((node) => (
                    <SelectItem key={node.id} value={node.id}>
                      {node.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="rule-egress">
                {t("notifications.rules.egress")}
              </Label>
              <Select
                value={egressId}
                onValueChange={setEgressId}
                disabled={nodeId === allValue || loadingEgresses}
              >
                <SelectTrigger id="rule-egress" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={allValue}>
                    {t("notifications.rules.allEgresses")}
                  </SelectItem>
                  {publicAddresses.map((address) => (
                    <SelectItem key={address.id} value={address.id}>
                      {address.address}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          {senders.length === 0 ? (
            <Alert>
              <BellRing aria-hidden="true" />
              <AlertDescription>
                {t("notifications.rules.senderRequired")}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="flex items-center justify-between gap-4 rounded-md border p-3">
            <span
              id="rule-enabled-label"
              className="text-sm leading-none font-medium"
            >
              {t("notifications.enabled")}
            </span>
            <Switch
              id="rule-enabled"
              aria-labelledby="rule-enabled-label"
              checked={enabled}
              onCheckedChange={setEnabled}
            />
          </div>
          <div className="flex justify-end gap-2 border-t pt-4">
            <Button type="button" variant="outline" onClick={onCancel}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={working || !senderId}>
              {working ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <Save data-icon="inline-start" aria-hidden="true" />
              )}
              {t("notifications.save")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function DeliveryHistory({
  senders,
  active,
  revision,
}: {
  senders: NotificationSender[];
  active: boolean;
  revision: number;
}) {
  const { i18n, t } = useTranslation();
  const [senderId, setSenderId] = useState(allValue);
  const [status, setStatus] = useState<
    NotificationDeliveryStatus | typeof allValue
  >(allValue);
  const [page, setPage] = useState(1);
  const [refresh, setRefresh] = useState(0);
  const [state, setState] = useState<
    | { kind: "loading" }
    | { kind: "error" }
    | {
        kind: "success";
        data: Awaited<ReturnType<typeof listNotificationDeliveries>>;
      }
  >({ kind: "loading" });

  useEffect(() => {
    if (!active) return;
    const controller = new AbortController();
    setState({ kind: "loading" });
    void listNotificationDeliveries(
      {
        ...(senderId !== allValue ? { senderId } : {}),
        ...(status !== allValue ? { status } : {}),
        page,
        pageSize: 25,
      },
      controller.signal,
    )
      .then((data) => setState({ kind: "success", data }))
      .catch((error) => {
        if (!(error instanceof DOMException && error.name === "AbortError"))
          setState({ kind: "error" });
      });
    return () => controller.abort();
  }, [active, page, refresh, revision, senderId, status]);

  const totalPages = state.kind === "success" ? state.data.totalPages : 0;
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("notifications.deliveries.title")}</CardTitle>
          <CardDescription>
            {t("notifications.deliveries.detail")}
          </CardDescription>
          <CardAction>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setRefresh((value) => value + 1)}
            >
              <RefreshCw data-icon="inline-start" aria-hidden="true" />
              {t("notifications.refresh")}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="delivery-sender">
              {t("notifications.deliveries.sender")}
            </Label>
            <Select
              value={senderId}
              onValueChange={(value) => {
                setSenderId(value);
                setPage(1);
              }}
            >
              <SelectTrigger id="delivery-sender" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={allValue}>
                  {t("notifications.deliveries.allSenders")}
                </SelectItem>
                {senders.map((sender) => (
                  <SelectItem key={sender.id} value={sender.id}>
                    {sender.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="delivery-status">
              {t("notifications.deliveries.status")}
            </Label>
            <Select
              value={status}
              onValueChange={(value) => {
                setStatus(
                  value as NotificationDeliveryStatus | typeof allValue,
                );
                setPage(1);
              }}
            >
              <SelectTrigger id="delivery-status" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={allValue}>
                  {t("notifications.deliveries.allStatuses")}
                </SelectItem>
                {(
                  [
                    "pending",
                    "running",
                    "retrying",
                    "succeeded",
                    "failed",
                  ] as NotificationDeliveryStatus[]
                ).map((value) => (
                  <SelectItem key={value} value={value}>
                    {t(`notifications.deliveryStatus.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </CardContent>
      </Card>
      {state.kind === "loading" ? <NotificationSkeleton /> : null}
      {state.kind === "error" ? (
        <Alert variant="destructive">
          <TriangleAlert aria-hidden="true" />
          <AlertTitle>{t("notifications.deliveries.loadFailed")}</AlertTitle>
        </Alert>
      ) : null}
      {state.kind === "success" ? (
        <Card>
          <CardContent className="overflow-x-auto px-0">
            {state.data.items.length === 0 ? (
              <p className="px-6 py-10 text-center text-sm text-muted-foreground">
                {t("notifications.deliveries.empty")}
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="pl-6">
                      {t("notifications.deliveries.sender")}
                    </TableHead>
                    <TableHead>{t("notifications.deliveries.event")}</TableHead>
                    <TableHead>
                      {t("notifications.deliveries.status")}
                    </TableHead>
                    <TableHead>
                      {t("notifications.deliveries.attempts")}
                    </TableHead>
                    <TableHead>
                      {t("notifications.deliveries.created")}
                    </TableHead>
                    <TableHead className="pr-6">
                      {t("notifications.deliveries.error")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {state.data.items.map((delivery) => (
                    <TableRow key={delivery.id}>
                      <TableCell className="pl-6 font-medium">
                        {delivery.senderName}
                      </TableCell>
                      <TableCell>
                        {delivery.test
                          ? t("notifications.deliveries.test")
                          : eventTypeLabel(delivery.eventType, t)}
                      </TableCell>
                      <TableCell>
                        <DeliveryStatusBadge status={delivery.status} />
                      </TableCell>
                      <TableCell>{delivery.attemptCount}/4</TableCell>
                      <TableCell>
                        {new Date(delivery.createdAt).toLocaleString(
                          i18n.language,
                        )}
                      </TableCell>
                      <TableCell className="max-w-64 truncate pr-6 text-muted-foreground">
                        {delivery.errorCode ?? t("notifications.none")}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      ) : null}
      {state.kind === "success" && totalPages > 1 ? (
        <Card>
          <CardContent className="flex items-center justify-between gap-3">
            <span className="text-sm text-muted-foreground">
              {t("notifications.deliveries.page", {
                page: state.data.page,
                total: totalPages,
              })}
            </span>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={page <= 1}
                onClick={() => setPage((value) => value - 1)}
              >
                {t("notifications.deliveries.previous")}
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= totalPages}
                onClick={() => setPage((value) => value + 1)}
              >
                {t("notifications.deliveries.next")}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

function DeliveryStatusBadge({
  status,
}: {
  status: NotificationDeliveryStatus;
}) {
  const { t } = useTranslation();
  const variant =
    status === "failed"
      ? "destructive"
      : status === "succeeded"
        ? "default"
        : "secondary";
  return (
    <Badge variant={variant}>
      {t(`notifications.deliveryStatus.${status}`)}
    </Badge>
  );
}

function DeleteAction({
  title,
  detail,
  confirm,
  onConfirm,
}: {
  title: string;
  detail: string;
  confirm: string;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="destructive" size="sm">
          <Trash2 data-icon="inline-start" aria-hidden="true" />
          {t("notifications.delete")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <TriangleAlert />
          </AlertDialogMedia>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{detail}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            {confirm}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function EmptyCard({ text }: { text: string }) {
  return (
    <Card>
      <CardContent className="py-10 text-center text-sm text-muted-foreground">
        {text}
      </CardContent>
    </Card>
  );
}

function NotificationSkeleton() {
  return (
    <Card aria-busy="true">
      <CardHeader>
        <Skeleton className="h-5 w-40" />
        <Skeleton className="h-4 w-64 max-w-full" />
      </CardHeader>
      <CardContent className="space-y-3">
        <Skeleton className="h-9 w-full" />
        <Skeleton className="h-9 w-4/5" />
      </CardContent>
    </Card>
  );
}

function senderIcon(kind: NotificationSender["kind"]) {
  if (kind === "telegram") return Bot;
  if (kind === "webhook") return Webhook;
  return Code2;
}

function senderDescription(
  sender: NotificationSender,
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (sender.telegram)
    return sender.telegram.topicId === undefined
      ? t("notifications.senders.telegramDetail", {
          value: sender.telegram.chatId,
        })
      : t("notifications.senders.telegramTopicDetail", {
          chatId: sender.telegram.chatId,
          topicId: sender.telegram.topicId,
        });
  if (sender.webhook) return sender.webhook.url;
  return t("notifications.senders.javascriptDetail");
}

function notificationProbeFieldLabel(
  fieldId: string,
  fields: NotificationProbeField[],
  t: ReturnType<typeof useTranslation>["t"],
) {
  const field = fields.find((item) => item.id === fieldId);
  return field
    ? presentProbeField(field, t).name
    : t("notifications.rules.fieldUnavailable");
}

function eventTypeLabel(
  value: string,
  t: ReturnType<typeof useTranslation>["t"],
) {
  return notificationEventTypes.includes(value as NotificationEventType)
    ? t(`notifications.eventType.${value as NotificationEventType}`)
    : value;
}

function parseHeaders(value: string) {
  const result: Record<string, string> = {};
  for (const source of value.split("\n")) {
    const line = source.trim();
    if (!line) continue;
    const separator = line.indexOf(":");
    if (separator <= 0) return undefined;
    const name = line.slice(0, separator).trim();
    const headerValue = line.slice(separator + 1).trim();
    if (!name || !headerValue) return undefined;
    result[name] = headerValue;
  }
  return result;
}
