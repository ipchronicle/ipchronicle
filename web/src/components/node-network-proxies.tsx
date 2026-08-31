import { useEffect, useState, type FormEvent } from "react";
import {
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
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
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
import { formatTime } from "@/pages/node-probe-page";

const emptyCreate: NetworkProxyCreate = {
  name: "",
  scheme: "http",
  host: "",
  port: 8080,
};

export function NodeNetworkProxies({
  proxies,
  onCreate,
  onUpdate,
  onDelete,
}: {
  proxies: NetworkProxy[];
  onCreate: (input: NetworkProxyCreate) => Promise<boolean>;
  onUpdate: (proxyId: string, input: NetworkProxyUpdate) => Promise<boolean>;
  onDelete: (proxyId: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Network aria-hidden="true" className="size-4" />
          {t("network.proxies.title")}
        </CardTitle>
        <CardDescription>{t("network.proxies.detail")}</CardDescription>
        <CardAction className="flex items-center gap-2">
          <Badge variant="info">{proxies.length}</Badge>
          <CreateProxyDialog onCreate={onCreate} />
        </CardAction>
      </CardHeader>
      <CardContent>
        {proxies.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            {t("network.proxies.empty")}
          </p>
        ) : (
          <div className="overflow-hidden rounded-lg border">
            {proxies.map((proxy) => (
              <ProxyRow
                key={proxy.id}
                proxy={proxy}
                onUpdate={onUpdate}
                onDelete={onDelete}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function CreateProxyDialog({
  onCreate,
}: {
  onCreate: (input: NetworkProxyCreate) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState<NetworkProxyCreate>(emptyCreate);
  const [working, setWorking] = useState(false);

  function changeOpen(next: boolean) {
    if (working) return;
    setOpen(next);
    if (!next) setInput(emptyCreate);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setWorking(true);
    try {
      const created = await onCreate({
        ...input,
        username: input.username || undefined,
        password: input.password || undefined,
      });
      if (created) {
        setInput(emptyCreate);
        setOpen(false);
      }
    } finally {
      setWorking(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={changeOpen}>
      <DialogTrigger asChild>
        <Button type="button" size="sm">
          <Plus data-icon="inline-start" aria-hidden="true" />
          {t("proxySettings.create.submit")}
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl" closeLabel={t("common.close")}>
        <DialogHeader>
          <DialogTitle>{t("proxySettings.create.title")}</DialogTitle>
          <DialogDescription>
            {t("proxySettings.create.detail")}
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <ProxyFields input={input} onChange={setInput} prefix="create" />
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={working}>
                {t("common.cancel")}
              </Button>
            </DialogClose>
            <Button type="submit" disabled={working}>
              {working ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <Plus data-icon="inline-start" aria-hidden="true" />
              )}
              {t("proxySettings.create.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function ProxyRow({
  proxy,
  onUpdate,
  onDelete,
}: {
  proxy: NetworkProxy;
  onUpdate: (proxyId: string, input: NetworkProxyUpdate) => Promise<boolean>;
  onDelete: (proxyId: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const deleting = proxy.deletionStatus === "pending";
  return (
    <div className="grid min-w-0 gap-4 border-t p-4 first:border-t-0 lg:grid-cols-[minmax(190px,1fr)_minmax(260px,1.35fr)_auto] lg:items-center">
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

      <div className="flex flex-wrap gap-2 lg:justify-end">
        <EditProxyDialog
          proxy={proxy}
          disabled={deleting}
          onUpdate={onUpdate}
        />
        {proxy.passwordConfigured ? (
          <ClearPasswordButton
            disabled={deleting}
            onClear={() =>
              void onUpdate(proxy.id, {
                name: proxy.name,
                scheme: proxy.scheme,
                host: proxy.host,
                port: proxy.port,
                username: proxy.username,
                passwordAction: "clear",
              })
            }
          />
        ) : null}
        <DeleteProxyButton
          proxy={proxy}
          disabled={deleting}
          onDelete={() => void onDelete(proxy.id)}
        />
      </div>

      {proxy.deletionError ? (
        <Alert variant="destructive" className="lg:col-span-3">
          <TriangleAlert aria-hidden="true" />
          <AlertDescription>{proxy.deletionError}</AlertDescription>
        </Alert>
      ) : null}
    </div>
  );
}

function EditProxyDialog({
  proxy,
  disabled,
  onUpdate,
}: {
  proxy: NetworkProxy;
  disabled: boolean;
  onUpdate: (proxyId: string, input: NetworkProxyUpdate) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [input, setInput] = useState<NetworkProxyCreate>(() =>
    proxyInput(proxy),
  );
  const [working, setWorking] = useState(false);

  useEffect(() => {
    if (!open) setInput(proxyInput(proxy));
  }, [open, proxy]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setWorking(true);
    try {
      const replacePassword = Boolean(input.password);
      const updated = await onUpdate(proxy.id, {
        name: input.name,
        scheme: input.scheme,
        host: input.host,
        port: input.port,
        username: input.username || undefined,
        passwordAction: replacePassword ? "replace" : "keep",
        password: replacePassword ? input.password : undefined,
      });
      if (updated) setOpen(false);
    } finally {
      setWorking(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(next) => !working && setOpen(next)}>
      <DialogTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={disabled}>
          <Pencil data-icon="inline-start" aria-hidden="true" />
          {t("proxySettings.edit")}
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl" closeLabel={t("common.close")}>
        <DialogHeader>
          <DialogTitle>{t("proxySettings.editTitle")}</DialogTitle>
          <DialogDescription>{t("proxySettings.editDetail")}</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="grid gap-4 sm:grid-cols-2">
            <ProxyFields
              input={input}
              onChange={setInput}
              prefix={`edit-${proxy.id}`}
              retainedPassword
              disabled={working}
            />
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" disabled={working}>
                {t("common.cancel")}
              </Button>
            </DialogClose>
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
              {t("proxySettings.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
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
    proxy.status === "unavailable"
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

function ClearPasswordButton({
  disabled,
  onClear,
}: {
  disabled: boolean;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button type="button" variant="outline" size="sm" disabled={disabled}>
          <KeyRound data-icon="inline-start" aria-hidden="true" />
          {t("proxySettings.password.clear")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <KeyRound aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle>
            {t("proxySettings.password.clearTitle")}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {t("proxySettings.password.clearDetail")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={onClear}>
            {t("proxySettings.password.clear")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function DeleteProxyButton({
  proxy,
  disabled,
  onDelete,
}: {
  proxy: NetworkProxy;
  disabled: boolean;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          variant="destructive"
          size="sm"
          disabled={disabled}
        >
          <Trash2 data-icon="inline-start" aria-hidden="true" />
          {t("proxySettings.delete.action")}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <TriangleAlert aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle>{t("proxySettings.delete.title")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("proxySettings.delete.detail", { name: proxy.name })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onDelete}>
            {t("proxySettings.delete.confirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
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
