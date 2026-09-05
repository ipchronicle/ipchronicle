import { expect, test } from "@playwright/test";

test("attention filters and batch probes retain node context", async ({
  page,
}) => {
  const now = "2026-09-05T00:00:00Z";
  const address = {
    id: "ip-1",
    address: "203.0.113.10",
    family: "ipv4",
    probeEnabled: false,
    available: true,
    selectedNodeId: "node-1",
    pathCount: 1,
    likelyNat: true,
    proxyPath: false,
    firstSeenAt: now,
    lastSeenAt: now,
  };
  const node = {
    id: "node-1",
    name: "Tokyo Edge",
    hostname: "edge",
    enabled: true,
    status: "online",
    agentVersion: "0.1.1",
    logLevel: "info",
    operatingSystem: "linux",
    architecture: "amd64",
    capabilities: [],
    configurationStatus: "current",
    desiredConfigurationRevision: 3,
    appliedConfigurationRevision: 3,
    registeredAt: now,
    publicAddresses: [address],
    pausedLowMemory: false,
  };
  const offline = {
    ...node,
    id: "node-2",
    name: "Offline Edge",
    status: "offline",
    publicAddresses: [],
  };
  const attention = [
    "probe",
    "format",
    "discovery",
    "configuration",
    "memory",
    "offline",
    "update",
    "delivery",
    "retention",
  ].map((kind) => ({
    kind,
    count: kind === "probe" ? 15 : 1,
    nodeIds: [kind === "offline" ? offline.id : node.id],
    publicAddressIds: kind === "probe" ? [address.id] : [],
    samples: [node.name],
  }));
  const routes: Record<string, unknown> = {
    "/api/v1/auth/session": {
      account: {
        username: "admin",
        locale: "en",
        usesDefaultCredentials: false,
        totpEnabled: false,
      },
      csrfToken: "test-only",
      expiresAt: "2030-01-01T00:00:00Z",
    },
    "/api/v1/nodes": { items: [node, offline] },
    "/api/v1/agent-enrollment": { enabled: false, hasKey: false },
    "/api/v1/system/settings": {
      externalOriginMode: "automatic",
      effectiveOrigin: "http://localhost:8080",
      releaseChannel: "stable",
    },
    "/api/v1/system/status": {
      service: "ipchronicle-center",
      status: "ok",
      version: "0.1.1",
      configSchemaVersion: 3,
      historySchemaVersion: 1,
      logsSchemaVersion: 1,
      transportSecurity: "https",
      transportWarning: false,
      externalOriginMode: "automatic",
    },
    "/api/v1/overview": {
      checkedAt: now,
      historyOverBudget: false,
      nodes: [node, offline],
      activeTasks: [],
      recentProbeRuns: [],
      recentAddressEvents: [],
      attention,
    },
    "/api/v1/agent-updates": {
      channel: "stable",
      currentVersion: "0.1.1",
      tasks: [],
    },
    "/api/v1/nodes/node-1/network": {
      publicAddresses: [address],
      networkProxies: [],
      addressEvents: [],
      addressGaps: [],
    },
    "/api/v1/nodes/node-1/probe/tasks": {
      id: "task-1",
      nodeId: node.id,
      status: "pending",
      createdAt: now,
      expiresAt: now,
      offline: false,
    },
  };
  const mutations: { path: string; body: unknown }[] = [];
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (!(path in routes)) {
      errors.push(`Unexpected API ${path}`);
      await route.abort();
      return;
    }
    if (request.method() !== "GET")
      mutations.push({ path, body: request.postDataJSON() });
    await route.fulfill({ json: routes[path] });
  });
  await page.goto("/");
  await expect(
    page.getByRole("link", { name: /Latest IP probes failed · 15/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: /History cleanup needs attention/ }),
  ).toBeVisible();
  await page
    .getByRole("link", { name: /Latest IP probes failed · 15/ })
    .click();
  await expect(page).toHaveURL(/attention=probe/);
  await expect(page.getByText("Offline Edge", { exact: true })).toHaveCount(0);
  await expect(
    page
      .getByRole("checkbox", { name: "Select Tokyo Edge", exact: true })
      .filter({ visible: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Clear filters" }).click();
  await page
    .getByRole("checkbox", { name: "Select all filtered nodes" })
    .filter({ visible: true })
    .first()
    .click();
  await expect(page.getByText("2 selected", { exact: true })).toBeVisible();
  await page.getByLabel("Search nodes").fill("Tokyo");
  await expect(page.getByText("2 selected", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Batch probe", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(/Node offline/)).toBeVisible();
  await dialog.locator("summary").filter({ hasText: "Tokyo Edge" }).click();
  await expect(
    dialog.getByRole("checkbox", { name: address.address }),
  ).toBeChecked();
  const box = await dialog.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  await dialog.getByRole("button", { name: "Run probe" }).click();
  await expect(dialog.getByText("Accepted", { exact: true })).toBeVisible();
  await expect(dialog.getByText("Skipped", { exact: true })).toBeVisible();
  expect(mutations).toEqual([
    {
      path: "/api/v1/nodes/node-1/probe/tasks",
      body: { publicAddressIds: [address.id] },
    },
  ]);
  expect(errors).toEqual([]);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
