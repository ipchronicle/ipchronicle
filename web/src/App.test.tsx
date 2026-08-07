import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  getAuthenticatedSession,
  login,
  logout,
  updateAccountLocale,
  type AuthenticatedSession,
} from "@/api/auth";
import { getSystemStatus } from "@/api/system";
import App from "@/App";
import { AuthProvider } from "@/auth-context";
import { TooltipProvider } from "@/components/ui/tooltip";
import i18n from "@/i18n";

vi.mock("@/api/auth", async (importOriginal) => {
  const original = await importOriginal<typeof import("@/api/auth")>();
  return {
    ...original,
    getAuthenticatedSession: vi.fn(),
    login: vi.fn(),
    logout: vi.fn(),
    updateAccountLocale: vi.fn(),
  };
});

vi.mock("@/api/system", () => ({
  getSystemStatus: vi.fn(),
}));

const getSessionMock = vi.mocked(getAuthenticatedSession);
const loginMock = vi.mocked(login);
const logoutMock = vi.mocked(logout);
const updateLocaleMock = vi.mocked(updateAccountLocale);
const getSystemStatusMock = vi.mocked(getSystemStatus);

const session: AuthenticatedSession = {
  account: {
    username: "admin",
    locale: "en",
    usesDefaultCredentials: true,
    totpEnabled: false,
  },
  csrfToken: "test-csrf-token-value",
  expiresAt: "2026-09-06T00:00:00Z",
};

const healthyStatus = {
  service: "ipchronicle-center" as const,
  status: "ok" as const,
  version: "0.0.0-test",
  configSchemaVersion: 1,
  historySchemaVersion: 1,
  transportSecurity: "http" as const,
  transportWarning: true,
  externalOriginConfigured: false,
  trustedProxyConfigured: false,
};

describe("administrator application", () => {
  beforeEach(async () => {
    window.localStorage.clear();
    document.documentElement.className = "";
    await i18n.changeLanguage("en");
    getSessionMock.mockReset();
    loginMock.mockReset();
    logoutMock.mockReset();
    updateLocaleMock.mockReset();
    getSystemStatusMock.mockReset();
  });

  it("routes an anonymous browser to the real login form", async () => {
    getSessionMock.mockResolvedValue(null);
    renderApplication("/");

    expect(
      await screen.findByRole("heading", { name: "Sign in" }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("admin");
    expect(screen.getByLabelText("Password")).toBeInTheDocument();
  });

  it("authenticates and renders persisted system status", async () => {
    getSessionMock.mockResolvedValue(null);
    loginMock.mockResolvedValue(session);
    getSystemStatusMock.mockResolvedValue(healthyStatus);
    renderApplication("/login");

    fireEvent.change(await screen.findByLabelText("Password"), {
      target: { value: "admin" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    expect(
      await screen.findByRole("heading", { name: "System status" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("navigation", { name: "Primary navigation" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "System status" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    const sidebar = document.querySelector('[data-slot="sidebar"][data-state]');
    expect(sidebar).toHaveAttribute("data-state", "expanded");
    fireEvent.click(screen.getByRole("button", { name: "Toggle sidebar" }));
    expect(sidebar).toHaveAttribute("data-state", "collapsed");
    expect(screen.getByText("Operational")).toBeInTheDocument();
    expect(
      screen.getByText("Default credentials are still active"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Browser connection uses HTTP"),
    ).toBeInTheDocument();
  });

  it("shows a recoverable status API failure", async () => {
    getSessionMock.mockResolvedValue(session);
    getSystemStatusMock
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(healthyStatus);
    renderApplication("/");

    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Operational")).toBeInTheDocument();
  });

  it("persists locale and changes theme without a reload", async () => {
    getSessionMock.mockResolvedValue(session);
    getSystemStatusMock.mockResolvedValue(healthyStatus);
    updateLocaleMock.mockResolvedValue({ ...session.account, locale: "zh-CN" });
    renderApplication("/");

    await screen.findByRole("heading", { name: "System status" });
    fireEvent.click(
      await screen.findByRole("button", {
        name: "Switch to Simplified Chinese",
      }),
    );
    await waitFor(() => expect(updateLocaleMock).toHaveBeenCalledWith("zh-CN"));
    expect(
      await screen.findByRole("heading", { name: "系统状态" }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "使用深色主题" }));
    expect(document.documentElement).toHaveClass("dark");
  });
});

function renderApplication(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <TooltipProvider>
          <App />
        </TooltipProvider>
      </AuthProvider>
    </MemoryRouter>,
  );
}
