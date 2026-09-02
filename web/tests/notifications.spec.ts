import { expect, test, type Page } from "@playwright/test";

const receiverURL = process.env.IPCHRONICLE_E2E_RECEIVER_URL;
const receiverInternalURL = process.env.IPCHRONICLE_E2E_RECEIVER_INTERNAL_URL;

function administratorWriteHeaders(page: Page, csrfToken: string) {
  return {
    Origin: new URL(page.url()).origin,
    "X-CSRF-Token": csrfToken,
  };
}

async function signIn(page: Page) {
  await page.goto("/notifications");
  await expect(page).toHaveURL(/\/login$/);
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("admin");
  const loginResponsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/auth/login") &&
      response.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Sign in" }).click();
  const loginResponse = await loginResponsePromise;
  expect(loginResponse.status()).toBe(200);
  const session = (await loginResponse.json()) as { csrfToken: string };

  const switchToEnglish = page.getByRole("button", { name: "切换到英文" });
  if (await switchToEnglish.isVisible()) await switchToEnglish.click();
  await page.goto("/notifications");
  await expect(
    page.getByRole("heading", { name: "Notifications" }),
  ).toBeVisible();
  return session.csrfToken;
}

async function createNodeWithPublicAddress(
  page: Page,
  csrfToken: string,
  nodeName: string,
) {
  const keyResponse = await page.request.post("/api/v1/agent-enrollment/key", {
    headers: administratorWriteHeaders(page, csrfToken),
    data: { defaultProbeTimezone: "UTC" },
  });
  expect(keyResponse.status()).toBe(200);
  const enrollment = (await keyResponse.json()) as {
    registrationKey: string;
  };
  const registrationKey = enrollment.registrationKey;
  expect(registrationKey).toBeTruthy();

  const metadata = {
    hostname: nodeName,
    agentVersion: "0.1.0-e2e",
    operatingSystem: "linux",
    architecture: "amd64",
    physicalMemoryBytes: 536870912,
    capabilities: [
      "control-v1",
      "configuration-v9",
      "network-inventory-v1",
      "complete-probe-v1",
    ],
  };
  const registration = await page.request.post("/api/v1/agent/enroll", {
    data: { registrationKey, metadata },
  });
  expect(registration.status()).toBe(201);
  const registered = (await registration.json()) as {
    credential: string;
    nodeId: string;
  };
  const control = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: 0,
      metadata,
      networkInventory: {
        capturedAt: "2026-08-09T12:00:00Z",
        interfaces: [{ name: "eth0", index: 2, up: true, loopback: false }],
        addresses: [
          {
            interfaceName: "eth0",
            address: "10.20.30.40",
            prefixLength: 24,
            family: "ipv4",
            scope: "private",
            temporary: false,
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
            gateway: "10.20.30.1",
            metric: 100,
            default: true,
          },
        ],
      },
    },
  });
  expect(control.status()).toBe(200);
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
  const path = configuration.discoveryPaths.find(
    (item) => item.family === "ipv4",
  );
  expect(path).toBeTruthy();
  const observedAt = "2026-08-09T12:01:00Z";
  const addressControl = await page.request.post("/api/v1/agent/control", {
    headers: { Authorization: `Bearer ${registered.credential}` },
    data: {
      appliedConfigurationRevision: configuration.revision,
      metadata,
      addressStates: [
        {
          egressId: path?.id,
          historyGeneration: configuration.historyGeneration,
          family: "ipv4",
          status: "confirmed",
          sequence: 1,
          publicAddress: "203.0.113.10",
          localInterface: "eth0",
          localAddress: "10.20.30.40",
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
          id: crypto.randomUUID(),
          egressId: path?.id,
          historyGeneration: configuration.historyGeneration,
          sequence: 1,
          kind: "first-observation",
          family: "ipv4",
          publicAddress: "203.0.113.10",
          localInterface: "eth0",
          localAddress: "10.20.30.40",
          proxyPath: false,
          likelyNat: true,
          temporary: false,
          observedAt,
        },
      ],
      addressGaps: [],
    },
  });
  expect(addressControl.status()).toBe(200);
  const networkResponse = await page.request.get(
    `/api/v1/nodes/${registered.nodeId}/network`,
  );
  expect(networkResponse.status()).toBe(200);
  const network = (await networkResponse.json()) as {
    publicAddresses: Array<{ id: string; address: string }>;
  };
  expect(network.publicAddresses).toHaveLength(1);
  return {
    nodeId: registered.nodeId,
    publicAddress: network.publicAddresses[0],
  };
}

async function expectDeliverySucceeded(page: Page, senderName: string) {
  const deliveryHistory = page.getByRole("tabpanel", {
    name: "Delivery history",
  });
  await expect
    .poll(
      async () => {
        await deliveryHistory.getByRole("button", { name: "Refresh" }).click();
        const row = deliveryHistory
          .getByRole("row")
          .filter({ hasText: senderName });
        return (await row.count()) === 1 ? await row.textContent() : "";
      },
      { timeout: 10_000 },
    )
    .toContain("Succeeded");
}

async function expectNoPageOverflow(page: Page) {
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

test("configures and delivers notifications through local receivers", async ({
  page,
}, testInfo) => {
  expect(receiverURL).toBeTruthy();
  expect(receiverInternalURL).toBeTruthy();
  await page.request.post(`${receiverURL}/reset`);
  const csrfToken = await signIn(page);
  const suffix = testInfo.project.name;
  const nodeName = `notify-node-${suffix}`;
  const webhookName = `Local webhook ${suffix}`;
  const updatedWebhookName = `${webhookName} updated`;
  const javascriptName = `Local JavaScript ${suffix}`;
  const ruleName = `Address changes ${suffix}`;
  const updatedRuleName = `${ruleName} updated`;
  const node = await createNodeWithPublicAddress(page, csrfToken, nodeName);

  try {
    await page.reload();
    await page.getByRole("button", { name: "Add sender" }).click();
    await page.getByLabel("Name", { exact: true }).fill(webhookName);
    await page.getByLabel("Sender type").click();
    await page.getByRole("option", { name: "Webhook" }).click();
    await page
      .getByLabel("Webhook URL")
      .fill(`${receiverInternalURL}/delivery`);
    await page.getByLabel("HTTP headers").fill("X-E2E-Token: local-e2e-secret");
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText(webhookName, { exact: true })).toBeVisible();

    const webhookCard = page
      .getByText(webhookName, { exact: true })
      .locator("..");
    await webhookCard.getByRole("button", { name: "Edit" }).click();
    await expect(
      page.getByText("Configured header names: X-E2E-Token"),
    ).toBeVisible();
    await expect(page.locator("body")).not.toContainText("local-e2e-secret");
    await page.getByLabel("Name", { exact: true }).fill(updatedWebhookName);
    await page.getByRole("button", { name: "Save" }).click();
    const updatedWebhookCard = page
      .getByText(updatedWebhookName, { exact: true })
      .locator("..");
    await updatedWebhookCard.getByRole("button", { name: "Send test" }).click();
    await expectDeliverySucceeded(page, updatedWebhookName);

    await page.getByRole("tab", { name: "Senders" }).click();
    await page.getByRole("button", { name: "Add sender" }).click();
    await page.getByLabel("Name", { exact: true }).fill(javascriptName);
    await page.getByLabel("Sender type").click();
    await page.getByRole("option", { name: "JavaScript" }).click();
    await page.getByLabel("JavaScript source").fill(`
      var response = ipchronicle.http.request({
        method: "POST",
        url: ${JSON.stringify(`${receiverInternalURL}/delivery`)},
        headers: {"Content-Type": "application/json", "X-E2E-Kind": "javascript"},
        body: JSON.stringify({id: ipchronicle.event.id, title: ipchronicle.title})
      });
      if (response.status !== 204) throw new Error("delivery failed");
    `);
    await page.getByRole("button", { name: "Save" }).click();
    const javascriptCard = page
      .getByText(javascriptName, { exact: true })
      .locator("..");
    await javascriptCard.getByRole("button", { name: "Send test" }).click();
    await expectDeliverySucceeded(page, javascriptName);

    await expect
      .poll(async () => {
        const response = await page.request.get(`${receiverURL}/messages`);
        const payload = (await response.json()) as {
          items: Array<{ body: string; headers: Record<string, string> }>;
        };
        return payload.items;
      })
      .toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            headers: expect.objectContaining({
              "x-e2e-token": "local-e2e-secret",
            }),
          }),
          expect.objectContaining({
            headers: expect.objectContaining({ "x-e2e-kind": "javascript" }),
          }),
        ]),
      );

    await page.getByRole("tab", { name: "Rules" }).click();
    await page.getByRole("button", { name: "Add rule" }).click();
    await page.getByLabel("Name", { exact: true }).fill(ruleName);
    await page.getByLabel("Event").click();
    await expect(
      page.getByRole("option", { name: "All events" }),
    ).toBeVisible();
    await page.getByRole("option", { name: "Probe field changed" }).click();
    const probeField = page.getByRole("combobox", { name: "Probe field" });
    await expect(probeField).toContainText("All probe fields");
    await probeField.click();
    await page.getByPlaceholder("Search probe fields...").fill("VPN IPQS");
    await page.getByText("VPN indicator (IPQS)", { exact: true }).click();
    await page.getByLabel("Node").click();
    await page.getByRole("option", { name: nodeName }).click();
    await expect(page.getByLabel("Public IP")).toBeEnabled();
    await page.getByLabel("Public IP").click();
    await page
      .getByRole("option", { name: node.publicAddress.address })
      .click();
    await page.getByRole("button", { name: "Save" }).click();
    const ruleCard = page
      .locator('[data-slot="card"]')
      .filter({ has: page.getByText(ruleName, { exact: true }) });
    await expect(
      ruleCard.getByText(node.publicAddress.address, { exact: true }),
    ).toBeVisible();
    await expect(
      ruleCard.getByText("VPN indicator (IPQS)", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText(node.publicAddress.id, { exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByText("Factor.VPN.IPQS", { exact: true }),
    ).toHaveCount(0);
    await ruleCard.getByRole("button", { name: "Edit" }).click();
    await page.getByLabel("Name", { exact: true }).fill(updatedRuleName);
    await page.getByRole("button", { name: "Save" }).click();
    await expect(
      page.getByText(updatedRuleName, { exact: true }),
    ).toBeVisible();

    await page.getByRole("tab", { name: "Delivery history" }).click();
    await page.getByLabel("Sender", { exact: true }).click();
    await page.getByRole("option", { name: javascriptName }).click();
    await expect(
      page.getByRole("row").filter({ hasText: javascriptName }),
    ).toBeVisible();
    await expect(
      page.getByRole("row").filter({ hasText: updatedWebhookName }),
    ).toHaveCount(0);
    await expectNoPageOverflow(page);
    await page.screenshot({
      path: testInfo.outputPath("notifications-en.png"),
      fullPage: true,
    });

    await page
      .getByRole("button", { name: "Switch to Simplified Chinese" })
      .click();
    await expect(page.getByRole("heading", { name: "通知" })).toBeVisible();
    await expect(page.getByText("Notification settings saved.")).toHaveCount(0);
    await expectNoPageOverflow(page);
    await page.screenshot({
      path: testInfo.outputPath("notifications-zh-CN.png"),
      fullPage: true,
    });
    await page.getByRole("button", { name: "切换到英文" }).click();

    await page.getByRole("tab", { name: "Rules" }).click();
    const updatedRuleCard = page
      .getByText(updatedRuleName, { exact: true })
      .locator("..");
    await updatedRuleCard.getByRole("button", { name: "Delete" }).click();
    await page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Delete", exact: true })
      .click();
    await expect(page.getByText(updatedRuleName, { exact: true })).toHaveCount(
      0,
    );

    await page.getByRole("tab", { name: "Senders" }).click();
    for (const name of [updatedWebhookName, javascriptName]) {
      const card = page.getByText(name, { exact: true }).locator("..");
      await card.getByRole("button", { name: "Delete" }).click();
      await page
        .getByRole("alertdialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click();
      await expect(page.getByText(name, { exact: true })).toHaveCount(0);
    }
  } finally {
    const rules = await page.request.get("/api/v1/notification-rules");
    if (rules.ok()) {
      const payload = (await rules.json()) as {
        items: Array<{ id: string; name: string }>;
      };
      for (const rule of payload.items.filter((item) =>
        item.name.startsWith(ruleName),
      )) {
        await page.request.delete(`/api/v1/notification-rules/${rule.id}`, {
          headers: administratorWriteHeaders(page, csrfToken),
        });
      }
    }
    const senders = await page.request.get("/api/v1/notification-senders");
    if (senders.ok()) {
      const payload = (await senders.json()) as {
        items: Array<{ id: string; name: string }>;
      };
      for (const sender of payload.items.filter(
        (item) =>
          item.name.startsWith(webhookName) ||
          item.name.startsWith(javascriptName),
      )) {
        await page.request.delete(`/api/v1/notification-senders/${sender.id}`, {
          headers: administratorWriteHeaders(page, csrfToken),
        });
      }
    }
    await page.request.delete(`/api/v1/nodes/${node.nodeId}`, {
      headers: administratorWriteHeaders(page, csrfToken),
    });
  }
});
