import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  LoaderCircle,
  RadioTower,
  RefreshCw,
  Save,
  TriangleAlert,
} from "lucide-react";
import { useTranslation } from "react-i18next";

import {
  getNetworkObservationSettings,
  updateNetworkObservationSettings,
  type NetworkObservationSettings,
  type NetworkObservationSettingsUpdate,
} from "@/api/network";
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
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { formatAPIError } from "@/lib/api-error";

type ViewState =
  | { kind: "loading" }
  | { kind: "success"; observation: NetworkObservationSettings }
  | { kind: "error" };

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
      const observation = await getNetworkObservationSettings(signal);
      setState({ kind: "success", observation });
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

  async function saveObservation(input: NetworkObservationSettingsUpdate) {
    setFeedback(undefined);
    try {
      const observation = await updateNetworkObservationSettings(
        input,
        csrfToken,
      );
      setState({ kind: "success", observation });
      return true;
    } catch (error) {
      setFeedback(formatAPIError(error, t));
      return false;
    }
  }

  return (
    <main className="w-full min-w-0 px-4 py-10 sm:px-6 sm:py-14">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          <p className="text-sm font-medium text-muted-foreground uppercase">
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
        {state.kind === "loading" ? <ObservationSkeleton /> : null}
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
          <ObservationSettingsCard
            settings={state.observation}
            onSave={saveObservation}
          />
        ) : null}
      </div>
    </main>
  );
}

function ObservationSettingsCard({
  settings,
  onSave,
}: {
  settings: NetworkObservationSettings;
  onSave: (input: NetworkObservationSettingsUpdate) => Promise<boolean>;
}) {
  const { i18n, t } = useTranslation();
  const [ipv4, setIPv4] = useState(settings.ipv4Services.join("\n"));
  const [ipv6, setIPv6] = useState(settings.ipv6Services.join("\n"));
  const [working, setWorking] = useState(false);
  const services = [...lines(ipv4), ...lines(ipv6)];
  const usesHTTP = services.some((service) => service.startsWith("http://"));

  useEffect(() => {
    setIPv4(settings.ipv4Services.join("\n"));
    setIPv6(settings.ipv6Services.join("\n"));
  }, [settings]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setWorking(true);
    try {
      await onSave({ ipv4Services: lines(ipv4), ipv6Services: lines(ipv6) });
    } finally {
      setWorking(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RadioTower aria-hidden="true" className="size-4" />
          {t("proxySettings.discovery.title")}
        </CardTitle>
        <CardDescription>{t("proxySettings.discovery.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">
            {t("proxySettings.discovery.updated", {
              value: new Date(settings.updatedAt).toLocaleString(i18n.language),
            })}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        <form className="space-y-4" onSubmit={submit}>
          {usesHTTP ? (
            <Alert>
              <TriangleAlert aria-hidden="true" />
              <AlertTitle>{t("proxySettings.discovery.httpTitle")}</AlertTitle>
              <AlertDescription>
                {t("proxySettings.discovery.httpDetail")}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="discovery-ipv4">
                {t("proxySettings.discovery.ipv4")}
              </Label>
              <Textarea
                id="discovery-ipv4"
                required
                rows={4}
                value={ipv4}
                onChange={(event) => setIPv4(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="discovery-ipv6">
                {t("proxySettings.discovery.ipv6")}
              </Label>
              <Textarea
                id="discovery-ipv6"
                required
                rows={4}
                value={ipv6}
                onChange={(event) => setIPv6(event.target.value)}
              />
            </div>
          </div>
          <p className="text-sm text-muted-foreground">
            {t("proxySettings.discovery.format")}
          </p>
          <Button type="submit" variant="outline" disabled={working}>
            {working ? (
              <LoaderCircle
                data-icon="inline-start"
                aria-hidden="true"
                className="animate-spin"
              />
            ) : (
              <Save data-icon="inline-start" aria-hidden="true" />
            )}
            {t("proxySettings.discovery.save")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function lines(value: string) {
  return value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}

function ObservationSkeleton() {
  return (
    <Card aria-busy="true">
      <CardHeader>
        <Skeleton className="h-5 w-48" />
        <Skeleton className="h-4 w-72 max-w-full" />
      </CardHeader>
      <CardContent className="grid gap-4 md:grid-cols-2">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </CardContent>
    </Card>
  );
}
