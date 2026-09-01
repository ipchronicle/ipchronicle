import { useEffect, useState, type FormEvent } from "react";
import {
  ArrowLeft,
  KeyRound,
  LoaderCircle,
  Network,
  Pencil,
  Plus,
  Save,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import type {
  NetworkProxy,
  NetworkProxyCreate,
  NetworkProxyUpdate,
} from "@/api/proxies";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { formatTime } from "@/pages/node-probe-page";

const emptyCreate: NetworkProxyCreate = {
  name: "",
  scheme: "http",
  host: "",
  port: 8080,
};

type ProxyManagerView =
  | { kind: "list" }
  | { kind: "create" }
  | { kind: "edit" | "clear-password" | "delete"; proxyId: string };

export function networkProxyAttentionCount(proxies: NetworkProxy[]) {
  return proxies.filter(
    (proxy) =>
      proxy.status === "unavailable" ||
      proxy.deletionStatus === "failed" ||
      Boolean(proxy.deletionError),
  ).length;
}

export function NetworkProxyManagerButton({
  proxies,
  onClick,
}: {
  proxies: NetworkProxy[];
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const attentionCount = networkProxyAttentionCount(proxies);
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className={
        attentionCount > 0
          ? "border-amber-500/60 text-amber-800 dark:text-amber-300"
          : undefined
      }
      onClick={onClick}
    >
      <Network data-icon="inline-start" aria-hidden="true" />
      {t("network.proxies.manage")}
      <Badge
        variant={attentionCount > 0 ? "warning" : "secondary"}
        aria-hidden="true"
      >
        {proxies.length}
      </Badge>
    </Button>
  );
}

export function NodeNetworkProxies({
  open,
  proxies,
  onOpenChange,
  onCreate,
  onUpdate,
  onDelete,
}: {
  open: boolean;
  proxies: NetworkProxy[];
  onOpenChange: (open: boolean) => void;
  onCreate: (input: NetworkProxyCreate) => Promise<boolean>;
  onUpdate: (proxyId: string, input: NetworkProxyUpdate) => Promise<boolean>;
  onDelete: (proxyId: string) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [view, setView] = useState<ProxyManagerView>({ kind: "list" });
  const selectedProxy =
    "proxyId" in view
      ? proxies.find((proxy) => proxy.id === view.proxyId)
      : undefined;

  useEffect(() => {
    if ("proxyId" in view && !selectedProxy) setView({ kind: "list" });
  }, [selectedProxy, view]);

  function changeOpen(next: boolean) {
    onOpenChange(next);
    if (!next) setView({ kind: "list" });
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogContent className="sm:max-w-4xl" closeLabel={t("common.close")}>
        {view.kind === "list" ? (
          <ProxyManagerList
            proxies={proxies}
            onCreate={() => setView({ kind: "create" })}
            onToggle={(proxy, enabled) =>
              onUpdate(proxy.id, {
                name: proxy.name,
                scheme: proxy.scheme,
                host: proxy.host,
                port: proxy.port,
                username: proxy.username,
                enabled,
                passwordAction: "keep",
              })
            }
            onEdit={(proxyId) => setView({ kind: "edit", proxyId })}
            onClearPassword={(proxyId) =>
              setView({ kind: "clear-password", proxyId })
            }
            onDelete={(proxyId) => setView({ kind: "delete", proxyId })}
          />
        ) : view.kind === "create" ? (
          <ProxyEditor
            onCreate={onCreate}
            onDone={() => setView({ kind: "list" })}
          />
        ) : selectedProxy && view.kind === "edit" ? (
          <ProxyEditor
            proxy={selectedProxy}
            onUpdate={onUpdate}
            onDone={() => setView({ kind: "list" })}
          />
        ) : selectedProxy &&
          (view.kind === "clear-password" || view.kind === "delete") ? (
          <ProxyConfirmation
            kind={view.kind}
            proxy={selectedProxy}
            onUpdate={onUpdate}
            onDelete={onDelete}
            onDone={() => setView({ kind: "list" })}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function ProxyManagerList({
  proxies,
  onCreate,
  onToggle,
  onEdit,
  onClearPassword,
  onDelete,
}: {
  proxies: NetworkProxy[];
  onCreate: () => void;
  onToggle: (proxy: NetworkProxy, enabled: boolean) => Promise<boolean>;
  onEdit: (proxyId: string) => void;
  onClearPassword: (proxyId: string) => void;
  onDelete: (proxyId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="flex items-start justify-between gap-4 pr-8">
        <DialogHeader>
          <DialogTitle>{t("network.proxies.title")}</DialogTitle>
          <DialogDescription>{t("network.proxies.detail")}</DialogDescription>
        </DialogHeader>
        <Button type="button" size="sm" onClick={onCreate}>
          <Plus data-icon="inline-start" aria-hidden="true" />
          {t("proxySettings.create.submit")}
        </Button>
      </div>
      {proxies.length === 0 ? (
        <p className="rounded-lg border border-dashed py-10 text-center text-sm text-muted-foreground">
          {t("network.proxies.empty")}
        </p>
      ) : (
        <div className="overflow-hidden rounded-lg border">
          {proxies.map((proxy) => (
            <ProxyRow
              key={proxy.id}
              proxy={proxy}
              onToggle={(enabled) => onToggle(proxy, enabled)}
              onEdit={() => onEdit(proxy.id)}
              onClearPassword={() => onClearPassword(proxy.id)}
              onDelete={() => onDelete(proxy.id)}
            />
          ))}
        </div>
      )}
    </>
  );
}

function ProxyRow({
  proxy,
  onToggle,
  onEdit,
  onClearPassword,
  onDelete,
}: {
  proxy: NetworkProxy;
  onToggle: (enabled: boolean) => Promise<boolean>;
  onEdit: () => void;
  onClearPassword: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  const deleting = proxy.deletionStatus === "pending";
  const [toggling, setToggling] = useState(false);

  async function toggle(enabled: boolean) {
    setToggling(true);
    try {
      await onToggle(enabled);
    } finally {
      setToggling(false);
    }
  }

  return (
    <div className="grid min-w-0 gap-4 border-t p-4 first:border-t-0 md:grid-cols-[minmax(180px,1fr)_minmax(260px,1.35fr)] md:items-center xl:grid-cols-[minmax(180px,1fr)_minmax(260px,1.35fr)_auto]">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <p className="truncate font-medium">{proxy.name}</p>
          <ProxyStatusBadge proxy={proxy} />
        </div>
        <p className="mt-1 break-all text-sm text-muted-foreground">
          {proxy.scheme.toUpperCase()} · {proxy.host}:{proxy.port}
        </p>
        {proxy.passwordConfigured ? (
          <Badge variant="outline" className="mt-2">
            <KeyRound aria-hidden="true" />
            {t("proxySettings.password.configured")}
          </Badge>
        ) : null}
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        <ProxyFamilyResult family="ipv4" result={proxy.ipv4} />
        <ProxyFamilyResult family="ipv6" result={proxy.ipv6} />
      </div>

      <div className="flex flex-wrap gap-2 md:col-span-2 xl:col-span-1 xl:justify-end">
        <div className="flex h-9 items-center gap-2 rounded-md border px-3">
          <span className="text-sm text-muted-foreground">
            {proxy.enabled
              ? t("network.proxies.enabled")
              : t("network.proxies.disabled")}
          </span>
          <Switch
            checked={proxy.enabled}
            disabled={deleting || toggling}
            aria-label={t("network.proxies.toggle", { name: proxy.name })}
            onCheckedChange={(enabled) => void toggle(enabled)}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={deleting}
          onClick={onEdit}
        >
          <Pencil data-icon="inline-start" aria-hidden="true" />
          {t("proxySettings.edit")}
        </Button>
        {proxy.passwordConfigured ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={deleting}
            onClick={onClearPassword}
          >
            <KeyRound data-icon="inline-start" aria-hidden="true" />
            {t("proxySettings.password.clear")}
          </Button>
        ) : null}
        <Button
          type="button"
          variant="destructive"
          size="sm"
          disabled={deleting}
          onClick={onDelete}
        >
          <Trash2 data-icon="inline-start" aria-hidden="true" />
          {t("proxySettings.delete.action")}
        </Button>
      </div>

      {proxy.deletionError ? (
        <Alert variant="destructive" className="md:col-span-2 xl:col-span-3">
          <TriangleAlert aria-hidden="true" />
          <AlertDescription>{proxy.deletionError}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  );
}

function proxyInput(proxy: NetworkProxy): NetworkProxyCreate {
  return {
    name: proxy.name,
    scheme: proxy.scheme,
    host: proxy.host,
    port: proxy.port,
    username: proxy.username,
  };
}

type ProxyEditorProps =
  | {
      proxy?: undefined;
      onCreate: (input: NetworkProxyCreate) => Promise<boolean>;
      onUpdate?: never;
      onDone: () => void;
    }
  | {
      proxy: NetworkProxy;
      onCreate?: never;
      onUpdate: (
        proxyId: string,
        input: NetworkProxyUpdate,
      ) => Promise<boolean>;
      onDone: () => void;
    };

function ProxyEditor({ proxy, onCreate, onUpdate, onDone }: ProxyEditorProps) {
  const { t } = useTranslation();
  const [input, setInput] = useState<NetworkProxyCreate>(() =>
    proxy ? proxyInput(proxy) : emptyCreate,
  );
  const [working, setWorking] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setWorking(true);
    try {
      let saved: boolean;
      if (proxy) {
        const replacePassword = Boolean(input.password);
        saved = await onUpdate(proxy.id, {
          name: input.name,
          scheme: input.scheme,
          host: input.host,
          port: input.port,
          username: input.username || undefined,
          enabled: proxy.enabled,
          passwordAction: replacePassword ? "replace" : "keep",
          password: replacePassword ? input.password : undefined,
        });
      } else {
        saved = await onCreate({
          ...input,
          username: input.username || undefined,
          password: input.password || undefined,
        });
      }
      if (saved) onDone();
    } finally {
      setWorking(false);
    }
  }

  return (
    <>
      <div className="flex items-start gap-3 pr-8">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          disabled={working}
          aria-label={t("network.proxies.back")}
          onClick={onDone}
        >
          <ArrowLeft aria-hidden="true" />
        </Button>
        <DialogHeader>
          <DialogTitle>
            {proxy
              ? t("proxySettings.editTitle")
              : t("proxySettings.create.title")}
          </DialogTitle>
          <DialogDescription>
            {proxy
              ? t("proxySettings.editDetail")
              : t("proxySettings.create.detail")}
          </DialogDescription>
        </DialogHeader>
      </div>
      <form className="space-y-4" onSubmit={submit}>
        <div className="grid gap-4 sm:grid-cols-2">
          <ProxyFields
            input={input}
            onChange={setInput}
            prefix={proxy ? `edit-${proxy.id}` : "create"}
            retainedPassword={Boolean(proxy)}
            disabled={working}
          />
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={working}
            onClick={onDone}
          >
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={working}>
            {working ? (
              <LoaderCircle
                data-icon="inline-start"
                aria-hidden="true"
                className="animate-spin"
              />
            ) : proxy ? (
              <Save data-icon="inline-start" aria-hidden="true" />
            ) : (
              <Plus data-icon="inline-start" aria-hidden="true" />
            )}
            {proxy ? t("proxySettings.save") : t("proxySettings.create.submit")}
          </Button>
        </DialogFooter>
      </form>
    </>
  );
}

function ProxyConfirmation({
  kind,
  proxy,
  onUpdate,
  onDelete,
  onDone,
}: {
  kind: "clear-password" | "delete";
  proxy: NetworkProxy;
  onUpdate: (proxyId: string, input: NetworkProxyUpdate) => Promise<boolean>;
  onDelete: (proxyId: string) => Promise<boolean>;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const [working, setWorking] = useState(false);
  const deleting = kind === "delete";

  async function confirm() {
    setWorking(true);
    try {
      const succeeded = deleting
        ? await onDelete(proxy.id)
        : await onUpdate(proxy.id, {
            name: proxy.name,
            scheme: proxy.scheme,
            host: proxy.host,
            port: proxy.port,
            username: proxy.username,
            enabled: proxy.enabled,
            passwordAction: "clear",
          });
      if (succeeded) onDone();
    } finally {
      setWorking(false);
    }
  }

  return (
    <>
      <div className="flex items-start gap-3 pr-8">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          disabled={working}
          aria-label={t("network.proxies.back")}
          onClick={onDone}
        >
          <ArrowLeft aria-hidden="true" />
        </Button>
        <DialogHeader>
          <DialogTitle>
            {deleting
              ? t("proxySettings.delete.title")
              : t("proxySettings.password.clearTitle")}
          </DialogTitle>
          <DialogDescription>
            {deleting
              ? t("proxySettings.delete.detail", { name: proxy.name })
              : t("proxySettings.password.clearDetail")}
          </DialogDescription>
        </DialogHeader>
      </div>
      <div className="rounded-lg border bg-muted/30 p-4">
        <p className="font-medium">{proxy.name}</p>
        <p className="mt-1 break-all text-sm text-muted-foreground">
          {proxy.scheme.toUpperCase()} · {proxy.host}:{proxy.port}
        </p>
      </div>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          disabled={working}
          onClick={onDone}
        >
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          variant={deleting ? "destructive" : "default"}
          disabled={working}
          onClick={() => void confirm()}
        >
          {working ? (
            <LoaderCircle
              data-icon="inline-start"
              aria-hidden="true"
              className="animate-spin"
            />
          ) : deleting ? (
            <Trash2 data-icon="inline-start" aria-hidden="true" />
          ) : (
            <KeyRound data-icon="inline-start" aria-hidden="true" />
          )}
          {deleting
            ? t("proxySettings.delete.confirm")
            : t("proxySettings.password.clear")}
        </Button>
      </DialogFooter>
    </>
  );
}

function ProxyStatusBadge({ proxy }: { proxy: NetworkProxy }) {
  const { t } = useTranslation();
  if (proxy.deletionStatus) {
    return (
      <Badge
        variant={proxy.deletionStatus === "failed" ? "destructive" : "warning"}
      >
        {t(`network.proxies.deletion.${proxy.deletionStatus}`)}
      </Badge>
    );
  }
  const variant =
    proxy.status === "disabled"
      ? "secondary"
      : proxy.status === "unavailable"
        ? "warning"
        : proxy.status === "checking"
          ? "info"
          : "success";
  return (
    <Badge variant={variant}>
      {proxy.status === "checking" ? (
        <LoaderCircle aria-hidden="true" className="animate-spin" />
      ) : null}
      {t(`network.proxies.status.${proxy.status}`)}
    </Badge>
  );
}

function ProxyFamilyResult({
  family,
  result,
}: {
  family: "ipv4" | "ipv6";
  result: NetworkProxy["ipv4"];
}) {
  const { i18n, t } = useTranslation();
  return (
    <div className="min-w-0 rounded-md bg-muted/50 p-3">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-muted-foreground">
          {t(`network.family.${family}`)}
        </span>
        <Badge
          variant={
            result.status === "available"
              ? "success"
              : result.status === "unavailable"
                ? "warning"
                : result.status === "disabled"
                  ? "secondary"
                  : "info"
          }
        >
          {t(`network.proxies.familyStatus.${result.status}`)}
        </Badge>
      </div>
      <p className="mt-2 break-all font-mono text-sm font-medium">
        {result.publicAddress ?? t("network.proxies.noAddress")}
      </p>
      {result.failureReason ? (
        <p className="mt-1 text-xs text-destructive">
          {t(`network.observation.failure.${result.failureReason}`)}
        </p>
      ) : result.lastCheckedAt ? (
        <p className="mt-1 text-xs text-muted-foreground">
          {formatTime(
            result.lastCheckedAt,
            i18n.resolvedLanguage,
            t("nodes.notAvailable"),
          )}
        </p>
      ) : null}
    </div>
  );
}

function ProxyFields({
  input,
  onChange,
  prefix,
  retainedPassword = false,
  disabled = false,
}: {
  input: NetworkProxyCreate;
  onChange: (input: NetworkProxyCreate) => void;
  prefix: string;
  retainedPassword?: boolean;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-name`}>
          {t("proxySettings.fields.name")}
        </Label>
        <Input
          id={`${prefix}-name`}
          required
          maxLength={128}
          disabled={disabled}
          value={input.name}
          onChange={(event) => onChange({ ...input, name: event.target.value })}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-scheme`}>
          {t("proxySettings.fields.scheme")}
        </Label>
        <Select
          value={input.scheme}
          disabled={disabled}
          onValueChange={(scheme) =>
            onChange({
              ...input,
              scheme: scheme as NetworkProxyCreate["scheme"],
            })
          }
        >
          <SelectTrigger id={`${prefix}-scheme`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="http">HTTP</SelectItem>
            <SelectItem value="https">HTTPS</SelectItem>
            <SelectItem value="socks5">SOCKS5</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-host`}>
          {t("proxySettings.fields.host")}
        </Label>
        <Input
          id={`${prefix}-host`}
          required
          maxLength={253}
          disabled={disabled}
          value={input.host}
          onChange={(event) => onChange({ ...input, host: event.target.value })}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-port`}>
          {t("proxySettings.fields.port")}
        </Label>
        <Input
          id={`${prefix}-port`}
          type="number"
          min={1}
          max={65535}
          required
          disabled={disabled}
          value={input.port}
          onChange={(event) =>
            onChange({ ...input, port: Number(event.target.value) })
          }
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-username`}>
          {t("proxySettings.fields.username")}
        </Label>
        <Input
          id={`${prefix}-username`}
          maxLength={512}
          autoComplete="off"
          disabled={disabled}
          value={input.username ?? ""}
          onChange={(event) =>
            onChange({ ...input, username: event.target.value })
          }
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-password`}>
          {t("proxySettings.fields.password")}
        </Label>
        <Input
          id={`${prefix}-password`}
          type="password"
          maxLength={4096}
          autoComplete="new-password"
          disabled={disabled}
          value={input.password ?? ""}
          placeholder={
            retainedPassword
              ? t("proxySettings.fields.passwordPlaceholder")
              : undefined
          }
          onChange={(event) =>
            onChange({ ...input, password: event.target.value })
          }
        />
      </div>
    </>
  );
}
