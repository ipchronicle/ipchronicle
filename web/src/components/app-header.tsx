import { type ReactNode, useState } from "react";
import {
  CircleUserRound,
  Gauge,
  Languages,
  LogOut,
  Moon,
  Radar,
  Sun,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router";

import { useAuth } from "@/auth-context";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { type SupportedLocale } from "@/i18n";
import { useTheme } from "@/theme";

export function AppHeader() {
  const { i18n, t } = useTranslation();
  const { state, changeLocale, logout } = useAuth();
  const { theme, setTheme } = useTheme();
  const location = useLocation();
  const [actionError, setActionError] = useState(false);
  const currentLocale: SupportedLocale =
    i18n.resolvedLanguage === "zh-CN" ? "zh-CN" : "en";
  const nextLocale: SupportedLocale =
    currentLocale === "zh-CN" ? "en" : "zh-CN";
  const nextTheme = theme === "dark" ? "light" : "dark";

  async function switchLocale() {
    setActionError(false);
    try {
      await changeLocale(nextLocale);
    } catch {
      setActionError(true);
    }
  }

  async function signOut() {
    setActionError(false);
    try {
      await logout();
    } catch {
      setActionError(true);
    }
  }

  return (
    <header className="border-b bg-background/95">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between gap-2 px-3 sm:px-6">
        <Link
          to={state.status === "authenticated" ? "/" : "/login"}
          className="flex min-w-0 items-center gap-2.5"
          aria-label={t("appName")}
        >
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-foreground text-background">
            <Radar aria-hidden="true" className="size-4.5" />
          </span>
          <span className="hidden truncate text-sm font-semibold sm:inline">
            {t("appName")}
          </span>
        </Link>

        <div className="flex min-w-0 items-center gap-0.5 sm:gap-1">
          {state.status === "authenticated" ? (
            <>
              <HeaderLink
                to="/"
                active={
                  location.pathname === "/" ||
                  location.pathname === "/system/status"
                }
                label={t("navigation.systemStatus")}
                icon={<Gauge aria-hidden="true" />}
              />
              <HeaderLink
                to="/settings/account"
                active={location.pathname === "/settings/account"}
                label={t("navigation.account")}
                icon={<CircleUserRound aria-hidden="true" />}
              />
            </>
          ) : null}
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void switchLocale()}
            aria-label={t("language.switch")}
          >
            <Languages data-icon="inline-start" aria-hidden="true" />
            {t("language.current")}
          </Button>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                onClick={() => setTheme(nextTheme)}
                aria-label={t(`theme.${nextTheme}`)}
              >
                {theme === "dark" ? (
                  <Sun aria-hidden="true" />
                ) : (
                  <Moon aria-hidden="true" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t(`theme.${nextTheme}`)}</TooltipContent>
          </Tooltip>
          {state.status === "authenticated" ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => void signOut()}
                  aria-label={t("authentication.logout")}
                >
                  <LogOut aria-hidden="true" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("authentication.logout")}</TooltipContent>
            </Tooltip>
          ) : null}
        </div>
      </div>
      {actionError ? (
        <p
          role="alert"
          className="border-t px-4 py-2 text-center text-xs text-destructive"
        >
          {t("errors.actionFailed")}
        </p>
      ) : null}
    </header>
  );
}

function HeaderLink({
  to,
  active,
  label,
  icon,
}: {
  to: string;
  active: boolean;
  label: string;
  icon: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button asChild variant={active ? "secondary" : "ghost"} size="sm">
          <Link to={to} aria-label={label}>
            {icon}
            <span className="hidden lg:inline">{label}</span>
          </Link>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}
