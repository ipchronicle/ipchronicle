import { type FormEvent, type ReactNode, useEffect, useState } from "react";
import {
  CheckCircle2,
  Clipboard,
  KeyRound,
  LoaderCircle,
  LogOut,
  ShieldCheck,
  ShieldOff,
  UserRound,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { QRCodeSVG } from "qrcode.react";

import {
  confirmTOTPEnrollment,
  disableTOTP,
  revokeAllSessions,
  startTOTPEnrollment,
  updateAccount,
  type TOTPEnrollment,
} from "@/api/auth";
import { useAuth } from "@/auth-context";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { type SupportedLocale } from "@/i18n";
import { formatAPIError } from "@/lib/api-error";

type Feedback =
  | { kind: "success"; message: string }
  | { kind: "error"; message: string }
  | null;

export function AccountPage() {
  const { t } = useTranslation();
  const { state, setAccount, clearSession, changeLocale } = useAuth();
  if (state.status !== "authenticated") {
    return null;
  }
  const account = state.session.account;

  return (
    <main className="mx-auto w-full max-w-4xl px-4 py-10 sm:px-6 sm:py-14">
      <div className="max-w-2xl">
        <p className="text-xs font-medium text-muted-foreground uppercase">
          {t("settings.section")}
        </p>
        <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
          {t("account.title")}
        </h1>
      </div>

      <div className="mt-8 border-y">
        <ProfileSection
          account={account}
          setAccount={setAccount}
          clearSession={clearSession}
        />
        <LocaleSection locale={account.locale} changeLocale={changeLocale} />
        <TOTPSection
          enabled={account.totpEnabled}
          setAccount={setAccount}
          clearSession={clearSession}
        />
        <SessionSection clearSession={clearSession} />
      </div>
    </main>
  );
}

function SectionHeading({
  icon,
  title,
  detail,
}: {
  icon: ReactNode;
  title: string;
  detail: string;
}) {
  return (
    <div className="flex items-start gap-3">
      <span className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        {icon}
      </span>
      <div>
        <h2 className="text-sm font-semibold">{title}</h2>
        <p className="mt-1 text-sm text-muted-foreground">{detail}</p>
      </div>
    </div>
  );
}

function ProfileSection({
  account,
  setAccount,
  clearSession,
}: {
  account: { username: string };
  setAccount: ReturnType<typeof useAuth>["setAccount"];
  clearSession: () => void;
}) {
  const { t } = useTranslation();
  const [username, setUsername] = useState(account.username);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [saving, setSaving] = useState(false);
  const [feedback, setFeedback] = useState<Feedback>(null);

  useEffect(() => setUsername(account.username), [account.username]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setFeedback(null);
    try {
      const result = await updateAccount({
        currentPassword,
        ...(username !== account.username ? { username } : {}),
        ...(newPassword !== "" ? { newPassword } : {}),
      });
      setAccount(result.account);
      setCurrentPassword("");
      setNewPassword("");
      if (result.sessionRevoked) {
        clearSession();
        return;
      }
      setFeedback({ kind: "success", message: t("account.saved") });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setSaving(false);
    }
  }

  const hasChange = username !== account.username || newPassword !== "";
  return (
    <section className="py-7">
      <SectionHeading
        icon={<UserRound aria-hidden="true" className="size-4" />}
        title={t("account.profile")}
        detail={t("account.profileDetail")}
      />
      <form className="mt-6 max-w-lg space-y-5" onSubmit={submit}>
        <FeedbackAlert feedback={feedback} />
        <div className="space-y-2">
          <Label htmlFor="account-username">{t("account.username")}</Label>
          <Input
            id="account-username"
            value={username}
            maxLength={64}
            onChange={(event) => setUsername(event.target.value)}
            required
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="new-password">{t("account.newPassword")}</Label>
          <Input
            id="new-password"
            type="password"
            autoComplete="new-password"
            value={newPassword}
            minLength={8}
            maxLength={128}
            onChange={(event) => setNewPassword(event.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="profile-current-password">
            {t("account.currentPassword")}
          </Label>
          <Input
            id="profile-current-password"
            type="password"
            autoComplete="current-password"
            value={currentPassword}
            maxLength={128}
            onChange={(event) => setCurrentPassword(event.target.value)}
            required
          />
        </div>
        <Button type="submit" disabled={!hasChange || saving}>
          {saving ? (
            <LoaderCircle
              data-icon="inline-start"
              aria-hidden="true"
              className="animate-spin"
            />
          ) : (
            <KeyRound data-icon="inline-start" aria-hidden="true" />
          )}
          {t("account.save")}
        </Button>
      </form>
    </section>
  );
}

function LocaleSection({
  locale,
  changeLocale,
}: {
  locale: SupportedLocale;
  changeLocale: (locale: SupportedLocale) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [feedback, setFeedback] = useState<Feedback>(null);
  const [saving, setSaving] = useState(false);

  async function selectLocale(value: SupportedLocale) {
    if (value === locale) return;
    setSaving(true);
    setFeedback(null);
    try {
      await changeLocale(value);
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="border-t py-7">
      <SectionHeading
        icon={
          <span aria-hidden="true" className="text-xs font-semibold">
            A
          </span>
        }
        title={t("account.language")}
        detail={t("account.languageDetail")}
      />
      <div className="mt-6 max-w-lg space-y-4">
        <FeedbackAlert feedback={feedback} />
        <div
          className="inline-flex rounded-lg border bg-muted/40 p-1"
          role="group"
          aria-label={t("account.language")}
        >
          <Button
            type="button"
            size="sm"
            variant={locale === "zh-CN" ? "secondary" : "ghost"}
            aria-pressed={locale === "zh-CN"}
            disabled={saving}
            onClick={() => void selectLocale("zh-CN")}
          >
            简体中文
          </Button>
          <Button
            type="button"
            size="sm"
            variant={locale === "en" ? "secondary" : "ghost"}
            aria-pressed={locale === "en"}
            disabled={saving}
            onClick={() => void selectLocale("en")}
          >
            English
          </Button>
        </div>
      </div>
    </section>
  );
}

function TOTPSection({
  enabled,
  setAccount,
  clearSession,
}: {
  enabled: boolean;
  setAccount: ReturnType<typeof useAuth>["setAccount"];
  clearSession: () => void;
}) {
  const { t } = useTranslation();
  const [currentPassword, setCurrentPassword] = useState("");
  const [code, setCode] = useState("");
  const [enrollment, setEnrollment] = useState<TOTPEnrollment | null>(null);
  const [working, setWorking] = useState(false);
  const [feedback, setFeedback] = useState<Feedback>(null);

  async function beginEnrollment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setWorking(true);
    setFeedback(null);
    try {
      setEnrollment(await startTOTPEnrollment(currentPassword));
      setCurrentPassword("");
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(false);
    }
  }

  async function confirmEnrollment(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setWorking(true);
    setFeedback(null);
    try {
      const account = await confirmTOTPEnrollment(code);
      setAccount(account);
      setEnrollment(null);
      setCode("");
      setFeedback({ kind: "success", message: t("totp.enabled") });
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(false);
    }
  }

  async function removeTOTP(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setWorking(true);
    setFeedback(null);
    try {
      await disableTOTP(currentPassword, code);
      clearSession();
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
    } finally {
      setWorking(false);
    }
  }

  async function copySecret() {
    if (enrollment === null) return;
    try {
      await navigator.clipboard.writeText(enrollment.secret);
      setFeedback({ kind: "success", message: t("totp.copied") });
    } catch {
      setFeedback({ kind: "error", message: t("errors.actionFailed") });
    }
  }

  return (
    <section className="border-t py-7">
      <SectionHeading
        icon={
          enabled ? (
            <ShieldCheck aria-hidden="true" className="size-4" />
          ) : (
            <ShieldOff aria-hidden="true" className="size-4" />
          )
        }
        title={t("totp.title")}
        detail={enabled ? t("totp.active") : t("totp.inactive")}
      />
      <div className="mt-6 max-w-lg space-y-5">
        <FeedbackAlert feedback={feedback} />
        {!enabled && enrollment === null ? (
          <form className="space-y-4" onSubmit={beginEnrollment}>
            <div className="space-y-2">
              <Label htmlFor="totp-current-password">
                {t("account.currentPassword")}
              </Label>
              <Input
                id="totp-current-password"
                type="password"
                autoComplete="current-password"
                value={currentPassword}
                maxLength={128}
                onChange={(event) => setCurrentPassword(event.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={working}>
              <ShieldCheck data-icon="inline-start" aria-hidden="true" />
              {t("totp.enable")}
            </Button>
          </form>
        ) : null}
        {!enabled && enrollment !== null ? (
          <form className="space-y-5" onSubmit={confirmEnrollment}>
            <div className="w-fit rounded-md bg-white p-3">
              <QRCodeSVG
                value={enrollment.provisioningUri}
                size={168}
                level="M"
                bgColor="#ffffff"
                fgColor="#000000"
                aria-label={t("totp.qrCode")}
              />
            </div>
            <div className="space-y-2">
              <Label>{t("totp.secret")}</Label>
              <div className="flex min-w-0 items-center gap-2">
                <code className="min-w-0 flex-1 overflow-x-auto rounded-md bg-muted px-3 py-2 text-sm">
                  {enrollment.secret}
                </code>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      size="icon"
                      variant="outline"
                      onClick={() => void copySecret()}
                      aria-label={t("totp.copySecret")}
                    >
                      <Clipboard aria-hidden="true" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t("totp.copySecret")}</TooltipContent>
                </Tooltip>
              </div>
            </div>
            <TOTPCodeInput code={code} setCode={setCode} />
            <Button type="submit" disabled={working}>
              {working ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <CheckCircle2 data-icon="inline-start" aria-hidden="true" />
              )}
              {t("totp.confirm")}
            </Button>
          </form>
        ) : null}
        {enabled ? (
          <form className="space-y-4" onSubmit={removeTOTP}>
            <div className="space-y-2">
              <Label htmlFor="disable-totp-password">
                {t("account.currentPassword")}
              </Label>
              <Input
                id="disable-totp-password"
                type="password"
                autoComplete="current-password"
                value={currentPassword}
                maxLength={128}
                onChange={(event) => setCurrentPassword(event.target.value)}
                required
              />
            </div>
            <TOTPCodeInput code={code} setCode={setCode} id="disable-code" />
            <Button type="submit" variant="destructive" disabled={working}>
              <ShieldOff data-icon="inline-start" aria-hidden="true" />
              {t("totp.disable")}
            </Button>
          </form>
        ) : null}
      </div>
    </section>
  );
}

function TOTPCodeInput({
  code,
  setCode,
  id = "totp-confirm-code",
}: {
  code: string;
  setCode: (code: string) => void;
  id?: string;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{t("totp.code")}</Label>
      <Input
        id={id}
        inputMode="numeric"
        autoComplete="one-time-code"
        pattern="[0-9]{6}"
        maxLength={6}
        value={code}
        onChange={(event) => setCode(event.target.value.replace(/\D/g, ""))}
        required
      />
    </div>
  );
}

function SessionSection({ clearSession }: { clearSession: () => void }) {
  const { t } = useTranslation();
  const [confirming, setConfirming] = useState(false);
  const [working, setWorking] = useState(false);
  const [feedback, setFeedback] = useState<Feedback>(null);

  async function revoke() {
    setWorking(true);
    setFeedback(null);
    try {
      await revokeAllSessions();
      clearSession();
    } catch (error) {
      setFeedback({ kind: "error", message: formatAPIError(error, t) });
      setWorking(false);
    }
  }

  return (
    <section className="border-t py-7">
      <SectionHeading
        icon={<LogOut aria-hidden="true" className="size-4" />}
        title={t("sessions.title")}
        detail={t("sessions.detail")}
      />
      <div className="mt-6 max-w-lg space-y-4">
        <FeedbackAlert feedback={feedback} />
        {confirming ? (
          <Alert variant="destructive">
            <AlertTitle>{t("sessions.confirmTitle")}</AlertTitle>
            <AlertDescription className="mt-3 flex flex-wrap gap-2">
              <Button
                type="button"
                variant="destructive"
                onClick={() => void revoke()}
                disabled={working}
              >
                {t("sessions.confirm")}
              </Button>
              <Button
                type="button"
                variant="outline"
                onClick={() => setConfirming(false)}
                disabled={working}
              >
                {t("common.cancel")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          <Button
            type="button"
            variant="destructive"
            onClick={() => setConfirming(true)}
          >
            <LogOut data-icon="inline-start" aria-hidden="true" />
            {t("sessions.revokeAll")}
          </Button>
        )}
      </div>
    </section>
  );
}

function FeedbackAlert({ feedback }: { feedback: Feedback }) {
  if (feedback === null) return null;
  return (
    <Alert variant={feedback.kind === "error" ? "destructive" : "default"}>
      {feedback.kind === "success" ? <CheckCircle2 aria-hidden="true" /> : null}
      <AlertDescription>{feedback.message}</AlertDescription>
    </Alert>
  );
}
