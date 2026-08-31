import { randomUUID } from "node:crypto";
import { readFile } from "node:fs/promises";

import { expect, test } from "@playwright/test";

async function signIn(page: import("@playwright/test").Page) {
  await page.goto("/system/status");
  await expect(page).toHaveURL(/\/login$/);
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("admin");
  await page.getByRole("button", { name: "Sign in" }).click();

  const switchToEnglish = page.getByRole("button", { name: "切换到英文" });
  if (await switchToEnglish.isVisible()) {
    await switchToEnglish.click();
  }
  await expect(
    page.getByRole("heading", { name: "System status" }),
  ).toBeVisible();
}

async function expectNoPageOverflow(page: import("@playwright/test").Page) {
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
}

function nodeAction(
  page: import("@playwright/test").Page,
  nodeName: string,
  action: string,
) {
  const group = page
    .getByRole("group", {
      name: `Node actions for ${nodeName}`,
      exact: true,
    })
    .filter({ visible: true });
  return group
    .getByRole("button", { name: action, exact: true })
    .or(group.getByRole("link", { name: action, exact: true }))
    .filter({ visible: true })
    .first();
}

async function clickNodeAction(
  page: import("@playwright/test").Page,
  nodeName: string,
  action: string,
) {
  await nodeAction(page, nodeName, action).click();
}

function updateState(channel: "stable" | "rc") {
  const isRC = channel === "rc";
  return {
    channel,
    currentVersion: "0.1.0",
    currentRevision: "1111111111111111111111111111111111111111",
    checkedAt: "2026-08-10T08:00:00Z",
    availableRelease: {
      version: isRC ? "0.2.0-rc.1" : "0.1.1",
      tag: isRC ? "v0.2.0-rc.1" : "v0.1.1",
      channel,
      revision: isRC
        ? "3333333333333333333333333333333333333333"
        : "2222222222222222222222222222222222222222",
      publishedAt: "2026-08-10T07:00:00Z",
      agentCapabilities: ["agent-update-v1"],
    },
    tasks: [],
  };
}

async function uploadSucceededProbeSnapshot(
  page: import("@playwright/test").Page,
  input: {
    credential: string;
    configurationRevision: number;
    historyGeneration: string;
    egressId: string;
    runId: string;
    executionId: string;
    sequence: number;
    startedAt: string;
    completedAt: string;
    raw: unknown;
  },
) {
  const runningRun = {
    id: input.runId,
    nodeConfigurationRevision: input.configurationRevision,
    historyGeneration: input.historyGeneration,
    trigger: "schedule",
    startedAt: input.startedAt,
    status: "running",
    executions: [
      {
        id: input.executionId,
        egressId: input.egressId,
        ordinal: 0,
        sequence: input.sequence,
      },
    ],
  };
  const runUpload = await page.request.post("/api/v1/agent/probe-artifacts", {
    headers: { Authorization: `Bearer ${input.credential}` },
    data: { artifactId: input.runId, revision: 1, run: runningRun },
  });
  expect(runUpload.status()).toBe(200);
  const executionUpload = await page.request.post(
    "/api/v1/agent/probe-artifacts",
    {
      headers: { Authorization: `Bearer ${input.credential}` },
      data: {
        artifactId: input.executionId,
        revision: 2,
        run: {
          ...runningRun,
          status: "succeeded",
          completedAt: input.completedAt,
        },
        execution: {
          id: input.executionId,
          egressId: input.egressId,
          ordinal: 0,
          sequence: input.sequence,
          status: "succeeded",
          startedAt: input.startedAt,
          completedAt: input.completedAt,
          rawResult: Buffer.from(JSON.stringify(input.raw)).toString("base64"),
        },
      },
    },
  );
  expect(executionUpload.status()).toBe(200);
}

test("authenticates and shows status warnings in both languages", async ({
  page,
}, testInfo) => {
  await page.emulateMedia({ colorScheme: "light" });
  await signIn(page);

  await expect(page.getByText("Operational")).toBeVisible();
  await expect(page.getByText("ipchronicle-center")).toBeVisible();
  await expect(
    page.getByText("Default credentials are still active"),
  ).toBeVisible();
  await expect(page.getByText("Browser connection uses HTTP")).toBeVisible();

  await page
    .getByRole("button", { name: "Switch to Simplified Chinese" })
    .click();
  await expect(page.getByRole("heading", { name: "系统状态" })).toBeVisible();
  await expect(page.getByText("运行正常")).toBeVisible();
  await expect(page.getByText("仍在使用默认凭据")).toBeVisible();
  await expect(page.getByText("浏览器正在使用 HTTP 连接")).toBeVisible();

  await page.screenshot({
    path: testInfo.outputPath("system-status.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "切换到英文" }).click();
  await expect(
    page.getByRole("heading", { name: "System status" }),
  ).toBeVisible();
});

test("changes theme immediately", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");

  await page.getByRole("button", { name: "Use dark theme" }).click();
  await expect(page.locator("html")).toHaveClass(/dark/);
});

test("switches the release channel from system settings", async ({ page }) => {
  await signIn(page);
  let channel: "stable" | "rc" = "stable";
  await page.route("**/api/v1/agent-updates/channel", async (route) => {
    const request = route.request().postDataJSON() as {
      channel: "stable" | "rc";
    };
    channel = request.channel;
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(updateState(channel)),
    });
  });
  await page.route("**/api/v1/agent-updates", async (route) => {
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(updateState(channel)),
    });
  });

  const systemLink = page.getByRole("link", { name: "System", exact: true });
  if (!(await systemLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await systemLink.click();
  await expect(page.getByRole("heading", { name: "System" })).toBeVisible();
  await expect(page.getByText("0.1.1")).toBeVisible();
  await expect(
    page.getByText("2222222222222222222222222222222222222222"),
  ).toBeVisible();

  await page.getByRole("combobox", { name: "Discovery channel" }).click();
  await page.getByRole("option", { name: "Release candidate" }).click();
  await expect(page.getByText("0.2.0-rc.1")).toBeVisible();
  await expectNoPageOverflow(page);
});

test("customizes and restores the automatic external address", async ({
  page,
}, testInfo) => {
  await signIn(page);
  const systemLink = page.getByRole("link", { name: "System", exact: true });
  if (!(await systemLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await systemLink.click();

  const automatic = page.getByRole("switch", {
    name: "Use this browser's current address",
  });
  const externalAddress = page.getByLabel("Custom external address");
  await expect(automatic).toBeChecked();
  await expect(externalAddress).toBeDisabled();
  await expect(externalAddress).toHaveValue(new URL(page.url()).origin);

  await automatic.click();
  await externalAddress.fill("https://ip.example.com");
  await page.getByRole("button", { name: "Save address" }).click();
  await expect(
    page.getByText("External address settings saved."),
  ).toBeVisible();
  await page.reload();
  await expect(automatic).not.toBeChecked();
  await expect(externalAddress).toHaveValue("https://ip.example.com");

  await page.screenshot({
    path: testInfo.outputPath("system-settings.png"),
    fullPage: true,
  });
  await expectNoPageOverflow(page);

  await automatic.click();
  await page.getByRole("button", { name: "Save address" }).click();
  await expect(
    page.getByText("External address settings saved."),
  ).toBeVisible();
  await page.reload();
  await expect(automatic).toBeChecked();
  await expect(externalAddress).toHaveValue(new URL(page.url()).origin);
});

test("generates an Agent installation command from the nodes page", async ({
  page,
}, testInfo) => {
  const fixtureSuffix =
    testInfo.project.name === "mobile-chromium" ? "mobile" : "desktop";
  const nodeName = `edge-e2e-${fixtureSuffix}`;
  const proxyName = `E2E proxy ${fixtureSuffix}`;
  const publicAddress = fixtureSuffix === "mobile" ? "8.8.4.4" : "8.8.8.8";
  await signIn(page);
  const nodesLink = page.getByRole("link", { name: "Nodes", exact: true });
  if (!(await nodesLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await nodesLink.click();
  await expect(page.getByRole("heading", { name: "Nodes" })).toBeVisible();

  const generate = page.getByRole("button", { name: "Generate key" });
  if (await generate.isVisible()) {
    await generate.click();
  }
  await expect(
    page.getByText("Installation command", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("install-agent.sh", { exact: false }),
  ).toBeVisible();
  await page.evaluate(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: async () => undefined },
    });
  });
  const copyCommand = page.getByRole("button", { name: "Copy command" });
  await copyCommand.click();
  await expect(copyCommand).toHaveText("Copy command");
  await expect(page.getByRole("tooltip")).toHaveText(
    "Installation command copied.",
  );
  const installationCommand = await page.locator("pre code").textContent();
  const registrationKey = installationCommand?.match(
    /--registration-key '([^']+)'/,
  )?.[1];
  expect(registrationKey).toBeTruthy();
  const registration = await page.request.post("/api/v1/agent/enroll", {
    data: {
      registrationKey,
      metadata: {
        hostname: nodeName,
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v8",
          "complete-probe-v1",
          "network-inventory-v1",
          "sync-wakeup-v1",
        ],
      },
    },
  });
  expect(registration.status()).toBe(201);
  const registered = (await registration.json()) as {
    credential: string;
    nodeId: string;
  };

  const responsiveItem = (text: string | RegExp) => {
    const items = page.getByText(text, { exact: true });
    return testInfo.project.name === "mobile-chromium"
      ? items.last()
      : items.first();
  };
  await expect(responsiveItem(nodeName)).toBeVisible({ timeout: 8_000 });
  await responsiveItem("Offline").click();
  await expect(page.getByRole("tab", { name: "Overview" })).toHaveAttribute(
    "data-state",
    "active",
  );
  await page.getByRole("button", { name: "Start temporary sync" }).click();
  await expect(
    page.getByText("Waiting for Agent", { exact: true }),
  ).toBeVisible();

  const poll = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: 0,
      metadata: {
        hostname: nodeName,
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v8",
          "complete-probe-v1",
          "network-inventory-v1",
          "sync-wakeup-v1",
        ],
      },
      networkInventory: {
        capturedAt: "2026-08-09T06:00:00Z",
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
          {
            interfaceName: "eth0",
            family: "ipv6",
            destination: "::/0",
            gateway: "fe80::1",
            metric: 100,
            default: true,
          },
        ],
      },
    },
  });
  expect(poll.status()).toBe(200);
  const configurationResponse = await page.request.get(
    "/api/v1/agent/configuration",
    { headers: { Authorization: `Bearer ${registered.credential}` } },
  );
  expect(configurationResponse.status()).toBe(200);
  const configuration = (await configurationResponse.json()) as {
    revision: number;
    historyGeneration: string;
    discoveryPaths: Array<{ id: string; family: "ipv4" | "ipv6" }>;
  };
  const ipv4Path = configuration.discoveryPaths.find(
    (path) => path.family === "ipv4",
  );
  expect(ipv4Path).toBeTruthy();
  const observedAt = "2026-08-09T06:02:00Z";
  const addressPoll = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: configuration.revision,
      metadata: {
        hostname: nodeName,
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v8",
          "network-inventory-v1",
          "address-observation-v1",
          "complete-probe-v1",
          "sync-wakeup-v1",
        ],
      },
      addressStates: [
        {
          egressId: ipv4Path?.id,
          historyGeneration: configuration.historyGeneration,
          family: "ipv4",
          status: "confirmed",
          sequence: 1,
          publicAddress,
          localInterface: "eth0",
          localAddress: "10.0.0.5",
          proxyPath: false,
          likelyNat: true,
          temporary: false,
          lastCheckedAt: observedAt,
          lastSucceededAt: observedAt,
          lastChangedAt: observedAt,
        },
      ],
      addressEvents: [
        {
          id: randomUUID(),
          egressId: ipv4Path?.id,
          historyGeneration: configuration.historyGeneration,
          sequence: 1,
          kind: "first-observation",
          family: "ipv4",
          publicAddress,
          localInterface: "eth0",
          localAddress: "10.0.0.5",
          proxyPath: false,
          likelyNat: true,
          temporary: false,
          observedAt,
        },
      ],
      addressGaps: [],
    },
  });
  expect(addressPoll.status()).toBe(200);
  await page.reload();
  await expect(responsiveItem("Using normal polling")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("sync-degraded.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Stop temporary sync" }).click();
  await expect(
    page.getByRole("button", { name: "Start temporary sync" }),
  ).toBeVisible();

  const networkSettingsLink = page.getByRole("link", {
    name: "Network probes",
    exact: true,
  });
  if (!(await networkSettingsLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await networkSettingsLink.click();
  await expect(
    page.getByRole("heading", { name: "Network probes" }),
  ).toBeVisible();
  await page
    .getByLabel("IPv4 services")
    .fill("http://one.example/ip\nhttps://two.example/ip");
  await page
    .getByLabel("IPv6 services")
    .fill("https://six-one.example/ip\nhttps://six-two.example/ip");
  await expect(page.getByText("A discovery service uses HTTP")).toBeVisible();
  await page.getByRole("button", { name: "Save discovery services" }).click();
  await expect
    .poll(async () => {
      const response = await page.request.get(
        "/api/v1/network-observation-settings",
      );
      const body = (await response.json()) as { ipv4Services: string[] };
      return body.ipv4Services[0];
    })
    .toBe("http://one.example/ip");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("network-proxies.png"),
    fullPage: true,
  });

  const returnToNodes = page.getByRole("link", { name: "Nodes", exact: true });
  if (!(await returnToNodes.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await returnToNodes.click();
  await expect(page.getByRole("heading", { name: "Nodes" })).toBeVisible();

  await clickNodeAction(page, nodeName, "Run probe");
  const targetDialog = page.getByRole("alertdialog");
  const targetCheckbox = targetDialog.getByRole("checkbox", {
    name: new RegExp(publicAddress),
  });
  await expect(targetCheckbox).toBeChecked();
  await expect(page).toHaveURL(/\/nodes$/);
  await expect(targetDialog).toBeVisible();
  await targetDialog.getByRole("button", { name: "Cancel" }).click();

  await page
    .getByRole("link", { name: nodeName, exact: true })
    .filter({ visible: true })
    .first()
    .click();
  await page.getByRole("tab", { name: "Public IPs" }).click();
  await expect(page.getByRole("heading", { name: nodeName })).toBeVisible();
  await expect(page.getByText(publicAddress, { exact: true })).toBeVisible();
  await expect(page.getByText("Reached through NAT")).toBeVisible();
  await expect(page.getByText("eth0")).toHaveCount(0);
  await expect(
    page.getByRole("switch", { name: "Enable complete probe" }),
  ).toBeChecked();
  await page.getByRole("button", { name: "Add proxy" }).click();
  const proxyDialog = page.getByRole("dialog", { name: "Add proxy" });
  await proxyDialog.getByLabel("Name", { exact: true }).fill(proxyName);
  await proxyDialog.getByLabel("Protocol").click();
  await page.getByRole("option", { name: "SOCKS5" }).click();
  await proxyDialog.getByLabel("Host or IP address").fill("proxy.example.test");
  await proxyDialog.getByLabel("Port").fill("1080");
  await proxyDialog.getByLabel("Username").fill("probe-user");
  await proxyDialog.getByLabel("Password").fill("e2e-proxy-secret");
  await proxyDialog.getByRole("button", { name: "Add proxy" }).click();
  await expect(page.getByText(proxyName, { exact: true })).toBeVisible();
  await expect(page.getByText("Password configured")).toBeVisible();
  await expect(page.getByText("IPv4", { exact: true }).last()).toBeVisible();
  await expect(page.getByText("IPv6", { exact: true }).last()).toBeVisible();
  await expect(
    page.getByText("Checking", { exact: true }).first(),
  ).toBeVisible();
  await expect(page.getByLabel("Address family")).toHaveCount(0);
  const inputValues = await page
    .locator("input")
    .evaluateAll((inputs) =>
      inputs.map((input) => (input as HTMLInputElement).value),
    );
  expect(inputValues).not.toContain("e2e-proxy-secret");
  await page.getByRole("button", { name: "Delete proxy" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Delete proxy", exact: true })
    .click();
  await expect(page.getByText("Deleting", { exact: true })).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      ),
    )
    .toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("public-ips.png"),
    fullPage: true,
  });
  await page.getByRole("tab", { name: "Address changes" }).click();
  await expect(
    page.getByText("First observation", { exact: true }),
  ).toBeVisible();
  await expectNoPageOverflow(page);
  await page.screenshot({
    path: testInfo.outputPath("node-address-history.png"),
    fullPage: true,
  });

  await page.getByRole("tab", { name: "Probes" }).click();
  await expect(page.getByRole("heading", { name: nodeName })).toBeVisible();
  await expect(page.getByLabel("Cron expression")).toHaveValue("0 0 0 * * *");
  await page.getByRole("button", { name: "Run complete probe" }).click();
  await expect(
    page.getByRole("checkbox", { name: new RegExp(publicAddress) }),
  ).toBeChecked();
  await expectNoPageOverflow(page);
  await page.screenshot({
    path: testInfo.outputPath("probe-target-selection.png"),
    fullPage: true,
  });
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Run probe" })
    .click();
  await expect(
    page.getByText("The task is waiting for the Agent."),
  ).toBeVisible();

  const probeConfigurationResponse = await page.request.get(
    "/api/v1/agent/configuration",
    { headers: { Authorization: `Bearer ${registered.credential}` } },
  );
  expect(probeConfigurationResponse.status()).toBe(200);
  const probeConfiguration = (await probeConfigurationResponse.json()) as {
    revision: number;
    historyGeneration: string;
    probeTargets: Array<{
      id: string;
      publicAddress: string;
      family: "ipv4" | "ipv6";
    }>;
  };
  const probeTarget = probeConfiguration.probeTargets.find(
    (target) => target.publicAddress === publicAddress,
  );
  expect(probeTarget).toBeTruthy();
  const deliveredTaskResponse = await page.request.post(
    "/api/v1/agent/control",
    {
      headers: { Authorization: `Bearer ${registered.credential}` },
      data: {
        appliedConfigurationRevision: probeConfiguration.revision,
        metadata: {
          hostname: nodeName,
          agentVersion: "0.1.0-e2e",
          operatingSystem: "linux",
          architecture: "amd64",
          physicalMemoryBytes: 536870912,
          capabilities: [
            "control-v1",
            "configuration-v8",
            "network-inventory-v1",
            "address-observation-v1",
            "complete-probe-v1",
            "sync-wakeup-v1",
          ],
        },
      },
    },
  );
  expect(deliveredTaskResponse.status()).toBe(200);
  const deliveredTask = (await deliveredTaskResponse.json()) as {
    task: { id: string; createdAt: string; expiresAt: string };
  };
  expect(deliveredTask.task.id).toBeTruthy();
  const runId = randomUUID();
  const executionId = randomUUID();
  const startedAt = new Date(
    Date.parse(deliveredTask.task.createdAt) + 1000,
  ).toISOString();
  const completedAt = new Date(
    Date.parse(deliveredTask.task.createdAt) + 2000,
  ).toISOString();
  const runningReport = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: probeConfiguration.revision,
      metadata: {
        hostname: nodeName,
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v8",
          "network-inventory-v1",
          "address-observation-v1",
          "complete-probe-v1",
          "sync-wakeup-v1",
        ],
      },
      probeStatus: {
        activeRunId: runId,
        lastOccurrenceAt: startedAt,
        lastOccurrenceTrigger: "manual",
        lastOccurrenceStatus: "started",
      },
      taskReport: {
        id: deliveredTask.task.id,
        status: "running",
        acknowledgedAt: startedAt,
        startedAt,
        runId,
      },
    },
  });
  expect(runningReport.status()).toBe(200);
  const runningRun = {
    id: runId,
    nodeConfigurationRevision: probeConfiguration.revision,
    historyGeneration: probeConfiguration.historyGeneration,
    trigger: "manual",
    taskId: deliveredTask.task.id,
    startedAt,
    status: "running",
    executions: [
      {
        id: executionId,
        egressId: probeTarget?.id,
        ordinal: 0,
        sequence: 1,
      },
    ],
  };
  const runUpload = await page.request.post("/api/v1/agent/probe-artifacts", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: { artifactId: runId, revision: 1, run: runningRun },
  });
  expect(runUpload.status()).toBe(200);
  await page.reload();
  await expect(page.getByText("Agent received")).toBeVisible();
  await expect(page.getByText("Running").first()).toBeVisible();

  const terminalRun = {
    ...runningRun,
    status: "succeeded",
    completedAt,
  };
  const executionUpload = await page.request.post(
    "/api/v1/agent/probe-artifacts",
    {
      headers: { Authorization: `Bearer ${registered.credential}` },
      data: {
        artifactId: executionId,
        revision: 2,
        run: terminalRun,
        execution: {
          id: executionId,
          egressId: probeTarget?.id,
          ordinal: 0,
          sequence: 1,
          status: "succeeded",
          startedAt,
          completedAt,
          rawResult: Buffer.from(
            JSON.stringify({
              Head: {
                IP: publicAddress,
                Time: "2026-08-11 14:53:00 UTC",
                Version: "e2e-1",
              },
              Info: {
                ASN: "AS15169",
                Organization: "Google LLC",
                TimeZone: "America/Chicago",
                City: { Name: "Council Bluffs" },
                Region: { Code: "US", Name: "United States" },
                RegisteredRegion: { Code: "US", Name: "United States" },
                Type: "Geo-consistent",
              },
              Type: {
                Usage: {
                  IPinfo: "Hosting",
                  ipregistry: "Business",
                  ipapi: "ISP",
                  AbuseIPDB: "Line ISP",
                  IP2LOCATION: "Line ISP",
                },
                Company: {
                  IPinfo: "ISP",
                  ipregistry: "ISP",
                  ipapi: "ISP",
                },
              },
              Score: {
                IP2LOCATION: "33",
                SCAMALYTICS: "75",
                ipapi: "0.47%",
                AbuseIPDB: "75",
                IPQS: "75",
                DBIP: "100",
              },
              Factor: {
                CountryCode: {
                  IP2LOCATION: "US",
                  ipapi: "US",
                  ipregistry: "US",
                  IPQS: "US",
                  SCAMALYTICS: "US",
                  ipdata: "US",
                  IPinfo: "US",
                  IPWHOIS: "US",
                  DBIP: "US",
                },
                Proxy: {
                  IP2LOCATION: false,
                  ipapi: false,
                  ipregistry: false,
                  IPQS: false,
                  SCAMALYTICS: false,
                  ipdata: false,
                  IPinfo: false,
                  IPWHOIS: false,
                  DBIP: null,
                },
                Tor: {
                  IP2LOCATION: false,
                  ipapi: false,
                  ipregistry: false,
                  IPQS: false,
                  SCAMALYTICS: false,
                  ipdata: false,
                  IPinfo: false,
                  IPWHOIS: false,
                  DBIP: null,
                },
                VPN: {
                  IP2LOCATION: false,
                  ipapi: false,
                  ipregistry: false,
                  IPQS: false,
                  SCAMALYTICS: false,
                  ipdata: null,
                  IPinfo: false,
                  IPWHOIS: false,
                  DBIP: null,
                },
                Server: {
                  IP2LOCATION: false,
                  ipapi: false,
                  ipregistry: false,
                  IPQS: null,
                  SCAMALYTICS: false,
                  ipdata: false,
                  IPinfo: false,
                  IPWHOIS: false,
                  DBIP: null,
                },
                Abuser: {
                  IP2LOCATION: false,
                  ipapi: false,
                  ipregistry: false,
                  IPQS: false,
                  SCAMALYTICS: false,
                  ipdata: false,
                  IPinfo: null,
                  IPWHOIS: null,
                  DBIP: false,
                },
                Robot: {
                  IP2LOCATION: false,
                  ipapi: false,
                  ipregistry: null,
                  IPQS: false,
                  SCAMALYTICS: false,
                  ipdata: null,
                  IPinfo: null,
                  IPWHOIS: null,
                  DBIP: false,
                },
              },
              Media: {
                TikTok: { Status: "Yes", Region: "US", Type: "Native" },
                DisneyPlus: {
                  Status: "Pending",
                  Region: "US",
                  Type: "ViaDNS",
                },
                Netflix: { Status: "Yes", Region: "US", Type: "Native" },
                Youtube: { Status: "Yes", Region: "US", Type: "Native" },
                AmazonPrimeVideo: {
                  Status: "Yes",
                  Region: "US",
                  Type: "Native",
                },
                Reddit: { Status: "Block", Region: null, Type: "" },
                ChatGPT: { Status: "Yes", Region: "US", Type: "Native" },
              },
              Mail: {
                Port25: false,
                Gmail: false,
                Outlook: false,
                Yahoo: false,
                Apple: false,
                QQ: false,
                MailRU: false,
                AOL: false,
                GMX: false,
                MailCOM: false,
                "163": false,
                Sohu: false,
                Sina: false,
                DNSBlacklist: {
                  Total: 439,
                  Clean: 411,
                  Marked: 26,
                  Blacklisted: 2,
                },
              },
            }),
          ).toString("base64"),
        },
      },
    },
  );
  expect(executionUpload.status()).toBe(200);
  const terminalReport = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: probeConfiguration.revision,
      metadata: {
        hostname: nodeName,
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v8",
          "network-inventory-v1",
          "address-observation-v1",
          "complete-probe-v1",
          "sync-wakeup-v1",
        ],
      },
      probeStatus: {
        lastOccurrenceAt: startedAt,
        lastOccurrenceTrigger: "manual",
        lastOccurrenceStatus: "started",
      },
      taskReport: {
        id: deliveredTask.task.id,
        status: "succeeded",
        acknowledgedAt: startedAt,
        startedAt,
        completedAt,
        runId,
      },
    },
  });
  expect(terminalReport.status()).toBe(200);
  const secondStartedAt = new Date(
    Date.parse(completedAt) + 1000,
  ).toISOString();
  const secondCompletedAt = new Date(
    Date.parse(completedAt) + 2000,
  ).toISOString();
  await uploadSucceededProbeSnapshot(page, {
    credential: registered.credential,
    configurationRevision: probeConfiguration.revision,
    historyGeneration: probeConfiguration.historyGeneration,
    egressId: probeTarget?.id ?? "",
    runId: randomUUID(),
    executionId: randomUUID(),
    sequence: 2,
    startedAt: secondStartedAt,
    completedAt: secondCompletedAt,
    raw: {
      Head: { IP: publicAddress, Version: "e2e-2" },
      Info: { ASN: "AS15169" },
    },
  });
  await page.reload();
  await expect(page.getByText("Succeeded").first()).toBeVisible();
  await page.getByRole("link", { name: "Open run" }).click();
  await expect(
    page.getByRole("heading", { name: "Complete-probe run" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Open report snapshot" }).click();
  await expect(page.getByRole("tab", { name: "Report" })).toHaveAttribute(
    "aria-selected",
    "true",
  );
  await expect(page.getByText(publicAddress, { exact: true })).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Basic information" }),
  ).toBeVisible();
  await expect(page.getByText("AS15169", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "IP type attributes" }),
  ).toBeVisible();
  await expect(
    page.getByRole("row", {
      name: /^Usage Hosting Business ISP Line ISP Line ISP/,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Risk scores" }),
  ).toBeVisible();
  await expect(page.getByText("Hosting", { exact: true })).toHaveClass(
    /text-destructive/,
  );
  await expect(page.getByText("Business", { exact: true })).toHaveClass(
    /text-amber-700/,
  );
  await expect(
    page.getByRole("row", { name: /^Region US US US US US US US US US/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("row", {
      name: /^Status Unlocked Pending Unlocked Unlocked Unlocked Blocked Unlocked/,
    }),
  ).toBeVisible();
  await expect(page.getByText("Pending", { exact: true })).toHaveClass(
    /text-amber-700/,
  );
  await expect(
    page.locator('[data-report-path="Media.Reddit.Region"]'),
  ).toHaveText("—");
  await page.evaluate(() => {
    Object.defineProperty(navigator.clipboard, "write", {
      configurable: true,
      value: async (items: ClipboardItem[]) => {
        const blob = await items[0].getType("image/png");
        const bytes = new Uint8Array(await blob.arrayBuffer());
        (
          window as typeof window & {
            __ipchroniclePNGClipboard?: {
              signature: number[];
              size: number;
              type: string;
            };
          }
        ).__ipchroniclePNGClipboard = {
          signature: Array.from(bytes.slice(0, 8)),
          size: blob.size,
          type: blob.type,
        };
      },
    });
  });
  await page.getByRole("button", { name: "Copy PNG" }).click();
  await expect(page.getByRole("button", { name: "PNG copied" })).toBeVisible();
  const copiedPNG = await page.evaluate(
    () =>
      (
        window as typeof window & {
          __ipchroniclePNGClipboard?: {
            signature: number[];
            size: number;
            type: string;
          };
        }
      ).__ipchroniclePNGClipboard,
  );
  expect(copiedPNG?.type).toBe("image/png");
  expect(copiedPNG?.size).toBeGreaterThan(100_000);
  expect(copiedPNG?.signature).toEqual([137, 80, 78, 71, 13, 10, 26, 10]);
  const pngDownloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export PNG" }).click();
  const pngDownload = await pngDownloadPromise;
  expect(pngDownload.suggestedFilename()).toMatch(
    /^ipchronicle-[0-9a-f-]+\.png$/,
  );
  const pngPath = await pngDownload.path();
  expect(pngPath).not.toBeNull();
  const png = await readFile(pngPath!);
  expect(png.subarray(0, 8)).toEqual(
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
  );
  expect(png.readUInt32BE(16)).toBe(2400);
  expect(png.readUInt32BE(20)).toBeGreaterThan(2000);
  const nonBackgroundPixels = await page.evaluate(async (base64) => {
    const image = new Image();
    image.src = `data:image/png;base64,${base64}`;
    await image.decode();
    const canvas = document.createElement("canvas");
    canvas.width = 240;
    canvas.height = Math.round((image.height / image.width) * canvas.width);
    const context = canvas.getContext("2d");
    if (!context) return 0;
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data;
    let count = 0;
    for (let index = 0; index < pixels.length; index += 4) {
      if (
        pixels[index] < 245 ||
        pixels[index + 1] < 245 ||
        pixels[index + 2] < 245
      ) {
        count += 1;
      }
    }
    return count;
  }, png.toString("base64"));
  expect(nonBackgroundPixels).toBeGreaterThan(1000);
  await pngDownload.saveAs(testInfo.outputPath("probe-export.png"));
  await page.getByRole("button", { name: "Star snapshot" }).click();
  await expect(
    page.getByRole("button", { name: "Unstar snapshot" }),
  ).toBeVisible();
  await expectNoPageOverflow(page);
  await page.screenshot({
    path: testInfo.outputPath("probe-snapshot.png"),
    fullPage: true,
  });
  await page.getByRole("tab", { name: /Format diagnostics/ }).click();
  await expect(page.getByText("Head.Command", { exact: true })).toBeVisible();
  await expect(
    page.getByText("Media.Reddit.Region", { exact: true }),
  ).toHaveCount(0);
  await page.getByRole("tab", { name: "Raw JSON" }).click();
  await expect(page.locator("pre")).toContainText(`"IP": "${publicAddress}"`);
  await expect(page.locator("pre")).toContainText('"Region": null');
  await expectNoPageOverflow(page);

  const historyLink = page.getByRole("link", {
    name: "History",
    exact: true,
  });
  if (!(await historyLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await historyLink.click();
  await expect(page.getByRole("heading", { name: "History" })).toBeVisible();
  await expect(page.getByText("2 retained snapshots")).toBeVisible();
  await expect(responsiveItem(`${publicAddress} · #2`)).toBeVisible();
  await page.getByRole("tab", { name: "Address changes" }).click();
  await expect(responsiveItem("First observation")).toBeVisible();
  await expect(responsiveItem(publicAddress)).toBeVisible();
  await expect(page.getByText("eth0", { exact: true })).toHaveCount(0);
  await expect(page.getByText("10.0.0.5", { exact: true })).toHaveCount(0);
  await page.getByRole("tab", { name: "Probe reports" }).click();
  const compareSnapshots = page.getByRole("link", {
    name: "Compare snapshots",
  });
  await (
    testInfo.project.name === "mobile-chromium"
      ? compareSnapshots.last()
      : compareSnapshots.first()
  ).click();
  await expect(
    page.getByRole("heading", { name: "Snapshot comparison" }),
  ).toBeVisible();
  await expect(page.getByText("2 snapshots")).toBeVisible();
  await expect(
    page.getByRole("slider", { name: "Start snapshot" }),
  ).toHaveAttribute("aria-valuenow", "0");
  await expect(
    page.getByRole("slider", { name: "End snapshot" }),
  ).toHaveAttribute("aria-valuenow", "1");
  await expect(
    page
      .getByRole("region", { name: "Start snapshot" })
      .getByText(publicAddress, { exact: true }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("region", { name: "End snapshot" })
      .getByText(publicAddress, { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("No fields changed between the selected snapshots"),
  ).toBeVisible();
  await expect(
    page.locator('[data-report-path="Head.IP"][data-report-changed]'),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "Switch to Simplified Chinese" })
    .click();
  await expect(page.getByRole("heading", { name: "快照比较" })).toBeVisible();
  await expect(page.getByText("2 份快照")).toBeVisible();
  await expectNoPageOverflow(page);
  const timeline = page.getByTestId("comparison-timeline");
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await expect
    .poll(async () => (await timeline.boundingBox())?.y ?? -1)
    .toBeGreaterThanOrEqual(60);
  await expect
    .poll(async () => (await timeline.boundingBox())?.y ?? 999)
    .toBeLessThan(72);
  await expect(timeline).toBeInViewport();
  await page.screenshot({
    path: testInfo.outputPath("history-comparison-sticky.png"),
  });
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("history-comparison.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "切换到英文" }).click();

  const historySettingsLink = page.getByRole("link", {
    name: "History and storage",
    exact: true,
  });
  if (!(await historySettingsLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await historySettingsLink.click();
  await expect(
    page.getByRole("heading", { name: "History and storage" }),
  ).toBeVisible();
  await page.getByLabel("Policy").click();
  await page.getByRole("option", { name: "Keep by age" }).click();
  await page.getByLabel("Retention days").fill("30");
  await page.getByRole("button", { name: "Save and apply" }).click();
  await expect(
    page.getByText("The retention policy was saved and applied."),
  ).toBeVisible();
  await page.getByRole("button", { name: "Clean now" }).click();
  await expect(
    page.getByText(/Cleanup completed and removed \d+ items/),
  ).toBeVisible();
  await expectNoPageOverflow(page);
  await page.screenshot({
    path: testInfo.outputPath("history-retention.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Clear history" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Clear all history" })
    .click();
  await expect(page.getByText("Never cleared")).not.toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("history-reset.png"),
    fullPage: true,
  });
  const probesNodesLink = page.getByRole("link", {
    name: "Nodes",
    exact: true,
  });
  if (!(await probesNodesLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await probesNodesLink.click();

  await clickNodeAction(page, nodeName, "Pause node");
  await expect(responsiveItem("Disabled")).toBeVisible();
  await expect(responsiveItem(/Pending · \d+\/\d+/)).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("node-actions.png"),
    fullPage: true,
  });

  await page
    .getByRole("link", { name: nodeName, exact: true })
    .filter({ visible: true })
    .first()
    .click();
  await page.getByRole("tab", { name: "Settings" }).click();
  await expect(
    page.getByText("Uninstall Agent", { exact: true }),
  ).toBeVisible();
  await expect(
    page.locator("pre code").filter({ hasText: "--uninstall" }).first(),
  ).not.toContainText("--purge");
  await expect(
    page.locator("pre code").filter({ hasText: "--uninstall --purge" }),
  ).toBeVisible();
  await expectNoPageOverflow(page);
  await page.screenshot({
    path: testInfo.outputPath("node-settings.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "Revoke Agent credential" }).click();
  await expect(
    page.getByRole("heading", {
      name: `Revoke the Agent credential for ${nodeName}?`,
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Revoke credential" }).click();
  await expect(
    page
      .locator('[data-slot="badge"]')
      .filter({ hasText: /^Revoked$/ })
      .first(),
  ).toBeVisible();

  await page.getByRole("button", { name: "Permanently delete node" }).click();
  await expect(
    page.getByText("The Center does not uninstall the Agent service", {
      exact: false,
    }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Permanently delete", exact: true })
    .click();
  await expect
    .poll(async () => {
      const response = await page.request.get("/api/v1/nodes");
      const body = (await response.json()) as { items: unknown[] };
      return body.items.some(
        (node) =>
          typeof node === "object" &&
          node !== null &&
          "name" in node &&
          node.name === nodeName,
      );
    })
    .toBe(false);
  await page.getByRole("link", { name: "Back to nodes" }).click();
  await expect(page.getByText(nodeName, { exact: true })).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("nodes.png"),
    fullPage: true,
  });
});

test("updates one registered Agent and keeps the task phase visible", async ({
  page,
}, testInfo) => {
  await signIn(page);
  const nodeName = `update-${testInfo.project.name}`;
  let task:
    | {
        id: string;
        nodeId: string;
        targetVersion: string;
        status: "pending";
        createdAt: string;
        expiresAt: string;
        offline: boolean;
      }
    | undefined;
  await page.route("**/api/v1/agent-updates*", async (route) => {
    const request = route.request();
    if (request.method() === "GET") {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({
          ...updateState("stable"),
          tasks: task === undefined ? [] : [task],
        }),
      });
      return;
    }
    if (request.method() === "POST") {
      const body = request.postDataJSON() as {
        nodeIds: string[];
        targetVersion: string;
      };
      task = {
        id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
        nodeId: body.nodeIds[0],
        targetVersion: body.targetVersion,
        status: "pending",
        createdAt: "2026-08-10T08:01:00Z",
        expiresAt: "2026-08-10T08:03:00Z",
        offline: false,
      };
      await route.fulfill({
        status: 202,
        contentType: "application/json",
        body: JSON.stringify({
          targetVersion: body.targetVersion,
          items: [{ nodeId: body.nodeIds[0], accepted: true, task }],
        }),
      });
      return;
    }
    await route.continue();
  });

  const nodesLink = page.getByRole("link", { name: "Nodes", exact: true });
  if (!(await nodesLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await nodesLink.click();
  const generate = page.getByRole("button", { name: "Generate key" });
  if (await generate.isVisible()) {
    await generate.click();
  }
  const installationCommand = await page.locator("pre code").textContent();
  const registrationKey = installationCommand?.match(
    /--registration-key '([^']+)'/,
  )?.[1];
  expect(registrationKey).toBeTruthy();
  const metadata = {
    hostname: nodeName,
    agentVersion: "0.1.0",
    sourceRevision: "1111111111111111111111111111111111111111",
    operatingSystem: "linux",
    architecture: "amd64",
    physicalMemoryBytes: 536870912,
    capabilities: ["control-v1", "configuration-v8", "agent-update-v1"],
  } as const;
  const registration = await page.request.post("/api/v1/agent/enroll", {
    data: {
      registrationKey,
      metadata,
    },
  });
  expect(registration.status()).toBe(201);
  const registered = (await registration.json()) as {
    credential: string;
    nodeId: string;
  };
  const poll = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: 0,
      metadata,
    },
  });
  expect(poll.status()).toBe(200);

  await page.reload();
  await expect(
    page.getByText(nodeName, { exact: true }).filter({ visible: true }).first(),
  ).toBeVisible();
  await clickNodeAction(page, nodeName, "Update Agent");
  await expect(
    page
      .getByText("Waiting for Agent", { exact: true })
      .filter({ visible: true }),
  ).toBeVisible();
  await expectNoPageOverflow(page);

  await page
    .getByRole("link", { name: nodeName, exact: true })
    .filter({ visible: true })
    .first()
    .click();
  await page.getByRole("tab", { name: "Settings" }).click();
  await page.getByRole("button", { name: "Permanently delete node" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Permanently delete", exact: true })
    .click();
  await expect
    .poll(async () => {
      const response = await page.request.get("/api/v1/nodes");
      const body = (await response.json()) as {
        items: Array<{ name: string }>;
      };
      return body.items.some((node) => node.name === nodeName);
    })
    .toBe(false);
});

test("shows account validation errors and starts TOTP enrollment", async ({
  page,
}, testInfo) => {
  await signIn(page);
  const accountLink = page.getByRole("link", { name: "Account", exact: true });
  if (!(await accountLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
    await expect(accountLink).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("sidebar-open.png"),
    });
  }
  await accountLink.click();
  await expect(
    page.getByRole("heading", { name: "Account and security" }),
  ).toBeVisible();

  await page.getByLabel("New password").fill("new-password");
  await page.getByLabel("Current password").first().fill("incorrect-password");
  await page.getByRole("button", { name: "Save changes" }).click();
  await expect(
    page.getByText("The current password is incorrect."),
  ).toBeVisible();

  await page.getByLabel("Current password").last().fill("admin");
  await page.getByRole("button", { name: "Enable TOTP" }).click();
  await expect(page.getByLabel("TOTP enrollment QR code")).toBeVisible();
  await expect(page.getByText("Setup key")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Confirm TOTP" }),
  ).toBeVisible();
});
