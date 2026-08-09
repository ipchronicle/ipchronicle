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
  createNodeEgress,
  deleteNodeEgress,
  getNetworkObservationSettings,
  getNodeNetwork,
  updateNetworkObservationSettings,
  updateNodeEgress,
} from "@/api/network";
import {
  deleteNode,
  getAgentEnrollment,
  listNodes,
  revokeNode,
  rotateAgentEnrollmentKey,
  startNodeSyncSession,
  stopNodeSyncSession,
  updateNode,
  updateAgentEnrollment,
} from "@/api/nodes";
import {
  createNetworkProxy,
  deleteNetworkProxy,
  listNetworkProxies,
  updateNetworkProxy,
} from "@/api/proxies";
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

vi.mock("@/api/network", () => ({
  createNodeEgress: vi.fn(),
  deleteNodeEgress: vi.fn(),
  getNetworkObservationSettings: vi.fn(),
  getNodeNetwork: vi.fn(),
  updateNetworkObservationSettings: vi.fn(),
  updateNodeEgress: vi.fn(),
}));

vi.mock("@/api/nodes", () => ({
  deleteNode: vi.fn(),
  getAgentEnrollment: vi.fn(),
  listNodes: vi.fn(),
  revokeNode: vi.fn(),
  rotateAgentEnrollmentKey: vi.fn(),
  startNodeSyncSession: vi.fn(),
  stopNodeSyncSession: vi.fn(),
  updateNode: vi.fn(),
  updateAgentEnrollment: vi.fn(),
}));

vi.mock("@/api/proxies", () => ({
  createNetworkProxy: vi.fn(),
  deleteNetworkProxy: vi.fn(),
  listNetworkProxies: vi.fn(),
  updateNetworkProxy: vi.fn(),
}));

const getSessionMock = vi.mocked(getAuthenticatedSession);
const loginMock = vi.mocked(login);
const logoutMock = vi.mocked(logout);
const updateLocaleMock = vi.mocked(updateAccountLocale);
const getSystemStatusMock = vi.mocked(getSystemStatus);
const createEgressMock = vi.mocked(createNodeEgress);
const deleteEgressMock = vi.mocked(deleteNodeEgress);
const getObservationSettingsMock = vi.mocked(getNetworkObservationSettings);
const getNodeNetworkMock = vi.mocked(getNodeNetwork);
const updateObservationSettingsMock = vi.mocked(
  updateNetworkObservationSettings,
);
const updateEgressMock = vi.mocked(updateNodeEgress);
const getEnrollmentMock = vi.mocked(getAgentEnrollment);
const listNodesMock = vi.mocked(listNodes);
const rotateEnrollmentMock = vi.mocked(rotateAgentEnrollmentKey);
const startSyncMock = vi.mocked(startNodeSyncSession);
const stopSyncMock = vi.mocked(stopNodeSyncSession);
const deleteNodeMock = vi.mocked(deleteNode);
const revokeNodeMock = vi.mocked(revokeNode);
const updateNodeMock = vi.mocked(updateNode);
const updateEnrollmentMock = vi.mocked(updateAgentEnrollment);
const createProxyMock = vi.mocked(createNetworkProxy);
const deleteProxyMock = vi.mocked(deleteNetworkProxy);
const listProxiesMock = vi.mocked(listNetworkProxies);
const updateProxyMock = vi.mocked(updateNetworkProxy);

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
  configSchemaVersion: 8,
  historySchemaVersion: 2,
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
    createEgressMock.mockReset();
    deleteEgressMock.mockReset();
    getObservationSettingsMock.mockReset();
    getObservationSettingsMock.mockResolvedValue({
      ipv4Services: ["https://one.example/ip", "https://two.example/ip"],
      ipv6Services: [
        "https://six-one.example/ip",
        "https://six-two.example/ip",
      ],
      updatedAt: "2026-08-09T06:00:00Z",
    });
    getNodeNetworkMock.mockReset();
    updateObservationSettingsMock.mockReset();
    updateEgressMock.mockReset();
    getEnrollmentMock.mockReset();
    listNodesMock.mockReset();
    rotateEnrollmentMock.mockReset();
    startSyncMock.mockReset();
    stopSyncMock.mockReset();
    deleteNodeMock.mockReset();
    revokeNodeMock.mockReset();
    updateNodeMock.mockReset();
    updateEnrollmentMock.mockReset();
    createProxyMock.mockReset();
    deleteProxyMock.mockReset();
    listProxiesMock.mockReset();
    listProxiesMock.mockResolvedValue([]);
    updateProxyMock.mockReset();
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
        capabilities: ["control-v1", "sync-wakeup-v1"],
        desiredConfigurationRevision: 1,
        appliedConfigurationRevision: 1,
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
    updateNodeMock.mockResolvedValue({
      id: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      name: "edge-1",
      hostname: "edge-1",
      status: "disabled",
      enabled: false,
      agentVersion: "0.1.0",
      operatingSystem: "linux",
      architecture: "amd64",
      capabilities: ["control-v1", "sync-wakeup-v1"],
      desiredConfigurationRevision: 2,
      appliedConfigurationRevision: 1,
      configurationStatus: "pending",
      registeredAt: "2026-08-07T12:00:00Z",
      lastSeenAt: "2026-08-07T12:01:00Z",
    });
    startSyncMock.mockResolvedValue({
      id: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      name: "edge-1",
      hostname: "edge-1",
      status: "online",
      enabled: true,
      agentVersion: "0.1.0",
      operatingSystem: "linux",
      architecture: "amd64",
      capabilities: ["control-v1", "sync-wakeup-v1"],
      desiredConfigurationRevision: 1,
      appliedConfigurationRevision: 1,
      configurationStatus: "current",
      syncStatus: "pending",
      syncExpiresAt: "2026-08-07T12:11:00Z",
      registeredAt: "2026-08-07T12:00:00Z",
      lastSeenAt: "2026-08-07T12:01:00Z",
    });
    stopSyncMock.mockResolvedValue({
      id: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      name: "edge-1",
      hostname: "edge-1",
      status: "online",
      enabled: true,
      agentVersion: "0.1.0",
      operatingSystem: "linux",
      architecture: "amd64",
      capabilities: ["control-v1", "sync-wakeup-v1"],
      desiredConfigurationRevision: 1,
      appliedConfigurationRevision: 1,
      configurationStatus: "current",
      registeredAt: "2026-08-07T12:00:00Z",
      lastSeenAt: "2026-08-07T12:01:00Z",
    });
    renderApplication("/nodes");

    expect(
      await screen.findByRole("heading", { name: "Nodes" }),
    ).toBeInTheDocument();
    expect((await screen.findAllByText("edge-1")).length).toBeGreaterThan(0);
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
    fireEvent.click(
      screen.getAllByRole("button", { name: "Start temporary sync" })[0],
    );
    await waitFor(() =>
      expect(startSyncMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        session.csrfToken,
      ),
    );
    expect(screen.getAllByText("Waiting for Agent").length).toBeGreaterThan(0);
    fireEvent.click(
      screen.getAllByRole("button", { name: "Stop temporary sync" })[0],
    );
    await waitFor(() =>
      expect(stopSyncMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        session.csrfToken,
      ),
    );
    fireEvent.click(screen.getAllByRole("button", { name: "Pause node" })[0]);
    await waitFor(() =>
      expect(updateNodeMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        false,
        session.csrfToken,
      ),
    );
    expect(screen.getAllByText("Disabled").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Pending · 1/2").length).toBeGreaterThan(0);
  });

  it("shows durable egresses and temporary IPv6 candidates", async () => {
    getSessionMock.mockResolvedValue(session);
    listProxiesMock.mockResolvedValue([
      {
        id: "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16",
        name: "Primary proxy",
        scheme: "socks5",
        host: "proxy.example.test",
        port: 1080,
        passwordConfigured: true,
        createdAt: "2026-08-09T06:00:00Z",
        updatedAt: "2026-08-09T06:00:00Z",
      },
    ]);
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
        capabilities: ["network-inventory-v1"],
        desiredConfigurationRevision: 2,
        appliedConfigurationRevision: 2,
        configurationStatus: "current",
        registeredAt: "2026-08-09T06:00:00Z",
      },
    ]);
    getNodeNetworkMock.mockResolvedValue({
      inventoryReceivedAt: "2026-08-09T06:01:00Z",
      inventory: {
        capturedAt: "2026-08-09T06:01:00Z",
        interfaces: [{ name: "eth0", index: 2, up: true, loopback: false }],
        addresses: [
          {
            interfaceName: "eth0",
            address: "10.0.0.5",
            prefixLength: 24,
            family: "ipv4",
            scope: "private",
            temporary: false,
            tentative: false,
            deprecated: false,
            duplicate: false,
          },
          {
            interfaceName: "eth0",
            address: "2001:4860::99",
            prefixLength: 64,
            family: "ipv6",
            scope: "global",
            temporary: true,
            tentative: false,
            deprecated: false,
            duplicate: false,
          },
        ],
        routes: [
          {
            interfaceName: "eth0",
            family: "ipv4",
            destination: "0.0.0.0/0",
            gateway: "10.0.0.1",
            metric: 100,
            default: true,
          },
        ],
      },
      egresses: [
        {
          id: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          nodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
          name: "default-ipv4",
          kind: "default",
          family: "ipv4",
          enabled: true,
          available: true,
          automatic: true,
          lightweightIntervalSeconds: 600,
          probeOnAddressChange: true,
        },
      ],
      candidates: [
        {
          kind: "source",
          family: "ipv6",
          interfaceName: "eth0",
          sourceAddress: "2001:4860::99",
          scope: "global",
          temporary: true,
          eligible: false,
          unavailableReason: "temporary-address",
        },
      ],
      addressStates: [],
      addressEvents: [],
      addressGaps: [],
    });

    renderApplication("/nodes/7289cfa3-a75d-4a3f-ac06-8f1074446a85/network");

    expect(
      await screen.findByRole("heading", { name: "edge-1" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Default IPv4").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Temporary IPv6").length).toBeGreaterThan(0);
    expect(screen.getByText("Primary proxy · SOCKS5")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enable path" })).toBeDisabled();
  });

  it("shows replace-only proxy credentials in network settings", async () => {
    getSessionMock.mockResolvedValue(session);
    listProxiesMock.mockResolvedValue([
      {
        id: "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16",
        name: "Primary proxy",
        scheme: "socks5",
        host: "proxy.example.test",
        port: 1080,
        username: "probe-user",
        passwordConfigured: true,
        createdAt: "2026-08-09T06:00:00Z",
        updatedAt: "2026-08-09T06:00:00Z",
      },
    ]);

    renderApplication("/settings/network");

    expect(
      await screen.findByRole("heading", { name: "Network probes" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("Password configured")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Replace password" }),
    ).toBeDisabled();
    expect(
      screen.queryByDisplayValue("retained-secret"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Network probes" }),
    ).toHaveAttribute("aria-current", "page");
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
