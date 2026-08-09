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

test("generates an Agent installation command from the nodes page", async ({
  page,
}, testInfo) => {
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
  await expect(page.getByText("No nodes are registered")).toBeVisible();

  const installationCommand = await page.locator("pre code").textContent();
  const registrationKey = installationCommand?.match(
    /--registration-key '([^']+)'/,
  )?.[1];
  expect(registrationKey).toBeTruthy();
  const registration = await page.request.post("/api/v1/agent/enroll", {
    data: {
      registrationKey,
      metadata: {
        hostname: "edge-e2e",
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v5",
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

  await page.reload();
  const responsiveItem = (text: string | RegExp) => {
    const items = page.getByText(text, { exact: true });
    return testInfo.project.name === "mobile-chromium"
      ? items.last()
      : items.first();
  };
  await expect(responsiveItem("edge-e2e")).toBeVisible();
  await page.getByRole("button", { name: "Start temporary sync" }).click();
  await expect(responsiveItem("Waiting for Agent")).toBeVisible();

  const poll = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: 0,
      metadata: {
        hostname: "edge-e2e",
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v5",
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
    egresses: Array<{ id: string; family: "ipv4" | "ipv6" }>;
  };
  const ipv4Egress = configuration.egresses.find(
    (egress) => egress.family === "ipv4",
  );
  expect(ipv4Egress).toBeTruthy();
  const observedAt = "2026-08-09T06:02:00Z";
  const addressPoll = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: configuration.revision,
      metadata: {
        hostname: "edge-e2e",
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v5",
          "network-inventory-v1",
          "address-observation-v1",
          "complete-probe-v1",
          "sync-wakeup-v1",
        ],
      },
      addressStates: [
        {
          egressId: ipv4Egress?.id,
          historyGeneration: configuration.historyGeneration,
          family: "ipv4",
          status: "confirmed",
          sequence: 1,
          publicAddress: "8.8.8.8",
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
          id: "76e22263-00ee-4656-a97a-90d74fd5f86d",
          egressId: ipv4Egress?.id,
          historyGeneration: configuration.historyGeneration,
          sequence: 1,
          kind: "first-observation",
          family: "ipv4",
          publicAddress: "8.8.8.8",
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
  await page.getByLabel("Name", { exact: true }).fill("E2E proxy");
  await page.getByLabel("Protocol").click();
  await page.getByRole("option", { name: "SOCKS5" }).click();
  await page.getByLabel("Host or IP address").fill("proxy.example.test");
  await page.getByLabel("Port").fill("1080");
  await page.getByLabel("Username").fill("probe-user");
  await page.getByLabel("Password").fill("e2e-proxy-secret");
  await page.getByRole("button", { name: "Add proxy" }).click();
  await expect(page.getByText("Password configured")).toBeVisible();
  const inputValues = await page
    .locator("input")
    .evaluateAll((inputs) =>
      inputs.map((input) => (input as HTMLInputElement).value),
    );
  expect(inputValues).not.toContain("e2e-proxy-secret");
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

  await page.getByRole("link", { name: "Network egresses" }).click();
  await expect(page.getByRole("heading", { name: "edge-e2e" })).toBeVisible();
  await expect(page.getByText("Default IPv4").first()).toBeVisible();
  await expect(page.getByText("Temporary IPv6").first()).toBeVisible();
  await expect(
    page.getByText("eth0 · 10.0.0.5 -> 8.8.8.8", { exact: true }).first(),
  ).toBeVisible();
  await expect(page.getByText("Likely NAT")).toBeVisible();
  await expect(
    page.getByText("First observation", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Enable path" }).last(),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Add egress" }).click();
  await expect(page.getByText("Proxy · E2E proxy")).toBeVisible();
  const interval = page
    .getByRole("spinbutton", { name: "Address check interval (seconds)" })
    .first();
  await interval.fill("15");
  await page
    .getByRole("button", { name: "Save address check interval" })
    .first()
    .click();
  await expect(interval).toHaveValue("15");
  const proxyEgress = page
    .getByText("Proxy · E2E proxy", { exact: true })
    .locator(
      "xpath=ancestor::div[contains(@class, 'space-y-4') and contains(@class, 'rounded-md')][1]",
    );
  await proxyEgress
    .getByRole("button", { name: "Permanently delete egress" })
    .click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Permanently delete", exact: true })
    .click();
  await expect(proxyEgress.getByText("Deletion pending")).toBeVisible();
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
    path: testInfo.outputPath("network-egresses.png"),
    fullPage: true,
  });
  await page.getByRole("link", { name: "Back to nodes" }).click();

  await page.getByRole("link", { name: "Complete probes" }).click();
  await expect(page.getByRole("heading", { name: "edge-e2e" })).toBeVisible();
  await expect(page.getByLabel("Cron expression")).toHaveValue("0 0 0 * * *");
  await page.getByRole("button", { name: "Run complete probe" }).click();
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
    egresses: Array<{ id: string; family: "ipv4" | "ipv6" }>;
  };
  const probeEgress = probeConfiguration.egresses.find(
    (egress) => egress.family === "ipv4",
  );
  expect(probeEgress).toBeTruthy();
  const deliveredTaskResponse = await page.request.post(
    "/api/v1/agent/control",
    {
      headers: { Authorization: `Bearer ${registered.credential}` },
      data: {
        appliedConfigurationRevision: probeConfiguration.revision,
        metadata: {
          hostname: "edge-e2e",
          agentVersion: "0.1.0-e2e",
          operatingSystem: "linux",
          architecture: "amd64",
          physicalMemoryBytes: 536870912,
          capabilities: [
            "control-v1",
            "configuration-v5",
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
  const runId = "d3173c4d-d437-49f8-89ac-f6cac3a73df3";
  const executionId = "9c862898-d88c-47af-bd04-52dfb293b9f3";
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
        hostname: "edge-e2e",
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v5",
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
        egressId: probeEgress?.id,
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
          egressId: probeEgress?.id,
          ordinal: 0,
          sequence: 1,
          status: "succeeded",
          startedAt,
          completedAt,
          rawResult: Buffer.from(
            JSON.stringify({
              Head: { IP: "8.8.8.8", Version: "e2e-1" },
              Info: { ASN: "AS15169" },
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
        hostname: "edge-e2e",
        agentVersion: "0.1.0-e2e",
        operatingSystem: "linux",
        architecture: "amd64",
        physicalMemoryBytes: 536870912,
        capabilities: [
          "control-v1",
          "configuration-v5",
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
    egressId: probeEgress?.id ?? "",
    runId: "2d6b34bf-4d3d-452f-a6a3-c2b94c5cb990",
    executionId: "54087de0-787a-4a99-8b75-4f321852e755",
    sequence: 2,
    startedAt: secondStartedAt,
    completedAt: secondCompletedAt,
    raw: {
      Head: { IP: "8.8.4.4", Version: "e2e-2" },
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
  await expect(
    page.getByRole("tab", { name: "Structured fields" }),
  ).toHaveAttribute("aria-selected", "true");
  await expect(page.getByText("IP address").first()).toBeVisible();
  await expect(page.getByText("Head.IP").first()).toBeVisible();
  await expect(page.getByText("8.8.8.8", { exact: true })).toBeVisible();
  await expect(page.getByText("Missing").first()).toBeVisible();
  await page.getByRole("button", { name: "Star snapshot" }).click();
  await expect(
    page.getByRole("button", { name: "Unstar snapshot" }),
  ).toBeVisible();
  await page.getByRole("tab", { name: "Raw JSON" }).click();
  await expect(page.locator("pre")).toContainText('"IP": "8.8.8.8"');
  await expectNoPageOverflow(page);
  await page.screenshot({
    path: testInfo.outputPath("probe-snapshot.png"),
    fullPage: true,
  });

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
  await expect(responsiveItem(/Default IPv4/)).toBeVisible();
  await page.getByRole("tab", { name: "Address changes" }).click();
  await expect(responsiveItem("First observation")).toBeVisible();
  await expect(responsiveItem("eth0 · 10.0.0.5 -> 8.8.8.8")).toBeVisible();
  await page.getByRole("tab", { name: "Probe reports" }).click();
  const comparePrevious = page.getByRole("link", {
    name: "Compare with previous",
  });
  await (
    testInfo.project.name === "mobile-chromium"
      ? comparePrevious.last()
      : comparePrevious.first()
  ).click();
  await expect(
    page.getByRole("heading", { name: "Snapshot comparison" }),
  ).toBeVisible();
  await expect(page.getByText("IP address").first()).toBeVisible();
  await expect(page.getByText("Head.IP").first()).toBeVisible();
  await expect(page.getByText("8.8.8.8", { exact: true })).toBeVisible();
  await expect(page.getByText("8.8.4.4", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "Switch to Simplified Chinese" })
    .click();
  await expect(page.getByRole("heading", { name: "快照比较" })).toBeVisible();
  await expect(page.getByText("IP 地址").first()).toBeVisible();
  await expectNoPageOverflow(page);
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

  await page.getByRole("button", { name: "Pause node" }).click();
  await expect(responsiveItem("Disabled")).toBeVisible();
  await expect(responsiveItem(/Pending · \d+\/\d+/)).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("node-actions.png"),
    fullPage: true,
  });

  await page.getByRole("button", { name: "Revoke Agent credential" }).click();
  await expect(
    page.getByRole("heading", {
      name: "Revoke the Agent credential for edge-e2e?",
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Revoke credential" }).click();
  await expect(responsiveItem("Revoked")).toBeVisible();

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
      return body.items.length;
    })
    .toBe(0);
  await page.reload();
  await expect(page.getByText("No nodes are registered")).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("nodes.png"),
    fullPage: true,
  });

  const cleanupSettingsLink = page.getByRole("link", {
    name: "Network probes",
    exact: true,
  });
  if (!(await cleanupSettingsLink.isVisible())) {
    await page.getByRole("button", { name: "Toggle sidebar" }).click();
  }
  await cleanupSettingsLink.click();
  await page.getByRole("button", { name: "Delete proxy" }).click();
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: "Delete proxy", exact: true })
    .click();
  await expect(
    page.getByText("No centrally managed proxies are configured."),
  ).toBeVisible();
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
