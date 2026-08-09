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
        capabilities: [
          "control-v1",
          "configuration-v1",
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
  const responsiveItem = (text: string) => {
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
        capabilities: [
          "control-v1",
          "configuration-v1",
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

  await page.getByRole("link", { name: "Network egresses" }).click();
  await expect(page.getByRole("heading", { name: "edge-e2e" })).toBeVisible();
  await expect(page.getByText("Default IPv4").first()).toBeVisible();
  await expect(page.getByText("Temporary IPv6").first()).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Enable path" }).last(),
  ).toBeDisabled();
  await page.screenshot({
    path: testInfo.outputPath("network-egresses.png"),
    fullPage: true,
  });
  await page.getByRole("link", { name: "Back to nodes" }).click();

  await page.getByRole("button", { name: "Pause node" }).click();
  await expect(responsiveItem("Disabled")).toBeVisible();
  await expect(responsiveItem("Pending · 0/3")).toBeVisible();
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
