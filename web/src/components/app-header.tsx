import { useState } from "react";
import { Languages, LogOut, Moon, Radar, Sun } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";

import { useAuth } from "@/auth-context";
import { Button } from "@/components/ui/button";
import { SidebarTrigger } from "@/components/ui/sidebar";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { type SupportedLocale } from "@/i18n";
import { useTheme } from "@/theme";

export function AppHeader({ withSidebar = false }: { withSidebar?: boolean }) {
  const { i18n, t } = useTranslation();
  const { state, changeLocale, logout } = useAuth();
  const { theme, setTheme } = useTheme();
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
    <header className="sticky top-0 z-20 border-b bg-background/95 backdrop-blur">
      <div
        className={
          withSidebar
            ? "flex h-14 items-center justify-between gap-2 px-3 sm:px-4"
            : "flex h-14 w-full items-center justify-between gap-2 px-3 sm:px-6"
        }
      >
        {withSidebar ? (
          <div className="flex min-w-0 items-center gap-2">
            <Tooltip>
              <TooltipTrigger asChild>
                <SidebarTrigger
                  className="size-8"
                  label={t("navigation.toggleSidebar")}
                  aria-label={t("navigation.toggleSidebar")}
                />
              </TooltipTrigger>
              <TooltipContent>{t("navigation.toggleSidebar")}</TooltipContent>
            </Tooltip>
            <Link to="/" className="truncate text-sm font-semibold md:hidden">
              {t("appName")}
            </Link>
          </div>
        ) : (
          <Link
            to="/login"
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
        )}

        <div className="flex min-w-0 items-center gap-0.5 sm:gap-1">
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
