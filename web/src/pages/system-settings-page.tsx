import {
  type FormEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useState,
} from "react";
import {
  CalendarClock,
  Database,
  GitCommitHorizontal,
  Globe2,
  LoaderCircle,
  PackageCheck,
  RefreshCw,
  Save,
  ServerCog,
  ShieldCheck,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  getSystemStatus,
  getSystemSettings,
  updateSystemSettings,
  type SystemSettings,
} from "@/api/system";
import {
  getAgentUpdateState,
  updateReleaseChannel,
  type AgentUpdateState,
  type ReleaseChannel,
} from "@/api/updates";
import { useAuth } from "@/auth-context";
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
import { formatAPIError } from "@/lib/api-error";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; value: AgentUpdateState }
  | { kind: "error" };

export function SystemSettingsPage() {
  const { i18n, t } = useTranslation();
  const { state: authState } = useAuth();
  const [state, setState] = useState<ViewState>({ kind: "loading" });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string>();

  const load = useCallback((signal?: AbortSignal) => {
    setState({ kind: "loading" });
    setError(undefined);
    void getAgentUpdateState(signal)
      .then((value) => setState({ kind: "success", value }))
      .catch((cause: unknown) => {
        if (cause instanceof DOMException && cause.name === "AbortError") {
          return;
        }
        setState({ kind: "error" });
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const csrfToken =
    authState.status === "authenticated" ? authState.session.csrfToken : "";

  async function changeChannel(channel: ReleaseChannel) {
    if (state.kind !== "success" || channel === state.value.channel) return;
    setSaving(true);
    setError(undefined);
    try {
      setState({
        kind: "success",
        value: await updateReleaseChannel(channel, csrfToken),
      });
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="max-w-2xl">
        <p className="text-sm font-medium text-muted-foreground uppercase">
          {t("settings.section")}
        </p>
        <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
          {t("systemSettings.title")}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {t("systemSettings.detail")}
        </p>
      </div>

      <ExternalOriginSettings csrfToken={csrfToken} />

      <Card className="mt-8" aria-live="polite">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <PackageCheck aria-hidden="true" className="size-4" />
            {t("systemSettings.release.title")}
          </CardTitle>
          <CardDescription>
            {t("systemSettings.release.detail")}
          </CardDescription>
          {state.kind === "success" ? (
            <CardAction>
              <Badge variant="secondary">
                {t(`systemSettings.release.channel.${state.value.channel}`)}
              </Badge>
            </CardAction>
          ) : null}
        </CardHeader>
        <CardContent className="space-y-5">
          {state.kind === "loading" ? <ReleaseSettingsSkeleton /> : null}
          {state.kind === "error" ? (
            <Alert variant="destructive">
              <TriangleAlert aria-hidden="true" />
              <AlertTitle>{t("systemSettings.release.loadFailed")}</AlertTitle>
              <AlertDescription>
                <Button
                  className="mt-3"
                  variant="outline"
                  size="sm"
                  onClick={() => load()}
                >
                  <RefreshCw data-icon="inline-start" aria-hidden="true" />
                  {t("systemSettings.release.retry")}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {state.kind === "success" ? (
            <>
              {error !== undefined ? (
                <Alert variant="destructive">
                  <TriangleAlert aria-hidden="true" />
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              ) : null}
              {state.value.discoveryError !== undefined ? (
                <Alert>
                  <TriangleAlert aria-hidden="true" />
                  <AlertTitle>
                    {t("systemSettings.release.discoveryFailed")}
                  </AlertTitle>
                  <AlertDescription>
                    {t(
                      `systemSettings.release.discoveryErrors.${state.value.discoveryError}`,
                    )}
                  </AlertDescription>
                </Alert>
              ) : null}

              <div className="flex flex-col justify-between gap-3 rounded-md border p-4 sm:flex-row sm:items-center">
                <div>
                  <p className="text-sm font-medium">
                    {t("systemSettings.release.channelLabel")}
                  </p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {t("systemSettings.release.channelDetail")}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  {saving ? (
                    <LoaderCircle
                      className="size-4 animate-spin text-muted-foreground"
                      aria-label={t("systemSettings.release.saving")}
                    />
                  ) : null}
                  <Select
                    value={state.value.channel}
                    disabled={saving}
                    onValueChange={(value) =>
                      void changeChannel(value as ReleaseChannel)
                    }
                  >
                    <SelectTrigger
                      className="w-full sm:w-52"
                      aria-label={t("systemSettings.release.channelLabel")}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="stable">
                        {t("systemSettings.release.channel.stable")}
                      </SelectItem>
                      <SelectItem value="rc">
                        {t("systemSettings.release.channel.rc")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <dl className="grid gap-px overflow-hidden rounded-md border bg-border sm:grid-cols-2 lg:grid-cols-3">
                <ReleaseField
                  icon={<ServerCog aria-hidden="true" />}
                  label={t("systemSettings.release.currentVersion")}
                  value={state.value.currentVersion}
                />
                <ReleaseField
                  icon={<GitCommitHorizontal aria-hidden="true" />}
                  label={t("systemSettings.release.currentRevision")}
                  value={state.value.currentRevision}
                  mono
                />
                <ReleaseField
                  icon={<CalendarClock aria-hidden="true" />}
                  label={t("systemSettings.release.checkedAt")}
                  value={formatDate(
                    state.value.checkedAt,
                    i18n.resolvedLanguage,
                  )}
                />
                <ReleaseField
                  icon={<PackageCheck aria-hidden="true" />}
                  label={t("systemSettings.release.availableVersion")}
                  value={
                    state.value.availableRelease?.version ??
                    t("systemSettings.release.noneAvailable")
                  }
                />
                <ReleaseField
                  icon={<GitCommitHorizontal aria-hidden="true" />}
                  label={t("systemSettings.release.availableRevision")}
                  value={
                    state.value.availableRelease?.revision ??
                    t("systemSettings.release.notAvailable")
                  }
                  mono
                />
                <ReleaseField
                  icon={<CalendarClock aria-hidden="true" />}
                  label={t("systemSettings.release.publishedAt")}
                  value={
                    state.value.availableRelease === undefined
                      ? t("systemSettings.release.notAvailable")
                      : formatDate(
                          state.value.availableRelease.publishedAt,
                          i18n.resolvedLanguage,
                        )
                  }
                />
              </dl>
            </>
          ) : null}
        </CardContent>
      </Card>

      <RuntimeInformation />
    </div>
  );
}

type RuntimeViewState =
  | { kind: "loading" }
  | {
      kind: "success";
      value: Awaited<ReturnType<typeof getSystemStatus>>;
      checkedAt: Date;
    }
  | { kind: "error" };

function RuntimeInformation() {
  const { i18n, t } = useTranslation();
  const [state, setState] = useState<RuntimeViewState>({ kind: "loading" });

  const load = useCallback((signal?: AbortSignal) => {
    setState({ kind: "loading" });
    void getSystemStatus(signal)
      .then((value) =>
        setState({ kind: "success", value, checkedAt: new Date() }),
      )
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }
        setState({ kind: "error" });
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  return (
    <Card className="mt-8" aria-live="polite">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ServerCog aria-hidden="true" className="size-4" />
          {t("systemSettings.runtime.title")}
        </CardTitle>
        <CardDescription>{t("systemSettings.runtime.detail")}</CardDescription>
        {state.kind === "success" ? (
          <CardAction>
            <Badge variant="success">
              {t("systemSettings.runtime.operational")}
            </Badge>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent>
        {state.kind === "loading" ? (
          <Skeleton className="h-36 w-full" aria-busy="true" />
        ) : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>{t("systemSettings.runtime.loadFailed")}</AlertTitle>
            <AlertDescription>
              <Button
                className="mt-3"
                variant="outline"
                size="sm"
                onClick={() => load()}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("systemSettings.runtime.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <dl className="grid gap-px overflow-hidden rounded-md border bg-border sm:grid-cols-2 lg:grid-cols-4">
            <ReleaseField
              icon={<ServerCog aria-hidden="true" />}
              label={t("systemSettings.runtime.service")}
              value={state.value.service}
            />
            <ReleaseField
              icon={<Database aria-hidden="true" />}
              label={t("systemSettings.runtime.configSchema")}
              value={String(state.value.configSchemaVersion)}
            />
            <ReleaseField
              icon={<Database aria-hidden="true" />}
              label={t("systemSettings.runtime.historySchema")}
              value={String(state.value.historySchemaVersion)}
            />
            <ReleaseField
              icon={<ShieldCheck aria-hidden="true" />}
              label={t("systemSettings.runtime.transport")}
              value={state.value.transportSecurity.toUpperCase()}
            />
            <ReleaseField
              icon={<Globe2 aria-hidden="true" />}
              label={t("systemSettings.runtime.externalOrigin")}
              value={t(
                `systemSettings.runtime.originMode.${state.value.externalOriginMode}`,
              )}
            />
            <ReleaseField
              icon={<ShieldCheck aria-hidden="true" />}
              label={t("systemSettings.runtime.trustedProxy")}
              value={t(
                state.value.trustedProxyConfigured
                  ? "systemSettings.runtime.configured"
                  : "systemSettings.runtime.notConfigured",
              )}
            />
            <ReleaseField
              icon={<GitCommitHorizontal aria-hidden="true" />}
              label={t("systemSettings.runtime.sourceRevision")}
              value={state.value.sourceRevision}
              mono
            />
            <ReleaseField
              icon={<CalendarClock aria-hidden="true" />}
              label={t("systemSettings.runtime.checkedAt")}
              value={new Intl.DateTimeFormat(i18n.resolvedLanguage, {
                dateStyle: "medium",
                timeStyle: "short",
              }).format(state.checkedAt)}
            />
          </dl>
        ) : null}
      </CardContent>
    </Card>
  );
}

type OriginViewState =
  | { kind: "loading" }
  | { kind: "success"; value: SystemSettings }
  | { kind: "error" };

function ExternalOriginSettings({ csrfToken }: { csrfToken: string }) {
  const { t } = useTranslation();
  const browserOrigin = window.location.origin;
  const [state, setState] = useState<OriginViewState>({ kind: "loading" });
  const [automatic, setAutomatic] = useState(true);
  const [externalOrigin, setExternalOrigin] = useState(browserOrigin);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string>();

  const load = useCallback(
    (signal?: AbortSignal) => {
      setState({ kind: "loading" });
      setError(undefined);
      setSaved(false);
      void getSystemSettings(signal)
        .then((value) => {
          setState({ kind: "success", value });
          setAutomatic(value.automatic);
          setExternalOrigin(
            value.automatic ? browserOrigin : value.externalOrigin,
          );
        })
        .catch((cause: unknown) => {
          if (cause instanceof DOMException && cause.name === "AbortError") {
            return;
          }
          setState({ kind: "error" });
        });
    },
    [browserOrigin],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (state.kind !== "success") return;
    const value = automatic ? "" : externalOrigin.trim();
    if (!automatic && value === "") {
      setError(t("systemSettings.externalOrigin.required"));
      return;
    }
    setSaving(true);
    setSaved(false);
    setError(undefined);
    try {
      const updated = await updateSystemSettings(value, csrfToken);
      setState({ kind: "success", value: updated });
      setAutomatic(updated.automatic);
      setExternalOrigin(
        updated.automatic ? browserOrigin : updated.externalOrigin,
      );
      setSaved(true);
    } catch (cause) {
      setError(formatAPIError(cause, t));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card className="mt-8" aria-live="polite">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Globe2 aria-hidden="true" className="size-4" />
          {t("systemSettings.externalOrigin.title")}
        </CardTitle>
        <CardDescription>
          {t("systemSettings.externalOrigin.detail")}
        </CardDescription>
        {state.kind === "success" ? (
          <CardAction>
            <Badge variant="secondary">
              {t(
                automatic
                  ? "systemSettings.externalOrigin.mode.automatic"
                  : "systemSettings.externalOrigin.mode.custom",
              )}
            </Badge>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent>
        {state.kind === "loading" ? (
          <Skeleton className="h-36 w-full" aria-busy="true" />
        ) : null}
        {state.kind === "error" ? (
          <Alert variant="destructive">
            <TriangleAlert aria-hidden="true" />
            <AlertTitle>
              {t("systemSettings.externalOrigin.loadFailed")}
            </AlertTitle>
            <AlertDescription>
              <Button
                className="mt-3"
                variant="outline"
                size="sm"
                onClick={() => load()}
              >
                <RefreshCw data-icon="inline-start" aria-hidden="true" />
                {t("systemSettings.externalOrigin.retry")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {state.kind === "success" ? (
          <form className="space-y-5" onSubmit={(event) => void save(event)}>
            {error !== undefined ? (
              <Alert variant="destructive">
                <TriangleAlert aria-hidden="true" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            {saved ? (
              <Alert>
                <AlertDescription>
                  {t("systemSettings.externalOrigin.saved")}
                </AlertDescription>
              </Alert>
            ) : null}

            <div className="flex items-center justify-between gap-4 rounded-md border p-4">
              <div className="min-w-0">
                <p
                  id="external-origin-automatic-label"
                  className="text-sm leading-none font-medium"
                >
                  {t("systemSettings.externalOrigin.automatic")}
                </p>
                <p className="mt-1 break-all text-sm text-muted-foreground">
                  {browserOrigin}
                </p>
              </div>
              <Switch
                id="external-origin-automatic"
                aria-labelledby="external-origin-automatic-label"
                checked={automatic}
                disabled={saving}
                onCheckedChange={(checked) => {
                  setAutomatic(checked);
                  setSaved(false);
                  setError(undefined);
                  if (checked) setExternalOrigin(browserOrigin);
                }}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="external-origin-value">
                {t("systemSettings.externalOrigin.label")}
              </Label>
              <Input
                id="external-origin-value"
                type="url"
                value={externalOrigin}
                disabled={automatic || saving}
                required={!automatic}
                maxLength={2048}
                placeholder="https://ip.example.com"
                onChange={(event) => {
                  setExternalOrigin(event.target.value);
                  setSaved(false);
                }}
              />
              <p className="text-sm text-muted-foreground">
                {t("systemSettings.externalOrigin.valueDetail")}
              </p>
            </div>

            <Button type="submit" disabled={saving}>
              {saving ? (
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              ) : (
                <Save aria-hidden="true" />
              )}
              {t(
                saving
                  ? "systemSettings.externalOrigin.saving"
                  : "systemSettings.externalOrigin.save",
              )}
            </Button>
          </form>
        ) : null}
      </CardContent>
    </Card>
  );
}

function ReleaseField({
  icon,
  label,
  value,
  mono = false,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0 bg-card p-4">
      <dt className="flex items-center gap-2 text-sm font-medium text-muted-foreground [&_svg]:size-3.5">
        {icon}
        {label}
      </dt>
      <dd
        className={`mt-2 break-all text-base font-medium ${mono ? "font-mono" : ""}`}
      >
        {value}
      </dd>
    </div>
  );
}

function ReleaseSettingsSkeleton() {
  return (
    <div className="space-y-4" aria-busy="true">
      <Skeleton className="h-20 w-full" />
      <Skeleton className="h-36 w-full" />
    </div>
  );
}

function formatDate(value: string, locale?: string) {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}
