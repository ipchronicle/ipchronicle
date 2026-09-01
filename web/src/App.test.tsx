import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { toBlob } from "html-to-image";

import {
  getAuthenticatedSession,
  login,
  logout,
  updateAccountLocale,
  type AuthenticatedSession,
} from "@/api/auth";
import {
  getSystemSettings,
  getSystemStatus,
  updateSystemSettings,
} from "@/api/system";
import { getOverview } from "@/api/overview";
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
  getNetworkObservationSettings,
  getNodeNetwork,
  updateNetworkObservationSettings,
  updatePublicAddress,
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
  previewProbeSchedule,
  resetHistory,
  updateNodeProbeSettings,
} from "@/api/probes";
import { APIError } from "@/api/errors";
import App from "@/App";
import { AuthProvider } from "@/auth-context";
import { TooltipProvider } from "@/components/ui/tooltip";
import i18n from "@/i18n";

const writeClipboardTextMock = vi.fn();

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
  getSystemSettings: vi.fn(),
  getSystemStatus: vi.fn(),
  updateSystemSettings: vi.fn(),
}));

vi.mock("@/api/overview", () => ({
  getOverview: vi.fn(),
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
  getNetworkObservationSettings: vi.fn(),
  getNodeNetwork: vi.fn(),
  updateNetworkObservationSettings: vi.fn(),
  updatePublicAddress: vi.fn(),
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
  previewProbeSchedule: vi.fn(),
  resetHistory: vi.fn(),
  updateNodeProbeSettings: vi.fn(),
}));

vi.mock("html-to-image", () => ({
  toBlob: vi.fn(),
}));

const getSessionMock = vi.mocked(getAuthenticatedSession);
const loginMock = vi.mocked(login);
const logoutMock = vi.mocked(logout);
const updateLocaleMock = vi.mocked(updateAccountLocale);
const getSystemSettingsMock = vi.mocked(getSystemSettings);
const getOverviewMock = vi.mocked(getOverview);
const getSystemStatusMock = vi.mocked(getSystemStatus);
const updateSystemSettingsMock = vi.mocked(updateSystemSettings);
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
const getObservationSettingsMock = vi.mocked(getNetworkObservationSettings);
const getNodeNetworkMock = vi.mocked(getNodeNetwork);
const updateObservationSettingsMock = vi.mocked(
  updateNetworkObservationSettings,
);
const updatePublicAddressMock = vi.mocked(updatePublicAddress);
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
const previewProbeScheduleMock = vi.mocked(previewProbeSchedule);
const resetHistoryMock = vi.mocked(resetHistory);
const updateProbeSettingsMock = vi.mocked(updateNodeProbeSettings);
const toBlobMock = vi.mocked(toBlob);

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
  configSchemaVersion: 1,
  historySchemaVersion: 1,
  transportSecurity: "http" as const,
  transportWarning: true,
  externalOriginMode: "automatic" as const,
  trustedProxyConfigured: false,
};

const healthyOverview = {
  checkedAt: "2026-08-10T08:00:00Z",
  historyOverBudget: false,
  nodes: [
    {
      id: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      name: "edge-1",
      status: "online" as const,
      configurationStatus: "current" as const,
      desiredConfigurationRevision: 2,
      appliedConfigurationRevision: 2,
      lastSeenAt: "2026-08-10T08:00:00Z",
      pausedLowMemory: false,
      nextScheduledAt: "2026-08-11T00:00:00Z",
      publicAddresses: [],
    },
  ],
  activeTasks: [],
  recentProbeRuns: [],
  recentAddressEvents: [],
};

const agentUpdateState = {
  channel: "stable" as const,
  currentVersion: "0.1.0",
  currentRevision: "1111111111111111111111111111111111111111",
  checkedAt: "2026-08-10T08:00:00Z",
  tasks: [],
};

function timelineSnapshot(
  id: string,
  egressId: string,
  sequence: number,
  observedAt: string,
) {
  return {
    id,
    executionId: "ff4ce696-03f4-422c-b1ba-dcc5e7ad48e3",
    runId: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
    nodeId: probeTestNode.id,
    egressId,
    owner: {
      nodeName: "edge-1",
      egressName: "Default IPv4",
      nodeDeleted: false,
    },
    sequence,
    trigger: "manual" as const,
    runStatus: "succeeded" as const,
    observedAt,
    receivedAt: observedAt,
    encodedSize: 128,
    starred: false,
    current: sequence === 3,
    processed: true,
    baseline: sequence === 1,
    changeCount: sequence === 1 ? 0 : 1,
    formatStatus: "compatible" as const,
    formatIssueCount: 0,
  };
}

describe("administrator application", () => {
  beforeEach(async () => {
    window.localStorage.clear();
    document.documentElement.className = "";
    await i18n.changeLanguage("en");
    writeClipboardTextMock.mockReset();
    writeClipboardTextMock.mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: writeClipboardTextMock },
    });
    getSessionMock.mockReset();
    loginMock.mockReset();
    logoutMock.mockReset();
    updateLocaleMock.mockReset();
    getSystemSettingsMock.mockReset();
    getSystemSettingsMock.mockResolvedValue({
      automatic: true,
      externalOrigin: "",
      effectiveOrigin: window.location.origin,
    });
    getSystemStatusMock.mockReset();
    getSystemStatusMock.mockResolvedValue(healthyStatus);
    getOverviewMock.mockReset();
    getOverviewMock.mockResolvedValue(healthyOverview);
    updateSystemSettingsMock.mockReset();
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
    getNodeProbeMock.mockResolvedValue({
      nodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      schedule: {
        enabled: true,
        cron: "0 0 0 * * *",
        timezone: "UTC",
      },
      lowMemoryOverride: false,
      probeOnNewAddress: true,
      pausedLowMemory: false,
      recentRuns: [],
    });
    getProbeRunMock.mockReset();
    getProbeSnapshotMock.mockReset();
    previewProbeScheduleMock.mockReset();
    previewProbeScheduleMock.mockResolvedValue({
      nextScheduledAt: "2026-08-11T00:00:00Z",
    });
    resetHistoryMock.mockReset();
    updateProbeSettingsMock.mockReset();
    toBlobMock.mockReset();
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

  it("authenticates and renders the global overview", async () => {
    getSessionMock.mockResolvedValue(null);
    loginMock.mockResolvedValue(session);
    getSystemStatusMock.mockResolvedValue(healthyStatus);
    renderApplication("/login");

    fireEvent.change(await screen.findByLabelText("Password"), {
      target: { value: "admin" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    expect(
      await screen.findByRole("heading", { name: "Overview" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("navigation", { name: "Primary navigation" }),
    ).toBeInTheDocument();
    const primaryGroup = screen
      .getByText("Primary navigation")
      .closest('[data-slot="sidebar-group"]') as HTMLElement;
    const settingsGroup = screen
      .getByText("Settings")
      .closest('[data-slot="sidebar-group"]') as HTMLElement;
    expect(
      within(primaryGroup).queryByRole("link", {
        name: /^System$/,
      }),
    ).not.toBeInTheDocument();
    expect(
      within(settingsGroup).getByRole("link", {
        name: /^System$/,
      }),
    ).toHaveAttribute("href", "/settings/system");
    expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    const sidebar = document.querySelector('[data-slot="sidebar"][data-state]');
    expect(sidebar).toHaveAttribute("data-state", "expanded");
    fireEvent.click(screen.getByRole("button", { name: "Toggle sidebar" }));
    expect(sidebar).toHaveAttribute("data-state", "collapsed");
    expect(
      await screen.findByText("No current issues need attention"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Default credentials are still active"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Browser connection uses HTTP"),
    ).toBeInTheDocument();
  });

  it("shows a recoverable overview API failure", async () => {
    getSessionMock.mockResolvedValue(session);
    getSystemStatusMock
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(healthyStatus);
    renderApplication("/");

    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
    expect(
      await screen.findByText("No current issues need attention"),
    ).toBeInTheDocument();
  });

  it("prioritizes current probe and node issues with drill-down links", async () => {
    getSessionMock.mockResolvedValue(session);
    getOverviewMock.mockResolvedValue({
      ...healthyOverview,
      nodes: [
        {
          ...healthyOverview.nodes[0],
          status: "offline",
          configurationStatus: "failed",
          appliedConfigurationRevision: 1,
          pausedLowMemory: true,
          publicAddresses: [
            {
              id: "6b15a701-8f23-40dd-a1b2-13982dba217f",
              address: "203.0.113.10",
              family: "ipv4",
              probeEnabled: true,
              likelyNat: true,
              proxyPath: false,
              lastSeenAt: "2026-08-10T07:50:00Z",
              latestProbeAt: "2026-08-10T07:55:00Z",
              latestProbeRunId: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
              latestProbeOutcome: "failed",
              formatStatus: "mismatch",
            },
          ],
        },
      ],
    });
    renderApplication("/");

    expect(
      await screen.findByText("The latest probe for 203.0.113.10 failed"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", {
        name: /The latest probe for 203\.0\.113\.10 failed/,
      }),
    ).toHaveAttribute(
      "href",
      "/probe-runs/84e7d535-e04e-47f9-8374-1585a5dce6c9",
    );
    expect(screen.getByText("edge-1 is offline")).toBeInTheDocument();
    expect(
      screen.getByText("Complete probes are paused on edge-1"),
    ).toBeInTheDocument();
  });

  it("uses the retained node name for deleted-node overview activity", async () => {
    getSessionMock.mockResolvedValue(session);
    getOverviewMock.mockResolvedValue({
      ...healthyOverview,
      recentProbeRuns: [
        {
          id: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
          nodeId: "cf6e7da4-4072-4ca5-a048-91ccfebeb537",
          owner: { nodeName: "retired-edge", nodeDeleted: true },
          trigger: "manual",
          startedAt: "2026-08-09T11:59:00Z",
          completedAt: "2026-08-09T12:00:00Z",
          status: "succeeded",
          expectedExecutions: 1,
          completedExecutions: 1,
        },
      ],
    });

    renderApplication("/");

    expect(
      await screen.findByText("retired-edge probe: Succeeded"),
    ).toBeInTheDocument();
    expect(screen.getByText("Node deleted")).toBeInTheDocument();
    expect(
      screen.queryByText("cf6e7da4-4072-4ca5-a048-91ccfebeb537"),
    ).not.toBeInTheDocument();
  });

  it("shows the Agent command directly on an empty overview", async () => {
    getSessionMock.mockResolvedValue(session);
    getOverviewMock.mockResolvedValue({
      ...healthyOverview,
      nodes: [],
    });
    getEnrollmentMock.mockResolvedValue({
      enabled: true,
      hasKey: true,
      registrationKey: "ipc_reg_test",
      defaultProbeTimezone: "UTC",
      rotatedAt: "2026-08-10T07:00:00Z",
    });
    renderApplication("/");

    expect(
      await screen.findByText("Connect the first node"),
    ).toBeInTheDocument();
    expect(await screen.findByText(/ipc_reg_test/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Copy command" }),
    ).toBeInTheDocument();
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
    updateSystemSettingsMock.mockResolvedValue({
      automatic: false,
      externalOrigin: "https://ip.example.com",
      effectiveOrigin: "https://ip.example.com",
    });
    renderApplication("/settings/system");

    expect(
      await screen.findByRole("heading", { name: "System" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("0.1.1")).toBeInTheDocument();
    expect(
      screen.getByText("2222222222222222222222222222222222222222"),
    ).toBeInTheDocument();
    const automaticSwitch = screen.getByRole("switch", {
      name: "Use this browser's current address",
    });
    expect(automaticSwitch).toBeChecked();
    expect(automaticSwitch).toHaveClass("cursor-pointer");
    fireEvent.click(screen.getByText("Use this browser's current address"));
    expect(automaticSwitch).toBeChecked();
    fireEvent.click(automaticSwitch);
    expect(automaticSwitch).not.toBeChecked();
    const externalOrigin = screen.getByLabelText("Custom external address");
    fireEvent.change(externalOrigin, {
      target: { value: "https://ip.example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save address" }));
    await waitFor(() =>
      expect(updateSystemSettingsMock).toHaveBeenCalledWith(
        "https://ip.example.com",
        session.csrfToken,
      ),
    );
    expect(
      screen.getByText("External address settings saved."),
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
    expect(screen.getByLabelText("Public IP")).toBeDisabled();
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

    await screen.findByRole("heading", { name: "Overview" });
    fireEvent.click(
      await screen.findByRole("button", {
        name: "Switch to Simplified Chinese",
      }),
    );
    await waitFor(() => expect(updateLocaleMock).toHaveBeenCalledWith("zh-CN"));
    expect(
      await screen.findByRole("heading", { name: "总览" }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "使用深色主题" }));
    expect(document.documentElement).toHaveClass("dark");
  });

  it("uses the browser timezone when generating an enrollment key", async () => {
    getSessionMock.mockResolvedValue(session);
    getEnrollmentMock.mockResolvedValue({
      enabled: false,
      hasKey: false,
    });
    listNodesMock.mockResolvedValue([]);
    const defaultProbeTimezone =
      Intl.DateTimeFormat().resolvedOptions().timeZone;
    rotateEnrollmentMock.mockResolvedValue({
      enabled: true,
      hasKey: true,
      registrationKey: "secret",
      defaultProbeTimezone,
      rotatedAt: "2026-08-31T10:00:00Z",
    });

    renderApplication("/nodes");

    fireEvent.click(
      await screen.findByRole("button", { name: "Generate key" }),
    );
    await waitFor(() =>
      expect(rotateEnrollmentMock).toHaveBeenCalledWith(
        defaultProbeTimezone,
        session.csrfToken,
      ),
    );
  });

  it("shows the installation command and registered node state", async () => {
    getSessionMock.mockResolvedValue(session);
    getEnrollmentMock.mockResolvedValue({
      enabled: true,
      hasKey: true,
      registrationKey: "secret",
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
        capabilities: ["control-v1", "sync-wakeup-v1", "complete-probe-v1"],
        desiredConfigurationRevision: 1,
        appliedConfigurationRevision: 1,
        configurationStatus: "current",
        publicAddresses: [
          {
            id: "14f44250-67e7-44d6-bb15-e30fc80af44c",
            address: "203.0.113.10",
            family: "ipv4",
            available: true,
            probeEnabled: false,
          },
        ],
        registeredAt: "2026-08-07T12:00:00Z",
        lastSeenAt: "2026-08-07T12:01:00Z",
      },
    ]);
    updateEnrollmentMock.mockResolvedValue({
      enabled: false,
      hasKey: true,
      registrationKey: "secret",
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
      publicAddresses: [],
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
      publicAddresses: [],
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
      publicAddresses: [],
      registeredAt: "2026-08-07T12:00:00Z",
      lastSeenAt: "2026-08-07T12:01:00Z",
    });

    createProbeTaskMock.mockResolvedValue({
      id: "b4bd9b72-a761-4f53-8a21-570aed465b88",
      nodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      status: "pending",
      createdAt: "2026-08-07T12:02:00Z",
      expiresAt: "2026-08-07T12:04:00Z",
      offline: false,
    });
    getNodeNetworkMock.mockResolvedValue({
      publicAddresses: [
        {
          id: "14f44250-67e7-44d6-bb15-e30fc80af44c",
          address: "203.0.113.10",
          family: "ipv4",
          probeEnabled: false,
          available: true,
          selectedNodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
          selectedNodeName: "edge-1",
          pathCount: 1,
          likelyNat: false,
          proxyPath: false,
          firstSeenAt: "2026-08-07T12:01:00Z",
          lastSeenAt: "2026-08-07T12:02:00Z",
        },
      ],
      networkProxies: [],
      addressEvents: [],
      addressGaps: [],
    });
    getNodeProbeMock.mockResolvedValue({
      nodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      schedule: {
        enabled: true,
        cron: "0 0 0 * * *",
        timezone: "UTC",
      },
      lowMemoryOverride: false,
      probeOnNewAddress: true,
      pausedLowMemory: false,
      recentRuns: [],
    });
    renderApplication("/nodes");

    expect(
      await screen.findByRole("heading", { name: "Nodes" }),
    ).toBeInTheDocument();
    const installationCommand = await waitFor(() => {
      const command = document.querySelector("pre code")?.textContent;
      expect(command).toContain(
        "https://raw.githubusercontent.com/ipchronicle/ipchronicle/main/scripts/install-agent.sh",
      );
      return command;
    });
    expect(installationCommand).toContain(
      `--center-url '${window.location.origin}'`,
    );
    expect(installationCommand).not.toContain("--version");
    const copyCommandButton = screen.getByRole("button", {
      name: "Copy command",
    });
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
    fireEvent.click(copyCommandButton);
    await waitFor(() =>
      expect(writeClipboardTextMock).toHaveBeenCalledWith(installationCommand),
    );
    expect(copyCommandButton).toHaveTextContent("Copy command");
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Installation command copied.",
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect((await screen.findAllByText("edge-1")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("203.0.113.10").length).toBeGreaterThan(0);
    expect(
      screen.getAllByText("Complete probe disabled").length,
    ).toBeGreaterThan(0);
    fireEvent.change(screen.getByLabelText("Search nodes"), {
      target: { value: "203.0.113.10" },
    });
    expect(screen.getAllByText("203.0.113.10").length).toBeGreaterThan(0);
    fireEvent.change(screen.getByLabelText("Search nodes"), {
      target: { value: "" },
    });
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
    expect(
      screen.queryByRole("button", { name: "Start temporary sync" }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", { name: "Run probe" })[0]);
    const probeDialog = await screen.findByRole("alertdialog");
    fireEvent.click(within(probeDialog).getByText("203.0.113.10"));
    expect(
      within(probeDialog).getByRole("checkbox", { name: /203\.0\.113\.10/ }),
    ).toBeChecked();
    expect(screen.getByRole("alertdialog")).toBe(probeDialog);
    fireEvent.click(
      within(probeDialog).getByRole("button", { name: "Run probe" }),
    );
    await waitFor(() =>
      expect(createProbeTaskMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        {
          publicAddressIds: ["14f44250-67e7-44d6-bb15-e30fc80af44c"],
        },
        session.csrfToken,
      ),
    );
    fireEvent.click(screen.getAllByRole("button", { name: "Pause node" })[0]);
    await waitFor(() =>
      expect(updateNodeMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        { enabled: false },
        session.csrfToken,
      ),
    );
    expect(screen.getAllByText("Disabled").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Pending · 1/2").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("row", { name: /edge-1/ }));
    expect(
      await screen.findByRole("heading", { name: "edge-1" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Overview" })).toHaveAttribute(
      "data-state",
      "active",
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Start temporary sync" }),
    );
    await waitFor(() =>
      expect(startSyncMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        session.csrfToken,
      ),
    );
    expect(screen.getAllByText("Waiting for Agent").length).toBeGreaterThan(0);
    fireEvent.click(
      (
        await screen.findAllByRole("button", {
          name: "Stop temporary sync",
        })
      )[0],
    );
    await waitFor(() =>
      expect(stopSyncMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        session.csrfToken,
      ),
    );
  });

  it("adds a newly enrolled node without reloading the page", async () => {
    getSessionMock.mockResolvedValue(session);
    getEnrollmentMock.mockResolvedValue({
      enabled: true,
      hasKey: true,
      registrationKey: "secret",
      rotatedAt: "2026-08-07T12:00:00Z",
    });
    getAgentUpdateStateMock.mockResolvedValue(agentUpdateState);
    listNodesMock.mockResolvedValueOnce([]).mockResolvedValue([probeTestNode]);

    let refresh: (() => void) | undefined;
    const interval = vi
      .spyOn(window, "setInterval")
      .mockImplementation((handler, timeout) => {
        if (timeout === 3_000 && typeof handler === "function") {
          refresh = handler as () => void;
        }
        return 1;
      });
    try {
      renderApplication("/nodes");

      expect(
        await screen.findByText("No nodes are registered"),
      ).toBeInTheDocument();
      expect(refresh).toBeDefined();
      await act(async () => refresh?.());
      expect(
        (await screen.findAllByText(probeTestNode.name)).length,
      ).toBeGreaterThan(0);
      expect(listNodesMock).toHaveBeenCalledTimes(2);
    } finally {
      interval.mockRestore();
    }
  });

  it("copies preserving and purging Agent uninstall commands", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);

    renderApplication(`/nodes/${probeTestNode.id}/settings`);

    expect(await screen.findByText("Uninstall Agent")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Copy uninstall command" }),
    );
    await waitFor(() =>
      expect(writeClipboardTextMock).toHaveBeenLastCalledWith(
        expect.stringMatching(/sh -s -- --uninstall$/),
      ),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Copy complete uninstall command" }),
    );
    await waitFor(() =>
      expect(writeClipboardTextMock).toHaveBeenLastCalledWith(
        expect.stringMatching(/sh -s -- --uninstall --purge$/),
      ),
    );
    for (const [command] of writeClipboardTextMock.mock.calls) {
      expect(command).not.toContain("--registration-key");
    }
  });

  it("filters updateable nodes and reports partial grouped update results", async () => {
    getSessionMock.mockResolvedValue(session);
    getEnrollmentMock.mockResolvedValue({
      enabled: true,
      hasKey: true,
      registrationKey: "secret",
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
      registrationKey: "secret",
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

  it("shows public IPs and node-scoped automatic proxy results", async () => {
    getSessionMock.mockResolvedValue(session);
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
        publicAddresses: [],
        registeredAt: "2026-08-09T06:00:00Z",
      },
    ]);
    getNodeNetworkMock.mockResolvedValue({
      publicAddresses: [
        {
          id: "4a44d3d7-7b45-4a3e-9e5c-e70fdba46e72",
          address: "8.8.8.8",
          family: "ipv4",
          probeEnabled: false,
          available: true,
          selectedNodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
          selectedNodeName: "edge-1",
          pathCount: 2,
          likelyNat: true,
          proxyPath: false,
          firstSeenAt: "2026-08-09T06:01:00Z",
          lastSeenAt: "2026-08-09T06:02:00Z",
          latestSnapshotId: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
          latestSnapshotAt: "2026-08-09T06:03:00Z",
        },
        {
          id: "48e69536-e0aa-47be-8832-0ca085fdc622",
          address: "2001:db8::8",
          family: "ipv6",
          probeEnabled: true,
          available: true,
          selectedNodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
          selectedNodeName: "edge-1",
          pathCount: 1,
          likelyNat: false,
          proxyPath: false,
          firstSeenAt: "2026-08-09T06:01:00Z",
          lastSeenAt: "2026-08-09T06:02:00Z",
        },
      ],
      networkProxies: [
        {
          id: "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16",
          nodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
          name: "Primary proxy",
          scheme: "socks5",
          host: "proxy.example.test",
          port: 1080,
          username: "probe-user",
          passwordConfigured: true,
          status: "ipv4-only",
          ipv4: {
            status: "available",
            publicAddress: "198.51.100.20",
            lastCheckedAt: "2026-08-09T06:02:00Z",
          },
          ipv6: {
            status: "unavailable",
            failureReason: "no-valid-response",
            lastCheckedAt: "2026-08-09T06:02:00Z",
          },
          createdAt: "2026-08-09T06:00:00Z",
          updatedAt: "2026-08-09T06:00:00Z",
        },
      ],
      addressEvents: [],
      addressGaps: [],
    });
    updatePublicAddressMock.mockResolvedValue({
      id: "4a44d3d7-7b45-4a3e-9e5c-e70fdba46e72",
      address: "8.8.8.8",
      family: "ipv4",
      probeEnabled: true,
      available: true,
      selectedNodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      selectedNodeName: "edge-1",
      pathCount: 2,
      likelyNat: true,
      proxyPath: false,
      firstSeenAt: "2026-08-09T06:01:00Z",
      lastSeenAt: "2026-08-09T06:02:00Z",
    });
    createProbeTaskMock.mockResolvedValue({
      id: "669a846d-59ac-45cc-a520-86cbc5e69992",
      nodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      status: "pending",
      createdAt: "2026-08-09T06:04:00Z",
      expiresAt: "2026-08-09T06:06:00Z",
      offline: false,
    });

    renderApplication("/nodes/7289cfa3-a75d-4a3f-ac06-8f1074446a85/network");

    expect(
      await screen.findByRole("heading", { name: "edge-1" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("8.8.8.8")).toBeInTheDocument();
    expect(screen.getByText("Reached through NAT")).toBeInTheDocument();
    expect(screen.queryByText("eth0")).not.toBeInTheDocument();
    expect(screen.getByText("Primary proxy")).toBeInTheDocument();
    expect(screen.getByText("IPv4 only")).toBeInTheDocument();
    expect(screen.getByText("198.51.100.20")).toBeInTheDocument();
    expect(screen.getByText("No public IP discovered")).toBeInTheDocument();
    expect(screen.queryByLabelText("Address family")).not.toBeInTheDocument();
    expect(screen.getByText("Password configured")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "View result" })).toHaveAttribute(
      "href",
      "/probe-snapshots/cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
    );
    expect(screen.getByRole("button", { name: "View result" })).toBeDisabled();
    fireEvent.click(screen.getAllByRole("button", { name: "Probe now" })[0]);
    await waitFor(() =>
      expect(createProbeTaskMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        { publicAddressIds: ["4a44d3d7-7b45-4a3e-9e5c-e70fdba46e72"] },
        session.csrfToken,
      ),
    );
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();
    expect(updatePublicAddressMock).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Edit" })).toBeEnabled();
    expect(
      screen.queryByDisplayValue("retained-secret"),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    const editDialog = screen.getByRole("dialog", { name: "Edit proxy" });
    expect(within(editDialog).getByLabelText("Password")).toHaveValue("");
    fireEvent.click(within(editDialog).getByRole("button", { name: "Cancel" }));
    fireEvent.click(
      screen.getAllByRole("switch", { name: "Enable complete probe" })[0],
    );
    await waitFor(() =>
      expect(updatePublicAddressMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        "4a44d3d7-7b45-4a3e-9e5c-e70fdba46e72",
        { probeEnabled: true },
        session.csrfToken,
      ),
    );

    deleteProxyMock.mockResolvedValue({
      id: "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16",
      nodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      name: "Primary proxy",
      scheme: "socks5",
      host: "proxy.example.test",
      port: 1080,
      username: "probe-user",
      passwordConfigured: true,
      status: "ipv4-only",
      ipv4: { status: "available", publicAddress: "198.51.100.20" },
      ipv6: { status: "unavailable", failureReason: "no-valid-response" },
      deletionStatus: "pending",
      createdAt: "2026-08-09T06:00:00Z",
      updatedAt: "2026-08-09T06:03:00Z",
    });
    fireEvent.click(screen.getByRole("button", { name: "Delete proxy" }));
    expect(
      await screen.findByText(
        "Discovered public IPs, reports, and address history are retained.",
        { exact: false },
      ),
    ).toBeInTheDocument();
    fireEvent.click(
      within(screen.getByRole("alertdialog")).getByRole("button", {
        name: "Delete proxy",
      }),
    );
    await waitFor(() =>
      expect(deleteProxyMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16",
        session.csrfToken,
      ),
    );
  });

  it("creates one node proxy and starts both family checks", async () => {
    getSessionMock.mockResolvedValue(session);
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
        publicAddresses: [],
        registeredAt: "2026-08-09T06:00:00Z",
      },
    ]);
    getNodeNetworkMock.mockResolvedValue({
      publicAddresses: [],
      networkProxies: [],
      addressEvents: [],
      addressGaps: [],
    });
    createProxyMock.mockResolvedValue({
      id: "6fc6d7e8-bc63-49e2-91fc-d4c58b43ac16",
      nodeId: "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
      name: "Automatic proxy",
      scheme: "http",
      host: "proxy.example.test",
      port: 8080,
      passwordConfigured: false,
      status: "checking",
      ipv4: { status: "checking" },
      ipv6: { status: "checking" },
      createdAt: "2026-08-09T06:00:00Z",
      updatedAt: "2026-08-09T06:00:00Z",
    });

    renderApplication("/nodes/7289cfa3-a75d-4a3f-ac06-8f1074446a85/network");

    expect(
      await screen.findByText("This node has no network proxy."),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText("Name", { exact: true }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add proxy" }));
    const createDialog = screen.getByRole("dialog", { name: "Add proxy" });
    fireEvent.change(
      within(createDialog).getByLabelText("Name", { exact: true }),
      {
        target: { value: "Automatic proxy" },
      },
    );
    fireEvent.change(
      within(createDialog).getByLabelText("Host or IP address"),
      {
        target: { value: "proxy.example.test" },
      },
    );
    fireEvent.click(
      within(createDialog).getByRole("button", { name: "Add proxy" }),
    );
    await waitFor(() =>
      expect(createProxyMock).toHaveBeenCalledWith(
        "7289cfa3-a75d-4a3f-ac06-8f1074446a85",
        {
          name: "Automatic proxy",
          scheme: "http",
          host: "proxy.example.test",
          port: 8080,
          username: undefined,
          password: undefined,
        },
        session.csrfToken,
      ),
    );
    expect(await screen.findByText("Automatic proxy")).toBeInTheDocument();
    expect(
      screen.queryByRole("dialog", { name: "Add proxy" }),
    ).not.toBeInTheDocument();
    expect(screen.getAllByText("Checking")).toHaveLength(3);
    expect(screen.queryByLabelText("Address family")).not.toBeInTheDocument();
  });

  it("searches and saves an explicit probe timezone", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    const probeState = {
      nodeId: probeTestNode.id,
      schedule: { enabled: true, cron: "0 0 0 * * *", timezone: "UTC" },
      lowMemoryOverride: false,
      probeOnNewAddress: true,
      pausedLowMemory: false,
      recentRuns: [],
    };
    getNodeProbeMock.mockResolvedValue(probeState);
    updateProbeSettingsMock.mockResolvedValue({
      ...probeState,
      schedule: { ...probeState.schedule, timezone: "Asia/Shanghai" },
    });

    renderApplication(`/nodes/${probeTestNode.id}/probe`);

    const timezone = await screen.findByRole("combobox", {
      name: "Time zone",
    });
    expect(timezone).toHaveTextContent("UTC");
    fireEvent.click(timezone);
    fireEvent.change(screen.getByPlaceholderText("Search time zones..."), {
      target: { value: "Asia/Shanghai" },
    });
    fireEvent.click(await screen.findByText("Asia/Shanghai"));
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
            timezone: "Asia/Shanghai",
          },
          lowMemoryOverride: false,
          probeOnNewAddress: true,
        },
        session.csrfToken,
      ),
    );
  });

  it("previews the next scheduled run while editing the Cron expression", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    getNodeProbeMock.mockResolvedValue({
      nodeId: probeTestNode.id,
      schedule: { enabled: true, cron: "0 0 0 * * *", timezone: "UTC" },
      lowMemoryOverride: false,
      probeOnNewAddress: true,
      pausedLowMemory: false,
      recentRuns: [],
    });
    previewProbeScheduleMock.mockReset();
    previewProbeScheduleMock
      .mockResolvedValueOnce({ nextScheduledAt: "2026-08-11T00:00:00Z" })
      .mockResolvedValueOnce({ nextScheduledAt: "2026-08-11T00:30:00Z" })
      .mockRejectedValueOnce(
        new APIError(400, { code: "invalid_probe_settings" }),
      );

    renderApplication(`/nodes/${probeTestNode.id}/probe`);

    expect(
      await screen.findByText(/Aug 11, 2026.*12:00:00 AM.*UTC/),
    ).toBeInTheDocument();
    const cron = screen.getByLabelText("Cron expression");
    fireEvent.change(cron, { target: { value: "0 30 0 * * *" } });
    expect(
      await screen.findByText(/Aug 11, 2026.*12:30:00 AM.*UTC/),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(previewProbeScheduleMock).toHaveBeenLastCalledWith(
        "0 30 0 * * *",
        "UTC",
        expect.any(AbortSignal),
      ),
    );

    fireEvent.change(cron, { target: { value: "0 0 0 * *" } });
    expect(
      await screen.findByText("The Cron expression or time zone is invalid"),
    ).toBeInTheDocument();
  });

  it("configures a low-memory node and creates an immediate probe task", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    const pausedState = {
      nodeId: probeTestNode.id,
      schedule: { enabled: true, cron: "0 0 0 * * *", timezone: "UTC" },
      lowMemoryOverride: false,
      probeOnNewAddress: true,
      physicalMemoryBytes: 64 * 1024 * 1024,
      pausedLowMemory: true,
      recentRuns: [],
    };
    const enabledState = {
      ...pausedState,
      lowMemoryOverride: true,
      pausedLowMemory: false,
    };
    getNodeProbeMock
      .mockResolvedValueOnce(pausedState)
      .mockResolvedValueOnce(pausedState)
      .mockResolvedValue(enabledState);
    updateProbeSettingsMock.mockResolvedValue(enabledState);
    getNodeNetworkMock.mockResolvedValue({
      publicAddresses: [
        {
          id: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          address: "8.8.8.8",
          family: "ipv4",
          probeEnabled: true,
          available: true,
          selectedNodeId: probeTestNode.id,
          selectedNodeName: probeTestNode.name,
          pathCount: 1,
          likelyNat: false,
          proxyPath: false,
          firstSeenAt: "2026-08-09T11:00:00Z",
          lastSeenAt: "2026-08-09T12:00:00Z",
        },
      ],
      networkProxies: [],
      addressEvents: [],
      addressGaps: [],
    });
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
    fireEvent.click(screen.getByRole("tab", { name: "Settings" }));
    fireEvent.click(
      await screen.findByRole("switch", {
        name: "Allow probes below 256 MiB",
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Save policies" }));
    await waitFor(() =>
      expect(updateProbeSettingsMock).toHaveBeenCalledWith(
        probeTestNode.id,
        {
          schedule: {
            enabled: true,
            cron: "0 0 0 * * *",
            timezone: "UTC",
          },
          lowMemoryOverride: true,
          probeOnNewAddress: true,
        },
        session.csrfToken,
      ),
    );
    fireEvent.click(screen.getByRole("tab", { name: "Probes" }));
    expect(
      await screen.findByRole("button", { name: "Run complete probe" }),
    ).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Run complete probe" }));
    expect(
      await screen.findByRole("checkbox", { name: /8\.8\.8\.8/ }),
    ).toBeChecked();
    const probeDialog = screen.getByRole("alertdialog");
    fireEvent.click(
      within(probeDialog).getByRole("button", { name: "Run probe" }),
    );
    await waitFor(() =>
      expect(createProbeTaskMock).toHaveBeenCalledWith(
        probeTestNode.id,
        {
          publicAddressIds: ["a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9"],
        },
        session.csrfToken,
      ),
    );
    expect(await screen.findByText("Waiting for Agent")).toBeInTheDocument();
  });

  it("shows partial probe runs without exposing unavailable public-IP IDs", async () => {
    getSessionMock.mockResolvedValue(session);
    listNodesMock.mockResolvedValue([probeTestNode]);
    getProbeRunMock.mockResolvedValue({
      id: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
      nodeId: probeTestNode.id,
      owner: { nodeName: probeTestNode.name, nodeDeleted: false },
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
          publicAddress: "8.8.8.8",
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
          publicAddress: "2001:4860:4860::8888",
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
      publicAddresses: [
        {
          id: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          address: "8.8.8.8",
          family: "ipv4",
          probeEnabled: true,
          available: true,
          pathCount: 1,
          likelyNat: false,
          proxyPath: false,
          firstSeenAt: "2026-08-09T11:00:00Z",
          lastSeenAt: "2026-08-09T12:00:00Z",
        },
      ],
      networkProxies: [],
      addressEvents: [],
      addressGaps: [],
    });

    renderApplication("/probe-runs/84e7d535-e04e-47f9-8374-1585a5dce6c9");

    expect(
      await screen.findByText("This run completed with partial success"),
    ).toBeInTheDocument();
    expect(screen.getByText("8.8.8.8")).toBeInTheDocument();
    expect(screen.getByText("2001:4860:4860::8888")).toBeInTheDocument();
    expect(
      screen.queryByText("da1a3999-e0bd-4649-85ae-aa9a4a9d6961"),
    ).not.toBeInTheDocument();
    expect(screen.getByText("exit status 1")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Open report snapshot" }),
    ).toHaveAttribute(
      "href",
      "/probe-snapshots/cd6233d2-a600-443b-9cf5-a0bc3c241ea5?runId=84e7d535-e04e-47f9-8374-1585a5dce6c9",
    );
  });

  it("opens a retained probe run after its node was deleted", async () => {
    getSessionMock.mockResolvedValue(session);
    getProbeRunMock.mockResolvedValue({
      id: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
      nodeId: "cf6e7da4-4072-4ca5-a048-91ccfebeb537",
      owner: { nodeName: "retired-edge", nodeDeleted: true },
      configurationRevision: 2,
      historyGeneration: "a".repeat(64),
      trigger: "manual",
      startedAt: "2026-08-09T11:59:00Z",
      completedAt: "2026-08-09T12:00:00Z",
      status: "succeeded",
      expectedExecutions: 1,
      executions: [
        {
          id: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
          runId: "84e7d535-e04e-47f9-8374-1585a5dce6c9",
          egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
          publicAddress: "8.8.8.8",
          ordinal: 0,
          sequence: 1,
          status: "succeeded",
          startedAt: "2026-08-09T11:59:01Z",
          completedAt: "2026-08-09T11:59:20Z",
          snapshotId: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
        },
      ],
    });

    renderApplication("/probe-runs/84e7d535-e04e-47f9-8374-1585a5dce6c9");

    expect(await screen.findByText("retired-edge")).toBeInTheDocument();
    expect(screen.getByText("Node deleted")).toBeInTheDocument();
    expect(screen.getByText("8.8.8.8")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Back to history" }),
    ).toHaveAttribute("href", "/history?tab=reports");
    expect(listNodesMock).not.toHaveBeenCalled();
    expect(getNodeNetworkMock).not.toHaveBeenCalled();
  });

  it("renders the exact decoded probe JSON", async () => {
    getSessionMock.mockResolvedValue(session);
    const rawReport = {
      Head: { IP: "203.0.113.10", Version: "v2026-08-10" },
      Info: {
        ASN: "AS3462",
        Organization: "Example Network",
        Region: { Code: "TW", Name: "Taiwan" },
      },
      Score: { IP2LOCATION: "0", SCAMALYTICS: "75" },
    };
    const availableField = (
      path: string,
      value: string,
      actualType: "string" | "number" | "boolean" = "string",
    ) => ({
      id: path,
      group: path.split(".")[0],
      path,
      expectedTypes: [actualType],
      status: "available" as const,
      actualType,
      value,
    });
    const snapshot = {
      id: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
      executionId: "cd6233d2-a600-443b-9cf5-a0bc3c241ea5",
      egressId: "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
      sequence: 1,
      observedAt: "2026-08-09T11:59:20Z",
      rawResult: window.btoa(JSON.stringify(rawReport)),
      starred: false,
      fields: [
        availableField("Head.IP", "203.0.113.10"),
        availableField("Head.Version", "v2026-08-10"),
        availableField("Info.ASN", "AS3462"),
        availableField("Info.Organization", "Example Network"),
        availableField("Info.Region.Code", "TW"),
        availableField("Info.Region.Name", "Taiwan"),
        availableField("Info.Type", "Geo-consistent"),
        availableField("Type.Usage.IPinfo", "Hosting"),
        availableField("Type.Usage.ipapi", "ISP"),
        availableField("Type.Company.IPinfo", "Business"),
        availableField("Factor.CountryCode.IPQS", "TW"),
        availableField("Score.IP2LOCATION", "33"),
        availableField("Score.SCAMALYTICS", "75"),
        availableField("Score.ipapi", "0.47%"),
        availableField("Score.AbuseIPDB", "75"),
        availableField("Score.IPQS", "75"),
        availableField("Score.DBIP", "100"),
        availableField("Factor.Proxy.IP2LOCATION", "false", "boolean"),
        availableField("Factor.Proxy.ipapi", "false", "boolean"),
        availableField("Factor.VPN.IP2LOCATION", "true", "boolean"),
        {
          id: "Factor.Proxy.DBIP",
          group: "Factor",
          path: "Factor.Proxy.DBIP",
          expectedTypes: ["boolean" as const],
          status: "unavailable" as const,
          actualType: "null" as const,
        },
        availableField("Media.Netflix.Status", "Yes"),
        availableField("Media.Netflix.Region", "TW"),
        availableField("Media.Netflix.Type", "Native"),
        availableField("Media.Reddit.Status", "Block"),
        availableField("Media.DisneyPlus.Status", "Pending"),
        availableField("Media.DisneyPlus.Type", "ViaDNS"),
        availableField("Mail.Port25", "false", "boolean"),
        availableField("Mail.Gmail", "true", "boolean"),
        availableField("Mail.Outlook", "false", "boolean"),
        availableField("Mail.DNSBlacklist.Total", "439", "number"),
        availableField("Mail.DNSBlacklist.Clean", "411", "number"),
        availableField("Mail.DNSBlacklist.Marked", "26", "number"),
        availableField("Mail.DNSBlacklist.Blacklisted", "2", "number"),
        {
          id: "Info.Latitude",
          group: "Info",
          path: "Info.Latitude",
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
          path: "Info.Latitude",
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

    expect(await screen.findByRole("tab", { name: "Report" })).toHaveAttribute(
      "data-state",
      "active",
    );
    expect(
      screen.getByText("203.0.113.10", { exact: true }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Basic information" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Example Network")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "IP type attributes" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("row", { name: /^Usage Hosting ISP/ }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("row", { name: /^Company Business/ }),
    ).toBeInTheDocument();
    expect(screen.getByText("Hosting")).toHaveClass("text-destructive");
    expect(screen.getByText("Business")).toHaveClass("text-amber-700");
    expect(screen.getByText("ISP")).toHaveClass("text-emerald-700");
    expect(
      screen.getByRole("heading", { name: "Risk scores" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Export PNG" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Copy PNG" }),
    ).toBeInTheDocument();
    toBlobMock.mockRejectedValueOnce(new Error("canvas failed"));
    fireEvent.click(screen.getByRole("button", { name: "Export PNG" }));
    expect(
      await screen.findByText("The PNG could not be generated. Try again."),
    ).toBeInTheDocument();
    const riskScoresCard = screen
      .getByRole("heading", { name: "Risk scores" })
      .closest<HTMLElement>('[data-slot="card"]');
    expect(riskScoresCard).not.toBeNull();
    const expectRiskTone = (
      provider: string,
      level: string,
      className: string,
    ) => {
      const row = within(riskScoresCard!).getByText(provider).parentElement;
      expect(row).not.toBeNull();
      expect(within(row!).getByText(level)).toHaveClass(className);
    };
    expectRiskTone("IP2LOCATION", "Medium", "text-amber-700");
    expectRiskTone("SCAMALYTICS", "High", "text-destructive");
    expectRiskTone("ipapi", "Low", "text-emerald-700");
    expectRiskTone("AbuseIPDB", "Block recommended", "text-destructive");
    expectRiskTone("IPQS", "Suspicious", "text-amber-700");
    expectRiskTone("DBIP", "High", "text-destructive");
    const riskFactorsCard = screen
      .getByRole("heading", { name: "Risk factors" })
      .closest<HTMLElement>('[data-slot="card"]');
    expect(riskFactorsCard).not.toBeNull();
    expect(
      within(riskFactorsCard!).getByRole("row", { name: /^Region TW/ }),
    ).toBeInTheDocument();
    expect(within(riskFactorsCard!).getByText("TW")).toHaveClass(
      "text-emerald-700",
    );
    expect(screen.getAllByText("No").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Yes").length).toBeGreaterThan(0);
    expect(
      document.querySelector('[data-report-path="Factor.Proxy.DBIP"]'),
    ).toHaveTextContent("—");
    expect(
      screen.getByRole("columnheader", { name: "Netflix" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Unlocked")).toBeInTheDocument();
    expect(screen.getAllByText("Pending")[0]).toHaveClass("text-amber-700");
    expect(screen.getByText("ViaDNS")).toHaveClass("text-amber-700");
    expect(screen.getByText("Gmail · Reachable")).toBeInTheDocument();
    expect(screen.getByText("439")).toHaveClass("text-cyan-700");
    expect(screen.getByText("411")).toHaveClass("text-emerald-700");
    expect(screen.getByText("26")).toHaveClass("text-amber-700");
    expect(screen.getByText("2", { exact: true })).toHaveClass(
      "text-destructive",
    );
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
    fireEvent.mouseDown(
      screen.getByRole("tab", { name: "Format diagnostics 2" }),
      { button: 0, ctrlKey: false },
    );
    expect(await screen.findByText("Info.Latitude")).toBeInTheDocument();
    expect(screen.getAllByText("Missing field").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Incompatible type").length).toBeGreaterThan(0);
    fireEvent.mouseDown(await screen.findByRole("tab", { name: "Raw JSON" }), {
      button: 0,
      ctrlKey: false,
    });
    await waitFor(() =>
      expect(document.querySelector("pre")).toHaveTextContent(
        '"IP": "203.0.113.10"',
      ),
    );
    expect(
      screen.getByRole("button", { name: "Copy JSON" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Download JSON" }),
    ).toBeInTheDocument();
  });

  it("indexes retained reports and links the egress comparison", async () => {
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
          owner: {
            nodeName: "edge-1",
            egressName: "Default IPv4",
            nodeDeleted: false,
          },
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
      screen.getAllByRole("link", { name: "Compare snapshots" })[0],
    ).toHaveAttribute(
      "href",
      "/history/compare?egress=a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9",
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
          owner: {
            nodeName: "edge-1",
            egressName: "203.0.113.10",
            nodeDeleted: false,
          },
          event: {
            id: "758db6d8-d8cd-44c5-a18d-ab7713012ec8",
            sequence: 3,
            kind: "address-added",
            family: "ipv4",
            publicAddress: "203.0.113.10",
            observedAt: "2026-08-09T12:00:00Z",
          },
        },
      ],
      gaps: [],
    });

    renderApplication(
      "/history?tab=addresses&from=2026-08-01T00%3A00&to=2026-08-10T00%3A00&eventKind=address-added&family=ipv4",
    );

    expect(
      (
        await screen.findAllByText("203.0.113.10", {
          exact: false,
        })
      ).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText("eth0")).not.toBeInTheDocument();
    expect(listHistoryAddressesMock).toHaveBeenCalledWith(
      {
        from: "2026-08-01T00:00:00.000Z",
        to: "2026-08-10T00:00:00.000Z",
        page: 1,
        gapPage: 1,
        pageSize: 25,
        eventKind: "address-added",
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
          owner: {
            nodeName: "edge-1",
            egressName: "Default IPv4",
            nodeDeleted: false,
          },
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
          owner: {
            nodeName: "edge-1",
            egressName: "Default IPv4",
            nodeDeleted: false,
          },
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

  it("defaults snapshot comparison to the earliest and latest reports", async () => {
    getSessionMock.mockResolvedValue(session);
    const egressId = "a6a2f052-f9c4-4f37-88d5-4dc4c95d68d9";
    const firstId = "9278587a-e1a9-4fe4-a5fc-5ece010c8a9f";
    const lastId = "cd6233d2-a600-443b-9cf5-a0bc3c241ea5";
    listHistorySnapshotsMock.mockResolvedValue({
      total: 3,
      items: [
        timelineSnapshot(lastId, egressId, 3, "2026-08-09T12:00:00Z"),
        timelineSnapshot(
          "f6f79d7e-bebb-4fae-bf0f-3bcb2c8ea668",
          egressId,
          2,
          "2026-08-08T12:00:00Z",
        ),
        timelineSnapshot(firstId, egressId, 1, "2026-08-07T12:00:00Z"),
      ],
    });
    compareSnapshotsMock.mockResolvedValue({
      beforeId: firstId,
      afterId: lastId,
      egressId,
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

    renderApplication(`/history/compare?egress=${egressId}`);

    expect(
      await screen.findByRole("heading", { name: "Snapshot comparison" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("3 snapshots")).toBeInTheDocument();
    expect(
      screen.getByRole("slider", { name: "Start snapshot" }),
    ).toHaveAttribute("aria-valuenow", "0");
    expect(
      screen.getByRole("slider", { name: "End snapshot" }),
    ).toHaveAttribute("aria-valuenow", "2");
    await waitFor(() =>
      expect(compareSnapshotsMock).toHaveBeenCalledWith(
        firstId,
        lastId,
        expect.any(AbortSignal),
      ),
    );
    expect(screen.getByText("1 change")).toBeInTheDocument();
    expect(screen.getAllByText("203.0.113.1").length).toBeGreaterThan(0);
    expect(screen.getAllByText("—").length).toBeGreaterThan(0);
    await i18n.changeLanguage("zh-CN");
    expect(await screen.findByText("3 份快照")).toBeInTheDocument();
    expect(screen.getByText("1 项变化")).toBeInTheDocument();
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
  publicAddresses: [],
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
    publicAddresses: [],
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
