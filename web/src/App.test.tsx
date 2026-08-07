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
import {
  getAgentEnrollment,
  listNodes,
  rotateAgentEnrollmentKey,
  updateAgentEnrollment,
} from "@/api/nodes";
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

vi.mock("@/api/nodes", () => ({
  getAgentEnrollment: vi.fn(),
  listNodes: vi.fn(),
  rotateAgentEnrollmentKey: vi.fn(),
  updateAgentEnrollment: vi.fn(),
}));

const getSessionMock = vi.mocked(getAuthenticatedSession);
const loginMock = vi.mocked(login);
const logoutMock = vi.mocked(logout);
const updateLocaleMock = vi.mocked(updateAccountLocale);
const getSystemStatusMock = vi.mocked(getSystemStatus);
const getEnrollmentMock = vi.mocked(getAgentEnrollment);
const listNodesMock = vi.mocked(listNodes);
const rotateEnrollmentMock = vi.mocked(rotateAgentEnrollmentKey);
const updateEnrollmentMock = vi.mocked(updateAgentEnrollment);

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
  configSchemaVersion: 2,
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
    getEnrollmentMock.mockReset();
    listNodesMock.mockReset();
    rotateEnrollmentMock.mockReset();
    updateEnrollmentMock.mockReset();
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

  it("shows the installation command and registered node state", async () => {
    getSessionMock.mockResolvedValue(session);
    getEnrollmentMock.mockResolvedValue({
      enabled: true,
      hasKey: true,
      installationCommand:
        "curl https://example.test/install-agent.sh | sh -s -- --registration-key secret",
      rotatedAt: "2026-08-07T12:00:00Z",
    });
    listNodesMock.mockResolvedValue([
      {
        id: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        name: "edge-1",
        hostname: "edge-1",
        status: "online",
        enabled: true,
        agentVersion: "0.1.0",
        operatingSystem: "linux",
        architecture: "amd64",
        capabilities: ["control-v1"],
        desiredConfigurationRevision: 0,
        appliedConfigurationRevision: 0,
        configurationStatus: "current",
        registeredAt: "2026-08-07T12:00:00Z",
        lastSeenAt: "2026-08-07T12:01:00Z",
      },
    ]);
    updateEnrollmentMock.mockResolvedValue({
      enabled: false,
      hasKey: true,
      installationCommand:
        "curl https://example.test/install-agent.sh | sh -s -- --registration-key secret",
      rotatedAt: "2026-08-07T12:00:00Z",
    });
    renderApplication("/nodes");

    expect(
      await screen.findByRole("heading", { name: "Nodes" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("edge-1").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Online").length).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: "Nodes" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    fireEvent.click(
      screen.getByRole("switch", { name: "Allow automatic registration" }),
    );
    await waitFor(() =>
      expect(updateEnrollmentMock).toHaveBeenCalledWith(
        false,
        session.csrfToken,
      ),
    );
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
