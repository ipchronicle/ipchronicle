import {
  BellRing,
  CircleUserRound,
  Database,
  Gauge,
  History,
  Network,
  Radar,
  Server,
  Settings2,
} from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router";

import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";

export function AppSidebar() {
  const { t } = useTranslation();
  const location = useLocation();

  return (
    <Sidebar
      collapsible="icon"
      mobileTitle={t("appName")}
      mobileDescription={t("navigation.mobileDescription")}
    >
      <SidebarHeader className="border-b border-sidebar-border">
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              size="lg"
              className="h-10"
              tooltip={t("appName")}
            >
              <SidebarLink to="/" aria-label={t("appName")}>
                <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground">
                  <Radar aria-hidden="true" className="size-4.5" />
                </span>
                <span className="font-semibold">{t("appName")}</span>
              </SidebarLink>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <nav aria-label={t("navigation.menu")}>
          <SidebarGroup>
            <SidebarGroupLabel>{t("navigation.menu")}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={location.pathname === "/"}
                    tooltip={t("navigation.systemStatus")}
                  >
                    <SidebarLink
                      to="/"
                      aria-current={
                        location.pathname === "/" ? "page" : undefined
                      }
                    >
                      <Gauge aria-hidden="true" />
                      <span>{t("navigation.systemStatus")}</span>
                    </SidebarLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={location.pathname.startsWith("/nodes")}
                    tooltip={t("navigation.nodes")}
                  >
                    <SidebarLink
                      to="/nodes"
                      aria-current={
                        location.pathname.startsWith("/nodes")
                          ? "page"
                          : undefined
                      }
                    >
                      <Server aria-hidden="true" />
                      <span>{t("navigation.nodes")}</span>
                    </SidebarLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={
                      location.pathname === "/history" ||
                      location.pathname.startsWith("/history/")
                    }
                    tooltip={t("navigation.history")}
                  >
                    <SidebarLink
                      to="/history"
                      aria-current={
                        location.pathname === "/history" ||
                        location.pathname.startsWith("/history/")
                          ? "page"
                          : undefined
                      }
                    >
                      <History aria-hidden="true" />
                      <span>{t("navigation.history")}</span>
                    </SidebarLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={location.pathname === "/notifications"}
                    tooltip={t("navigation.notifications")}
                  >
                    <SidebarLink
                      to="/notifications"
                      aria-current={
                        location.pathname === "/notifications"
                          ? "page"
                          : undefined
                      }
                    >
                      <BellRing aria-hidden="true" />
                      <span>{t("navigation.notifications")}</span>
                    </SidebarLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          <SidebarGroup>
            <SidebarGroupLabel>{t("navigation.settings")}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={location.pathname === "/settings/system"}
                    tooltip={t("navigation.systemSettings")}
                  >
                    <SidebarLink
                      to="/settings/system"
                      aria-current={
                        location.pathname === "/settings/system"
                          ? "page"
                          : undefined
                      }
                    >
                      <Settings2 aria-hidden="true" />
                      <span>{t("navigation.systemSettings")}</span>
                    </SidebarLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={location.pathname === "/settings/network"}
                    tooltip={t("navigation.networkSettings")}
                  >
                    <SidebarLink
                      to="/settings/network"
                      aria-current={
                        location.pathname === "/settings/network"
                          ? "page"
                          : undefined
                      }
                    >
                      <Network aria-hidden="true" />
                      <span>{t("navigation.networkSettings")}</span>
                    </SidebarLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={location.pathname === "/settings/history"}
                    tooltip={t("navigation.historySettings")}
                  >
                    <SidebarLink
                      to="/settings/history"
                      aria-current={
                        location.pathname === "/settings/history"
                          ? "page"
                          : undefined
                      }
                    >
                      <Database aria-hidden="true" />
                      <span>{t("navigation.historySettings")}</span>
                    </SidebarLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    asChild
                    isActive={location.pathname === "/settings/account"}
                    tooltip={t("navigation.account")}
                  >
                    <SidebarLink
                      to="/settings/account"
                      aria-current={
                        location.pathname === "/settings/account"
                          ? "page"
                          : undefined
                      }
                    >
                      <CircleUserRound aria-hidden="true" />
                      <span>{t("navigation.account")}</span>
                    </SidebarLink>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </nav>
      </SidebarContent>
    </Sidebar>
  );
}

function SidebarLink({ onClick, ...props }: React.ComponentProps<typeof Link>) {
  const { isMobile, setOpenMobile } = useSidebar();

  return (
    <Link
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented && isMobile) {
          setOpenMobile(false);
        }
      }}
      {...props}
    />
  );
}
