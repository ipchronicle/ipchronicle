import { randomUUID } from "node:crypto";

import { expect, test, type Page } from "@playwright/test";

test.use({ timezoneId: "Asia/Shanghai" });

async function signIn(page: Page) {
  await page.goto("/nodes");
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

  const switchToEnglish = page.getByRole("button", { name: "切换到英文" });
  if (await switchToEnglish.isVisible()) await switchToEnglish.click();
  await page.goto("/nodes");
  await expect(page.getByRole("heading", { name: "Nodes" })).toBeVisible();
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

test("uses the browser timezone and exposes a searchable timezone selector", async ({
  page,
}) => {
  await signIn(page);
  const browserTimezone = await page.evaluate(
    () => Intl.DateTimeFormat().resolvedOptions().timeZone,
  );
  expect(browserTimezone).toBe("Asia/Shanghai");

  const rotationRequestPromise = page.waitForRequest(
    (request) =>
      request.url().endsWith("/api/v1/agent-enrollment/key") &&
      request.method() === "POST",
  );
  const rotationResponsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/agent-enrollment/key") &&
      response.request().method() === "POST",
  );
  const generateKey = page.getByRole("button", { name: "Generate key" });
  if (await generateKey.isVisible()) {
    await generateKey.click();
  } else {
    await page.getByRole("button", { name: "Rotate key", exact: true }).click();
    await page
      .getByRole("alertdialog")
      .getByRole("button", { name: "Rotate key", exact: true })
      .click();
  }
  const rotationRequest = await rotationRequestPromise;
  expect(rotationRequest.postDataJSON()).toEqual({
    defaultProbeTimezone: browserTimezone,
  });
  const rotationResponse = await rotationResponsePromise;
  expect(rotationResponse.status()).toBe(200);
  const enrollment = (await rotationResponse.json()) as {
    registrationKey: string;
  };

  const nodeName = `timezone-${randomUUID()}`;
  const registration = await page.request.post("/api/v1/agent/enroll", {
    data: {
      registrationKey: enrollment.registrationKey,
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
          "complete-probe-v1",
        ],
      },
    },
  });
  expect(registration.status()).toBe(201);
  const registered = (await registration.json()) as { nodeId: string };

  await page.goto(`/nodes/${registered.nodeId}/probe`);
  const timezone = page.getByRole("combobox", { name: "Time zone" });
  await expect(timezone).toHaveText(browserTimezone);
  await timezone.click();
  await page.getByPlaceholder("Search time zones...").fill("Europe/London");
  await page.getByRole("option", { name: "Europe/London" }).click();
  await expect(timezone).toHaveText("Europe/London");
  await expectNoPageOverflow(page);
});
