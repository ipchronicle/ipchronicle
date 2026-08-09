import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  KeyRound,
  LoaderCircle,
  Network,
  Plus,
  RefreshCw,
  Save,
  Trash2,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  createNetworkProxy,
  deleteNetworkProxy,
  listNetworkProxies,
  updateNetworkProxy,
  type NetworkProxy,
  type NetworkProxyCreate,
  type NetworkProxyUpdate,
} from "@/api/proxies";
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
import { formatAPIError } from "@/lib/api-error";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; proxies: NetworkProxy[] }
  | { kind: "error" };

const emptyCreate: NetworkProxyCreate = {
  name: "",
  scheme: "http",
  host: "",
  port: 8080,
};

export function NetworkSettingsPage() {
  const { t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [feedback, setFeedback] = useState<string>();
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(async (signal?: AbortSignal, initial = false) => {
    if (initial) setState({ kind: "loading" });
    else setRefreshing(true);
    try {
      setState({ kind: "success", proxies: await listNetworkProxies(signal) });
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

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  function replaceProxy(proxy: NetworkProxy) {
    setState((current) =>
      current.kind === "success"
        ? {
            kind: "success",
            proxies: current.proxies.map((item) =>
              item.id === proxy.id ? proxy : item,
            ),
          }
        : current,
    );
  }

  async function create(input: NetworkProxyCreate) {
    setFeedback(undefined);
    try {
      const proxy = await createNetworkProxy(input, csrfToken);
      setState((current) =>
        current.kind === "success"
          ? { kind: "success", proxies: [...current.proxies, proxy] }
          : current,
      );
      return true;
    } catch (error) {
      setFeedback(formatAPIError(error, t));
      return false;
    }
  }

  async function update(proxyId: string, input: NetworkProxyUpdate) {
    setFeedback(undefined);
    try {
      replaceProxy(await updateNetworkProxy(proxyId, input, csrfToken));
      return true;
    } catch (error) {
      setFeedback(formatAPIError(error, t));
      return false;
    }
  }

  async function remove(proxyId: string) {
    setFeedback(undefined);
    try {
      await deleteNetworkProxy(proxyId, csrfToken);
      setState((current) =>
        current.kind === "success"
          ? {
              kind: "success",
              proxies: current.proxies.filter((item) => item.id !== proxyId),
            }
          : current,
      );
    } catch (error) {
      setFeedback(formatAPIError(error, t));
    }
  }

  return (
    <main className="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          <p className="text-xs font-medium text-muted-foreground uppercase">
            {t("proxySettings.section")}
          </p>
          <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
            {t("proxySettings.title")}
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {t("proxySettings.detail")}
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
          {t("proxySettings.refresh")}
        </Button>
      </div>

      <div className="mt-8 space-y-4" aria-live="polite">
        {feedback ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertDescription>{feedback}</AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "loading" ? <ProxySkeleton /> : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("proxySettings.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                variant="outline"
                size="sm"
                className="mt-3"
                onClick={() => void load(undefined, true)}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("proxySettings.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <>
            <CreateProxyCard onCreate={create} />
            {state.proxies.length === 0 ? (
              <Card>
                <CardContent className="py-10 text-center text-sm text-muted-foreground">
                  {t("proxySettings.empty")}
                </CardContent>
              </Card>
            ) : (
              state.proxies.map((proxy) => (
                <ProxyCard
                  key={proxy.id}
                  proxy={proxy}
                  onUpdate={update}
                  onDelete={remove}
                />
              ))
            )}
          </>
        ) : null}
      </div>
    </main>
  );
}

function CreateProxyCard({
  onCreate,
}: {
  onCreate: (input: NetworkProxyCreate) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const [input, setInput] = useState<NetworkProxyCreate>(emptyCreate);
  const [working, setWorking] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setWorking(true);
    try {
      const created = await onCreate({
        ...input,
        username: input.username || undefined,
        password: input.password || undefined,
      });
      if (created) setInput(emptyCreate);
    } finally {
      setWorking(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Plus aria-hidden="true" className="size-4" />
          {t("proxySettings.create.title")}
        </CardTitle>
        <CardDescription>{t("proxySettings.create.detail")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form className="grid gap-4 md:grid-cols-2" onSubmit={submit}>
          <ProxyFields input={input} onChange={setInput} prefix="create" />
          <div className="md:col-span-2">
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
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function ProxyCard({
  proxy,
  onUpdate,
  onDelete,
}: {
  proxy: NetworkProxy;
  onUpdate: (proxyId: string, input: NetworkProxyUpdate) => Promise<boolean>;
  onDelete: (proxyId: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [input, setInput] = useState<NetworkProxyCreate>({
    name: proxy.name,
    scheme: proxy.scheme,
    host: proxy.host,
    port: proxy.port,
    username: proxy.username,
  });
  const [working, setWorking] = useState(false);

  async function update(passwordAction: "keep" | "replace" | "clear") {
    setWorking(true);
    try {
      const updated = await onUpdate(proxy.id, {
        name: input.name,
        scheme: input.scheme,
        host: input.host,
        port: input.port,
        username: input.username || undefined,
        passwordAction,
        password: passwordAction === "replace" ? input.password : undefined,
      });
      if (updated) {
        setInput((current) => ({ ...current, password: undefined }));
      }
    } finally {
      setWorking(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex min-w-0 items-center gap-2">
          <Network aria-hidden="true" className="size-4 shrink-0" />
          <span className="truncate">{proxy.name}</span>
        </CardTitle>
        <CardDescription className="break-all">
          {proxy.scheme}://{proxy.host}:{proxy.port}
        </CardDescription>
        <CardAction>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                disabled={working}
                aria-label={t("proxySettings.delete.action")}
              >
                <Trash2 aria-hidden="true" />
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogMedia>
                  <TriangleAlert aria-hidden="true" />
                </AlertDialogMedia>
                <AlertDialogTitle>
                  {t("proxySettings.delete.title")}
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {t("proxySettings.delete.detail")}
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                <AlertDialogAction
                  variant="destructive"
                  onClick={() => void onDelete(proxy.id)}
                >
                  {t("proxySettings.delete.confirm")}
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className="grid gap-4 md:grid-cols-2">
          <ProxyFields
            input={input}
            onChange={setInput}
            prefix={proxy.id}
            retainedPassword
          />
          <div className="flex flex-wrap items-center gap-2 md:col-span-2">
            <Badge variant={proxy.passwordConfigured ? "outline" : "secondary"}>
              <KeyRound aria-hidden="true" />
              {proxy.passwordConfigured
                ? t("proxySettings.password.configured")
                : t("proxySettings.password.empty")}
            </Badge>
            <Button
              type="button"
              variant="outline"
              disabled={working}
              onClick={() => void update("keep")}
            >
              <Save data-icon="inline-start" aria-hidden="true" />
              {t("proxySettings.save")}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={working || !input.password}
              onClick={() => void update("replace")}
            >
              <KeyRound data-icon="inline-start" aria-hidden="true" />
              {t("proxySettings.password.replace")}
            </Button>
            {proxy.passwordConfigured ? (
              <AlertDialog>
                <AlertDialogTrigger asChild>
                  <Button type="button" variant="ghost" disabled={working}>
                    {t("proxySettings.password.clear")}
                  </Button>
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogMedia>
                      <TriangleAlert aria-hidden="true" />
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
                    <AlertDialogAction onClick={() => void update("clear")}>
                      {t("proxySettings.password.clear")}
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            ) : null}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function ProxyFields({
  input,
  onChange,
  prefix,
  retainedPassword = false,
}: {
  input: NetworkProxyCreate;
  onChange: (input: NetworkProxyCreate) => void;
  prefix: string;
  retainedPassword?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-proxy-name`}>
          {t("proxySettings.fields.name")}
        </Label>
        <Input
          id={`${prefix}-proxy-name`}
          required
          maxLength={128}
          value={input.name}
          onChange={(event) => onChange({ ...input, name: event.target.value })}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-proxy-scheme`}>
          {t("proxySettings.fields.scheme")}
        </Label>
        <Select
          value={input.scheme}
          onValueChange={(scheme: "http" | "https" | "socks5") =>
            onChange({ ...input, scheme })
          }
        >
          <SelectTrigger id={`${prefix}-proxy-scheme`} className="w-full">
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
        <Label htmlFor={`${prefix}-proxy-host`}>
          {t("proxySettings.fields.host")}
        </Label>
        <Input
          id={`${prefix}-proxy-host`}
          required
          maxLength={253}
          value={input.host}
          placeholder="proxy.example.com"
          onChange={(event) => onChange({ ...input, host: event.target.value })}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-proxy-port`}>
          {t("proxySettings.fields.port")}
        </Label>
        <Input
          id={`${prefix}-proxy-port`}
          type="number"
          required
          min={1}
          max={65535}
          value={input.port}
          onChange={(event) =>
            onChange({ ...input, port: Number(event.target.value) })
          }
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-proxy-username`}>
          {t("proxySettings.fields.username")}
        </Label>
        <Input
          id={`${prefix}-proxy-username`}
          maxLength={512}
          autoComplete="off"
          value={input.username ?? ""}
          onChange={(event) =>
            onChange({ ...input, username: event.target.value })
          }
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${prefix}-proxy-password`}>
          {t("proxySettings.fields.password")}
        </Label>
        <Input
          id={`${prefix}-proxy-password`}
          type="password"
          maxLength={4096}
          autoComplete="new-password"
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

function ProxySkeleton() {
  return (
    <Card aria-busy="true">
      <CardHeader>
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-4 w-72 max-w-full" />
      </CardHeader>
      <CardContent className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-8 w-full" />
      </CardContent>
    </Card>
  );
}
