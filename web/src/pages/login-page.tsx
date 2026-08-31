import { type FormEvent, useState } from "react";
import { LoaderCircle, LogIn } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";

import { APIError } from "@/api/errors";
import { useAuth } from "@/auth-context";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatAPIError } from "@/lib/api-error";

export function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { login } = useAuth();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [totpCode, setTOTPCode] = useState("");
  const [showTOTP, setShowTOTP] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await login({
        username,
        password,
        ...(showTOTP ? { totpCode } : {}),
      });
      navigate("/", { replace: true });
    } catch (cause) {
      if (cause instanceof APIError && cause.code === "totp_required") {
        setShowTOTP(true);
      }
      setError(formatAPIError(cause, t));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="flex min-h-[calc(100svh-3.5rem)] w-full items-center px-4 py-10 sm:px-6">
      <Card className="mx-auto w-full max-w-sm">
        <CardHeader>
          <p className="text-sm font-medium text-muted-foreground uppercase">
            {t("authentication.section")}
          </p>
          <h1 className="mt-1 text-2xl font-semibold">
            {t("authentication.loginTitle")}
          </h1>
        </CardHeader>
        <CardContent>
          <form className="space-y-5" onSubmit={submit}>
            {error ? (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
            <div className="space-y-2">
              <Label htmlFor="username">{t("account.username")}</Label>
              <Input
                id="username"
                name="username"
                autoComplete="username"
                value={username}
                maxLength={64}
                onChange={(event) => setUsername(event.target.value)}
                required
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">{t("account.password")}</Label>
              <Input
                id="password"
                name="password"
                type="password"
                autoComplete="current-password"
                value={password}
                maxLength={128}
                onChange={(event) => setPassword(event.target.value)}
                required
              />
            </div>
            {showTOTP ? (
              <div className="space-y-2">
                <Label htmlFor="totp-code">{t("totp.code")}</Label>
                <Input
                  id="totp-code"
                  name="totp-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  value={totpCode}
                  maxLength={6}
                  onChange={(event) =>
                    setTOTPCode(event.target.value.replace(/\D/g, ""))
                  }
                  required
                  autoFocus
                />
              </div>
            ) : null}
            <Button className="w-full" type="submit" disabled={submitting}>
              {submitting ? (
                <LoaderCircle
                  data-icon="inline-start"
                  aria-hidden="true"
                  className="animate-spin"
                />
              ) : (
                <LogIn data-icon="inline-start" aria-hidden="true" />
              )}
              {t("authentication.login")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
