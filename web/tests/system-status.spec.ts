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
