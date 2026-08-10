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
  createAgentUpdateTasks,
  getAgentUpdateState,
  updateReleaseChannel,
} from "@/api/updates";
import {
  cleanupHistory,
  compareProbeSnapshots,
  listHistoryAddressEvents,
  listHistoryFormatEvents,
  listHistoryProbeGaps,
  listHistoryProbeSnapshots,
  setProbeSnapshotStarred,
  updateHistoryRetention,
} from "@/api/history";
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
import {
  createNotificationRule,
  createNotificationSender,
  createNotificationTestDelivery,
  deleteNotificationRule,
  deleteNotificationSender,
  listNotificationDeliveries,
  listNotificationRules,
  listNotificationSenders,
  updateNotificationRule,
  updateNotificationSender,
} from "@/api/notifications";
import {
  createCompleteProbeTask,
  getHistoryState,
  getNodeProbe,
  getProbeRun,
  getProbeSnapshot,
  resetHistory,
  updateNodeProbeSettings,
} from "@/api/probes";
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

vi.mock("@/api/updates", () => ({
  createAgentUpdateTasks: vi.fn(),
  getAgentUpdateState: vi.fn(),
  updateReleaseChannel: vi.fn(),
}));

vi.mock("@/api/history", () => ({
  cleanupHistory: vi.fn(),
  compareProbeSnapshots: vi.fn(),
  listHistoryAddressEvents: vi.fn(),
  listHistoryFormatEvents: vi.fn(),
  listHistoryProbeGaps: vi.fn(),
  listHistoryProbeSnapshots: vi.fn(),
  setProbeSnapshotStarred: vi.fn(),
  updateHistoryRetention: vi.fn(),
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

vi.mock("@/api/notifications", () => ({
  createNotificationRule: vi.fn(),
  createNotificationSender: vi.fn(),
  createNotificationTestDelivery: vi.fn(),
  deleteNotificationRule: vi.fn(),
  deleteNotificationSender: vi.fn(),
  listNotificationDeliveries: vi.fn(),
  listNotificationRules: vi.fn(),
  listNotificationSenders: vi.fn(),
  updateNotificationRule: vi.fn(),
  updateNotificationSender: vi.fn(),
}));

vi.mock("@/api/probes", () => ({
  createCompleteProbeTask: vi.fn(),
  getHistoryState: vi.fn(),
  getNodeProbe: vi.fn(),
  getProbeRun: vi.fn(),
  getProbeSnapshot: vi.fn(),
  resetHistory: vi.fn(),
  updateNodeProbeSettings: vi.fn(),
}));

const getSessionMock = vi.mocked(getAuthenticatedSession);
const loginMock = vi.mocked(login);
const logoutMock = vi.mocked(logout);
const updateLocaleMock = vi.mocked(updateAccountLocale);
const getSystemStatusMock = vi.mocked(getSystemStatus);
const createAgentUpdateTasksMock = vi.mocked(createAgentUpdateTasks);
const getAgentUpdateStateMock = vi.mocked(getAgentUpdateState);
const updateReleaseChannelMock = vi.mocked(updateReleaseChannel);
const cleanupHistoryMock = vi.mocked(cleanupHistory);
const compareSnapshotsMock = vi.mocked(compareProbeSnapshots);
const listHistoryAddressesMock = vi.mocked(listHistoryAddressEvents);
const listHistoryFormatsMock = vi.mocked(listHistoryFormatEvents);
const listHistoryGapsMock = vi.mocked(listHistoryProbeGaps);
const listHistorySnapshotsMock = vi.mocked(listHistoryProbeSnapshots);
const setSnapshotStarredMock = vi.mocked(setProbeSnapshotStarred);
const updateRetentionMock = vi.mocked(updateHistoryRetention);
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
const createNotificationRuleMock = vi.mocked(createNotificationRule);
const createNotificationSenderMock = vi.mocked(createNotificationSender);
const createNotificationTestDeliveryMock = vi.mocked(
  createNotificationTestDelivery,
);
const deleteNotificationRuleMock = vi.mocked(deleteNotificationRule);
const deleteNotificationSenderMock = vi.mocked(deleteNotificationSender);
const listNotificationDeliveriesMock = vi.mocked(listNotificationDeliveries);
const listNotificationRulesMock = vi.mocked(listNotificationRules);
const listNotificationSendersMock = vi.mocked(listNotificationSenders);
const updateNotificationRuleMock = vi.mocked(updateNotificationRule);
const updateNotificationSenderMock = vi.mocked(updateNotificationSender);
const createProbeTaskMock = vi.mocked(createCompleteProbeTask);
const getHistoryStateMock = vi.mocked(getHistoryState);
const getNodeProbeMock = vi.mocked(getNodeProbe);
const getProbeRunMock = vi.mocked(getProbeRun);
const getProbeSnapshotMock = vi.mocked(getProbeSnapshot);
const resetHistoryMock = vi.mocked(resetHistory);
const updateProbeSettingsMock = vi.mocked(updateNodeProbeSettings);

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
  sourceRevision: "1111111111111111111111111111111111111111",
  configSchemaVersion: 8,
  historySchemaVersion: 2,
  transportSecurity: "http" as const,
  transportWarning: true,
  externalOriginConfigured: false,
  trustedProxyConfigured: false,
};

const agentUpdateState = {
  channel: "stable" as const,
  currentVersion: "0.1.0",
  currentRevision: "1111111111111111111111111111111111111111",
  checkedAt: "2026-08-10T08:00:00Z",
  tasks: [],
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
    createAgentUpdateTasksMock.mockReset();
    getAgentUpdateStateMock.mockReset();
    getAgentUpdateStateMock.mockResolvedValue(agentUpdateState);
    updateReleaseChannelMock.mockReset();
    cleanupHistoryMock.mockReset();
    compareSnapshotsMock.mockReset();
    listHistoryAddressesMock.mockReset();
    listHistoryAddressesMock.mockResolvedValue({
      events: [],
      gaps: [],
      total: 0,
      gapTotal: 0,
    });
    listHistoryFormatsMock.mockReset();
    listHistoryFormatsMock.mockResolvedValue({ items: [], total: 0 });
    listHistoryGapsMock.mockReset();
    listHistoryGapsMock.mockResolvedValue({ items: [], total: 0 });
    listHistorySnapshotsMock.mockReset();
    listHistorySnapshotsMock.mockResolvedValue({ items: [], total: 0 });
    setSnapshotStarredMock.mockReset();
    updateRetentionMock.mockReset();
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
    createNotificationRuleMock.mockReset();
    createNotificationSenderMock.mockReset();
    createNotificationTestDeliveryMock.mockReset();
    deleteNotificationRuleMock.mockReset();
    deleteNotificationSenderMock.mockReset();
    listNotificationDeliveriesMock.mockReset();
    listNotificationDeliveriesMock.mockResolvedValue({
      items: [],
      page: 1,
      pageSize: 25,
      totalItems: 0,
      totalPages: 0,
    });
    listNotificationRulesMock.mockReset();
    listNotificationRulesMock.mockResolvedValue([]);
    listNotificationSendersMock.mockReset();
    listNotificationSendersMock.mockResolvedValue([]);
    updateNotificationRuleMock.mockReset();
    updateNotificationSenderMock.mockReset();
    createProbeTaskMock.mockReset();
    getHistoryStateMock.mockReset();
    getNodeProbeMock.mockReset();
    getProbeRunMock.mockReset();
    getProbeSnapshotMock.mockReset();
    resetHistoryMock.mockReset();
    updateProbeSettingsMock.mockReset();
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
    expect(await screen.findByText("Operational")).toBeInTheDocument();
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

  it("shows release metadata and switches the unified release channel", async () => {
    getSessionMock.mockResolvedValue(session);
    getAgentUpdateStateMock.mockResolvedValue({
      ...agentUpdateState,
      availableRelease: {
        version: "0.1.1",
        tag: "v0.1.1",
        channel: "stable",
        revision: "2222222222222222222222222222222222222222",
        publishedAt: "2026-08-10T07:00:00Z",
        agentCapabilities: ["agent-update-v1"],
      },
    });
    updateReleaseChannelMock.mockResolvedValue({
      ...agentUpdateState,
      channel: "rc",
      availableRelease: {
        version: "0.2.0-rc.1",
        tag: "v0.2.0-rc.1",
        channel: "rc",
        revision: "3333333333333333333333333333333333333333",
        publishedAt: "2026-08-10T09:00:00Z",
        agentCapabilities: ["agent-update-v1"],
      },
    });
    renderApplication("/settings/system");

    expect(
      await screen.findByRole("heading", { name: "System" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("0.1.1")).toBeInTheDocument();
    expect(
      screen.getByText("2222222222222222222222222222222222222222"),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("combobox", { name: "Discovery channel" }),
    );
    fireEvent.click(
      await screen.findByRole("option", { name: "Release candidate" }),
    );
    await waitFor(() =>
      expect(updateReleaseChannelMock).toHaveBeenCalledWith(
        "rc",
        session.csrfToken,
      ),
    );
    expect(screen.getByText("0.2.0-rc.1")).toBeInTheDocument();
  });

  it("opens notification rules and follows test delivery status", async () => {
    getSessionMock.mockResolvedValue(session);
    const sender = {
      id: "5fca3887-f7ef-4988-a3d0-75e8682e7775",
      name: "Local webhook",
      kind: "webhook" as const,
      enabled: true,
      webhook: {
        url: "http://127.0.0.1:19090/notify",
        headerNames: ["Authorization"],
      },
      createdAt: "2026-08-09T12:00:00Z",
      updatedAt: "2026-08-09T12:00:00Z",
    };
    const delivery = {
      id: "ee9a2ab0-c091-45a0-89d1-ac06bc1979ec",
      eventId: "98efe8ab-b72a-47ef-8907-15383adb3589",
      senderId: sender.id,
      senderName: sender.name,
      senderKind: sender.kind,
      eventType: "test",
      test: true,
      status: "pending" as const,
      attemptCount: 0,
      matchedRuleIds: [],
      event: { type: "test" },
      title: "IPChronicle test notification",
      body: "Test",
      createdAt: "2026-08-09T12:01:00Z",
      updatedAt: "2026-08-09T12:01:00Z",
    };
    listNotificationSendersMock.mockResolvedValue([sender]);
    listNodesMock.mockResolvedValue([probeTestNode]);
    createNotificationTestDeliveryMock.mockResolvedValue(delivery);
    listNotificationDeliveriesMock.mockResolvedValue({
      items: [delivery],
      page: 1,
      pageSize: 25,
      totalItems: 1,
      totalPages: 1,
    });

    renderApplication("/notifications");

    expect(
      await screen.findByRole("heading", { name: "Notifications" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Notifications" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    fireEvent.click(await screen.findByRole("button", { name: "Send test" }));
    await waitFor(() =>
      expect(createNotificationTestDeliveryMock).toHaveBeenCalledWith(
        sender.id,
        session.csrfToken,
      ),
    );
    expect(await screen.findByText("Pending")).toBeInTheDocument();
    fireEvent.mouseDown(screen.getByRole("tab", { name: "Rules" }), {
      button: 0,
      ctrlKey: false,
    });
    fireEvent.click(await screen.findByRole("button", { name: "Add rule" }));
    expect(screen.getByLabelText("Event")).toBeInTheDocument();
    expect(screen.getByLabelText("Node")).toBeInTheDocument();
    expect(screen.getByLabelText("Network egress")).toBeDisabled();
  });

  it("creates a webhook sender without rendering its secret header", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([]);
    createNotificationSenderMock.mockResolvedValue({
      id: "5fca3887-f7ef-4988-a3d0-75e8682e7775",
      name: "Local webhook",
      kind: "webhook",
      enabled: true,
      webhook: {
        url: "http://127.0.0.1:19090/notify",
        headerNames: ["Authorization"],
      },
      createdAt: "2026-08-09T12:00:00Z",
      updatedAt: "2026-08-09T12:00:00Z",
    });

    renderApplication("/notifications");

    fireEvent.click(await screen.findByRole("button", { name: "Add sender" }));
    fireEvent.click(screen.getByLabelText("Sender type"));
    fireEvent.click(await screen.findByRole("option", { name: "Webhook" }));
    fireEvent.change(screen.getByLabelText("Name"), {
      target: { value: "Local webhook" },
    });
    fireEvent.change(screen.getByLabelText("Webhook URL"), {
      target: { value: "http://127.0.0.1:19090/notify" },
    });
    fireEvent.change(screen.getByLabelText("HTTP headers"), {
      target: { value: "Authorization: Bearer local-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(createNotificationSenderMock).toHaveBeenCalledWith(
        {
          name: "Local webhook",
          kind: "webhook",
          enabled: true,
          webhook: {
            url: "http://127.0.0.1:19090/notify",
            headers: { Authorization: "Bearer local-secret" },
          },
        },
        session.csrfToken,
      ),
    );
    expect(
      screen.queryByDisplayValue("Authorization: Bearer local-secret"),
    ).not.toBeInTheDocument();
    expect(screen.getByText("Local webhook")).toBeInTheDocument();
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
      await screen.findByRole("switch", {
        name: "Allow automatic registration",
      }),
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

  it("filters updateable nodes and reports partial grouped update results", async () => {
    getSessionMock.mockResolvedValue(session);
    getEnrollmentMock.mockResolvedValue({
      enabled: true,
      hasKey: true,
      installationCommand: "install-agent",
      rotatedAt: "2026-08-10T07:00:00Z",
    });
    const firstNode = updateTestNode("edge-1", "1");
    const secondNode = updateTestNode("edge-2", "2");
    listNodesMock.mockResolvedValue([firstNode, secondNode]);
    getAgentUpdateStateMock.mockResolvedValue({
      ...agentUpdateState,
      availableRelease: {
        version: "0.2.0",
        tag: "v0.2.0",
        channel: "stable",
        revision: "2222222222222222222222222222222222222222",
        publishedAt: "2026-08-10T07:00:00Z",
        agentCapabilities: ["agent-update-v1"],
      },
    });
    createAgentUpdateTasksMock.mockResolvedValue({
      targetVersion: "0.2.0",
      items: [
        {
          nodeId: firstNode.id,
          accepted: true,
          task: {
            id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
            nodeId: firstNode.id,
            targetVersion: "0.2.0",
            status: "pending",
            createdAt: "2026-08-10T08:01:00Z",
            expiresAt: "2026-08-10T08:03:00Z",
            offline: false,
          },
        },
        {
          nodeId: secondNode.id,
          accepted: false,
          error: "agent_update_task_slot_occupied",
        },
      ],
    });
    renderApplication("/nodes");

    await screen.findByRole("heading", { name: "Nodes" });
    expect(await screen.findAllByText("Source 111111111111")).toHaveLength(4);
    fireEvent.click(screen.getByRole("switch", { name: "Updates available" }));
    expect(screen.getAllByText("Update available: 0.2.0")).toHaveLength(4);
    fireEvent.click(
      screen.getAllByRole("checkbox", {
        name: "Select edge-1 for Agent update",
      })[0],
    );
    fireEvent.click(
      screen.getAllByRole("checkbox", {
        name: "Select edge-2 for Agent update",
      })[0],
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Update selected (2)" }),
    );

    await waitFor(() =>
      expect(createAgentUpdateTasksMock).toHaveBeenCalledWith(
        [firstNode.id, secondNode.id],
        "0.2.0",
        session.csrfToken,
      ),
    );
    expect(
      await screen.findByText("Some Agent update tasks were not accepted"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "edge-2: Another Center-issued task already occupies this node's task slot.",
      ),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Waiting for Agent").length).toBeGreaterThan(0);
  });

  it("keeps an offline update phase and bounded diagnostics visible", async () => {
    getSessionMock.mockResolvedValue(session);
    getEnrollmentMock.mockResolvedValue({
      enabled: true,
      hasKey: true,
      installationCommand: "install-agent",
      rotatedAt: "2026-08-10T07:00:00Z",
    });
    const node = updateTestNode("edge-offline", "3", "offline");
    listNodesMock.mockResolvedValue([node]);
    getAgentUpdateStateMock.mockResolvedValue({
      ...agentUpdateState,
      tasks: [
        {
          id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
          nodeId: node.id,
          targetVersion: "0.2.0",
          status: "verifying",
          createdAt: "2026-08-10T08:01:00Z",
          expiresAt: "2026-08-10T08:03:00Z",
          acknowledgedAt: "2026-08-10T08:01:10Z",
          startedAt: "2026-08-10T08:01:12Z",
          failureCode: "artifact_checksum_mismatch",
          diagnostic: "sha256 did not match the release manifest",
          offline: true,
        },
      ],
    });
    renderApplication("/nodes");

    expect(
      (await screen.findAllByText("Offline · Verifying artifact")).length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText("Failure code: artifact_checksum_mismatch").length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText("sha256 did not match the release manifest").length,
    ).toBeGreaterThan(0);
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
    expect((await screen.findAllByText("Default IPv4")).length).toBeGreaterThan(
      0,
    );
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

  it("configures a low-memory node and creates an immediate probe task", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    const pausedState = {
      nodeId: probeTestNode.id,
      schedule: { enabled: true, cron: "0 0 0 * * *", timezone: "agent-local" },
      lowMemoryOverride: false,
      physicalMemoryBytes: 64 * 1024 * 1024,
      pausedLowMemory: true,
      recentRuns: [],
    };
    const enabledState = {
      ...pausedState,
      lowMemoryOverride: true,
      pausedLowMemory: false,
    };
    getNodeProbeMock.mockResolvedValue(pausedState);
    updateProbeSettingsMock.mockResolvedValue(enabledState);
    createProbeTaskMock.mockResolvedValue({
      id: "b4bd9b72-a761-4f53-8a21-570aed465b88",
      nodeId: probeTestNode.id,
      status: "pending",
      createdAt: "2026-08-09T12:00:00Z",
      expiresAt: "2026-08-09T12:02:00Z",
      offline: false,
    });

    renderApplication(`/nodes/${probeTestNode.id}/probe`);

    expect(
      await screen.findByRole("heading", { name: "edge-1" }),
    ).toBeInTheDocument();
    expect(
      await screen.findByText("Complete probes are paused for low memory"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Run complete probe" }),
    ).toBeDisabled();
    fireEvent.click(
      screen.getByRole("switch", { name: "Allow probes below 256 MiB" }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Save probe settings" }),
    );
    await waitFor(() =>
      expect(updateProbeSettingsMock).toHaveBeenCalledWith(
        probeTestNode.id,
        {
          schedule: {
            enabled: true,
            cron: "0 0 0 * * *",
            timezone: "agent-local",
          },
          lowMemoryOverride: true,
        },
        session.csrfToken,
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Run complete probe" }));
    await waitFor(() =>
      expect(createProbeTaskMock).toHaveBeenCalledWith(
        probeTestNode.id,
        session.csrfToken,
      ),
    );
    expect(await screen.findByText("Waiting for Agent")).toBeInTheDocument();
  });

  it("shows partial probe runs with successful and failed egresses", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    getProbeRunMock.mockResolvedValue({
      id: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
      nodeId: probeTestNode.id,
      configurationRevision: 2,
      historyGeneration: "a".repeat(64),
      trigger: "schedule",
      startedAt: "2026-08-09T11:59:00Z",
      completedAt: "2026-08-09T12:00:00Z",
      status: "partial",
      expectedExecutions: 2,
      executions: [
        {
          id: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
          runId: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
          egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          ordinal: 0,
          sequence: 1,
          status: "succeeded",
          startedAt: "2026-08-09T11:59:01Z",
          completedAt: "2026-08-09T11:59:20Z",
          snapshotId: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
        },
        {
          id: "ea79d052-545b-46f2-926c-b82be60647e8",
          runId: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
          egressId: "da1a3999-e0bd-4649-85ae-aa9a4a9d6961",
          ordinal: 1,
          sequence: 1,
          status: "failed",
          startedAt: "2026-08-09T11:59:21Z",
          completedAt: "2026-08-09T11:59:40Z",
          failureStage: "process",
          diagnostic: "exit status 1",
        },
      ],
    });
    getNodeNetworkMock.mockResolvedValue({
      egresses: [
        {
          id: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          nodeId: probeTestNode.id,
          name: "default-ipv4",
          kind: "default",
          family: "ipv4",
          enabled: true,
          available: true,
          automatic: true,
          lightweightIntervalSeconds: 600,
          probeOnAddressChange: true,
        },
        {
          id: "da1a3999-e0bd-4649-85ae-aa9a4a9d6961",
          nodeId: probeTestNode.id,
          name: "default-ipv6",
          kind: "default",
          family: "ipv6",
          enabled: true,
          available: true,
          automatic: true,
          lightweightIntervalSeconds: 600,
          probeOnAddressChange: true,
        },
      ],
      candidates: [],
      addressStates: [],
      addressEvents: [],
      addressGaps: [],
    });

    renderApplication("/probe-runs/84e7d535-e04e-47f9-8374-1585a5dce6c9");

    expect(
      await screen.findByText("This run completed with partial success"),
    ).toBeInTheDocument();
    expect(screen.getByText("Default IPv4")).toBeInTheDocument();
    expect(screen.getByText("exit status 1")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Open report snapshot" }),
    ).toHaveAttribute(
      "href",
      "/probe-snapshots/cd6233d2-a600-443b-9cf5-a0bc3c241ea5?runId=84e7d535-e04e-47f9-8374-1585a5dce6c9",
    );
  });

  it("renders the exact decoded probe JSON", async () => {
    getSessionMock.mockResolvedValue(session);
    const snapshot = {
      id: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
      executionId: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
      egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
      sequence: 1,
      observedAt: "2026-08-09T11:59:20Z",
      rawResult: window.btoa('{"ip":"203.0.113.10"}'),
      starred: false,
      fields: [
        {
          id: "Head.IP",
          group: "Head",
          path: "Head.IP",
          expectedTypes: ["string" as const],
          status: "missing" as const,
        },
        {
          id: "Head.CountryCode",
          group: "Head",
          path: "Head.CountryCode",
          expectedTypes: ["string" as const],
          status: "incompatible" as const,
          actualType: "number" as const,
        },
      ],
      formatIssues: [
        {
          path: "Head.IP",
          kind: "missing" as const,
          expectedTypes: ["string" as const],
        },
        {
          path: "Head.CountryCode",
          kind: "incompatible" as const,
          expectedTypes: ["string" as const],
          actualType: "number" as const,
        },
      ],
      changes: [],
    };
    getProbeSnapshotMock.mockResolvedValue(snapshot);
    setSnapshotStarredMock.mockResolvedValue({ ...snapshot, starred: true });

    renderApplication("/probe-snapshots/cd6233d2-a600-443b-9cf5-a0bc3c241ea5");

    expect((await screen.findAllByText("IP address")).length).toBeGreaterThan(
      0,
    );
    expect(
      screen.getAllByText("Public IP address reported by the upstream probe.")
        .length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText("Head.IP").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Missing").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Incompatible type").length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", { name: "Star snapshot" }));
    await waitFor(() =>
      expect(setSnapshotStarredMock).toHaveBeenCalledWith(
        snapshot.id,
        true,
        session.csrfToken,
      ),
    );
    expect(
      await screen.findByRole("button", { name: "Unstar snapshot" }),
    ).toBeInTheDocument();
    fireEvent.mouseDown(await screen.findByRole("tab", { name: "Raw JSON" }), {
      button: 0,
      ctrlKey: false,
    });
    await waitFor(() =>
      expect(document.querySelector("pre")).toHaveTextContent("203.0.113.10"),
    );
    expect(
      screen.getByRole("button", { name: "Copy JSON" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Download JSON" }),
    ).toBeInTheDocument();
  });

  it("indexes retained reports and links the previous comparison", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    listHistorySnapshotsMock.mockResolvedValue({
      total: 1,
      items: [
        {
          id: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
          executionId: "ff4ce696-03f4-422c-b1ba-dcc5e7ad48e3",
          runId: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
          nodeId: probeTestNode.id,
          egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          owner: { nodeName: "edge-1", egressName: "Default IPv4" },
          sequence: 2,
          trigger: "manual",
          runStatus: "succeeded",
          observedAt: "2026-08-09T11:59:20Z",
          receivedAt: "2026-08-09T11:59:21Z",
          encodedSize: 128,
          starred: true,
          current: true,
          processed: true,
          baseline: false,
          changeCount: 1,
          formatStatus: "compatible",
          formatIssueCount: 0,
          previousSnapshotId: "9278587a-e1a9-4fe4-a5fc-5ece010c8a9f",
        },
      ],
    });

    renderApplication(
      "/history?from=2026-08-01T00%3A00&to=2026-08-10T00%3A00&runStatus=succeeded&trigger=manual&changed=true&formatStatus=compatible",
    );

    expect(
      await screen.findByRole("heading", { name: "History" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(listHistorySnapshotsMock).toHaveBeenCalled());
    expect(
      (await screen.findAllByText("Default IPv4", { exact: false })).length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText("Succeeded").length).toBeGreaterThan(0);
    expect(
      screen.getAllByRole("link", { name: "Compare with previous" })[0],
    ).toHaveAttribute(
      "href",
      "/history/compare?before=9278587a-e1a9-4fe4-a5fc-5ece010c8a9f&after=cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
    );
    expect(listHistorySnapshotsMock).toHaveBeenCalledWith(
      {
        from: "2026-08-01T00:00:00.000Z",
        to: "2026-08-10T00:00:00.000Z",
        page: 1,
        pageSize: 25,
        runStatus: "succeeded",
        trigger: "manual",
        changed: true,
        formatStatus: "compatible",
      },
      expect.any(AbortSignal),
    );
  });

  it("indexes address transitions with URL-backed filters", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    listHistoryAddressesMock.mockResolvedValue({
      total: 1,
      gapTotal: 0,
      events: [
        {
          nodeId: probeTestNode.id,
          owner: { nodeName: "edge-1", egressName: "Default IPv4" },
          event: {
            id: "758db6d8-d8cd-44c5-a18d-ab7713012ec8",
            egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
            historyGeneration: "a".repeat(64),
            sequence: 3,
            kind: "address-change",
            family: "ipv4",
            previousAddress: "203.0.113.9",
            publicAddress: "203.0.113.10",
            localInterface: "eth0",
            localAddress: "10.0.0.5",
            proxyPath: false,
            likelyNat: true,
            temporary: false,
            observedAt: "2026-08-09T12:00:00Z",
          },
        },
      ],
      gaps: [],
    });

    renderApplication(
      "/history?tab=addresses&from=2026-08-01T00%3A00&to=2026-08-10T00%3A00&eventKind=address-change&family=ipv4",
    );

    expect(
      (
        await screen.findAllByText("10.0.0.5 -> 203.0.113.10", {
          exact: false,
        })
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText("Likely NAT").length).toBeGreaterThan(0);
    expect(listHistoryAddressesMock).toHaveBeenCalledWith(
      {
        from: "2026-08-01T00:00:00.000Z",
        to: "2026-08-10T00:00:00.000Z",
        page: 1,
        gapPage: 1,
        pageSize: 25,
        eventKind: "address-change",
        family: "ipv4",
      },
      expect.any(AbortSignal),
    );
  });

  it("pages report gaps and format events independently", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    listHistoryGapsMock.mockResolvedValue({
      total: 26,
      items: [
        {
          id: "b52f3131-f684-4c28-bdf0-1be6498d79f8",
          nodeId: probeTestNode.id,
          egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          owner: { nodeName: "edge-1", egressName: "Default IPv4" },
          droppedCount: 1,
          firstSequence: 4,
          lastSequence: 4,
          firstObservedAt: "2026-08-09T12:00:00Z",
          lastObservedAt: "2026-08-09T12:00:00Z",
        },
      ],
    });
    listHistoryFormatsMock.mockResolvedValue({
      total: 26,
      items: [
        {
          id: "f6f79d7e-bebb-4fae-bf0f-3bcb2c8ea668",
          nodeId: probeTestNode.id,
          egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          executionId: "6a88f1ee-4e4c-4d25-9edb-c0a508afeb56",
          snapshotId: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
          owner: { nodeName: "edge-1", egressName: "Default IPv4" },
          sequence: 4,
          kind: "mismatch",
          issues: [],
          observedAt: "2026-08-09T12:00:00Z",
          recordedAt: "2026-08-09T12:00:01Z",
        },
      ],
    });

    renderApplication("/history");

    expect(await screen.findByText("Probe history gaps")).toBeInTheDocument();
    expect(screen.getByText("Upstream format events")).toBeInTheDocument();
    let nextButtons = screen.getAllByRole("button", { name: "Next" });
    expect(nextButtons).toHaveLength(2);
    fireEvent.click(nextButtons[0]);
    await waitFor(() =>
      expect(listHistoryGapsMock).toHaveBeenLastCalledWith(
        expect.objectContaining({ page: 2 }),
        expect.any(AbortSignal),
      ),
    );
    expect(listHistoryFormatsMock).toHaveBeenLastCalledWith(
      expect.objectContaining({ page: 1 }),
      expect.any(AbortSignal),
    );

    nextButtons = screen.getAllByRole("button", { name: "Next" });
    fireEvent.click(nextButtons[1]);
    await waitFor(() =>
      expect(listHistoryFormatsMock).toHaveBeenLastCalledWith(
        expect.objectContaining({ page: 2 }),
        expect.any(AbortSignal),
      ),
    );
  });

  it("renders typed snapshot comparison states", async () => {
    getSessionMock.mockResolvedValue(session);
    compareSnapshotsMock.mockResolvedValue({
      beforeId: "9278587a-e1a9-4fe4-a5fc-5ece010c8a9f",
      afterId: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
      egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
      fields: [
        {
          id: "Head.IP",
          group: "Head",
          path: "Head.IP",
          expectedTypes: ["string"],
          changed: true,
          before: {
            id: "Head.IP",
            group: "Head",
            path: "Head.IP",
            expectedTypes: ["string"],
            status: "available",
            actualType: "string",
            value: "203.0.113.1",
          },
          after: {
            id: "Head.IP",
            group: "Head",
            path: "Head.IP",
            expectedTypes: ["string"],
            status: "incompatible",
            actualType: "number",
          },
        },
      ],
    });

    renderApplication(
      "/history/compare?before=9278587a-e1a9-4fe4-a5fc-5ece010c8a9f&after=cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
    );

    expect(
      await screen.findByRole("heading", { name: "Snapshot comparison" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("IP address")).toBeInTheDocument();
    expect(screen.getByText("Head.IP")).toBeInTheDocument();
    expect(screen.getByText("Incompatible type")).toBeInTheDocument();
    expect(screen.getByText("Unavailable")).toBeInTheDocument();
    await i18n.changeLanguage("zh-CN");
    expect(await screen.findByText("IP 地址")).toBeInTheDocument();
    expect(
      screen.getByText("上游探测报告的公网 IP 地址。"),
    ).toBeInTheDocument();
  });

  it("clears history only after destructive confirmation", async () => {
    getSessionMock.mockResolvedValue(session);
    getHistoryStateMock.mockResolvedValue(historyState("a".repeat(64)));
    resetHistoryMock.mockResolvedValue({
      ...historyState("b".repeat(64)),
      resetAt: "2026-08-09T12:00:00Z",
    });

    renderApplication("/settings/history");

    expect(
      await screen.findByRole("heading", { name: "History and storage" }),
    ).toBeInTheDocument();
    fireEvent.click(
      await screen.findByRole("button", { name: "Clear history" }),
    );
    expect(resetHistoryMock).not.toHaveBeenCalled();
    fireEvent.click(
      await screen.findByRole("button", { name: "Clear all history" }),
    );
    await waitFor(() =>
      expect(resetHistoryMock).toHaveBeenCalledWith(session.csrfToken),
    );
    expect(await screen.findByText("b".repeat(64))).toBeInTheDocument();
  });

  it("applies retention and runs immediate history cleanup", async () => {
    getSessionMock.mockResolvedValue(session);
    const initial = {
      ...historyState("a".repeat(64)),
      retention: {
        ...historyState("a".repeat(64)).retention,
        mode: "age" as const,
        maxAgeDays: 30,
      },
    };
    const cleaned = {
      ...initial,
      retention: {
        ...initial.retention,
        lastCleanupAt: "2026-08-09T12:00:00Z",
        lastCleanupDeletedItems: 4,
      },
    };
    getHistoryStateMock
      .mockResolvedValueOnce(initial)
      .mockResolvedValueOnce(cleaned);
    updateRetentionMock.mockResolvedValue({
      ...initial,
      retention: { ...initial.retention, maxAgeDays: 45 },
    });
    cleanupHistoryMock.mockResolvedValue({
      deletedItems: 4,
      completedAt: "2026-08-09T12:00:00Z",
      usage: cleaned.usage,
    });

    renderApplication("/settings/history");

    const days = await screen.findByLabelText("Retention days");
    fireEvent.change(days, { target: { value: "45" } });
    fireEvent.click(screen.getByRole("button", { name: "Save and apply" }));
    await waitFor(() =>
      expect(updateRetentionMock).toHaveBeenCalledWith(
        { mode: "age", maxAgeDays: 45 },
        session.csrfToken,
      ),
    );
    expect(
      await screen.findByText("The retention policy was saved and applied."),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Clean now" }));
    await waitFor(() =>
      expect(cleanupHistoryMock).toHaveBeenCalledWith(session.csrfToken),
    );
    expect(getHistoryStateMock).toHaveBeenCalledTimes(2);
    expect(
      await screen.findByText("Cleanup completed and removed 4 items."),
    ).toBeInTheDocument();
  });
});

const probeTestNode = {
  id: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
  name: "edge-1",
  hostname: "edge-1",
  status: "online" as const,
  enabled: true,
  agentVersion: "0.1.0",
  operatingSystem: "linux" as const,
  architecture: "amd64" as const,
  capabilities: ["control-v1", "complete-probe-v1"],
  desiredConfigurationRevision: 2,
  appliedConfigurationRevision: 2,
  configurationStatus: "current" as const,
  registeredAt: "2026-08-09T11:00:00Z",
  lastSeenAt: "2026-08-09T12:00:00Z",
};

function updateTestNode(
  name: string,
  suffix: string,
  status: "online" | "offline" = "online",
) {
  return {
    id: `7289cfa3-a75d-4a3f-ac06-${suffix.padStart(12, "0")}`,
    name,
    hostname: name,
    status,
    enabled: true,
    agentVersion: "0.1.0",
    sourceRevision: "1111111111111111111111111111111111111111",
    operatingSystem: "linux" as const,
    architecture: "amd64" as const,
    capabilities: ["control-v1", "agent-update-v1"],
    desiredConfigurationRevision: 2,
    appliedConfigurationRevision: 2,
    configurationStatus: "current" as const,
    registeredAt: "2026-08-09T11:00:00Z",
    lastSeenAt: "2026-08-10T08:00:00Z",
  };
}

function historyState(generation: string) {
  return {
    generation,
    retention: {
      mode: "indefinite" as const,
      updatedAt: "2026-08-09T00:00:00Z",
      lastCleanupDeletedItems: 0,
    },
    usage: {
      logicalBytes: 0,
      protectedLogicalBytes: 0,
      recordCount: 0,
      databaseBytes: 4096,
      walBytes: 0,
      sharedMemoryBytes: 0,
      overBudget: false,
      overageBytes: 0,
    },
  };
}

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
