import { Languages, Moon, Radar, Sun } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { setLocale, type SupportedLocale } from "@/i18n";
import { useTheme } from "@/theme";

export function AppHeader() {
  const { i18n, t } = useTranslation();
  const { theme, setTheme } = useTheme();
  const currentLocale: SupportedLocale =
    i18n.resolvedLanguage === "zh-CN" ? "zh-CN" : "en";
  const nextLocale: SupportedLocale =
    currentLocale === "zh-CN" ? "en" : "zh-CN";
  const nextTheme = theme === "dark" ? "light" : "dark";

  return (
    <header className="border-b bg-background/95">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4 sm:px-6">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-foreground text-background">
            <Radar aria-hidden="true" className="size-4.5" />
          </span>
          <span className="truncate text-sm font-semibold">{t("appName")}</span>
        </div>

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void setLocale(nextLocale)}
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
        </div>
      </div>
    </header>
  );
}
