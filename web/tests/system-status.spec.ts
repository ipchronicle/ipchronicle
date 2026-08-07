import { expect, test } from "@playwright/test";

test("shows a healthy center in both supported languages", async ({
  page,
}, testInfo) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/system/status");

  await expect(
    page.getByRole("heading", { name: "System status" }),
  ).toBeVisible();
  await expect(page.getByText("Operational")).toBeVisible();
  await expect(page.getByText("ipchronicle-center")).toBeVisible();

  await page
    .getByRole("button", { name: "Switch to Simplified Chinese" })
    .click();
  await expect(page.getByRole("heading", { name: "系统状态" })).toBeVisible();
  await expect(page.getByText("运行正常")).toBeVisible();

  await page.screenshot({
    path: testInfo.outputPath("system-status.png"),
    fullPage: true,
  });
});

test("changes theme immediately", async ({ page }) => {
  await page.emulateMedia({ colorScheme: "light" });
  await page.goto("/");

  await page.getByRole("button", { name: "Use dark theme" }).click();
  await expect(page.locator("html")).toHaveClass(/dark/);
});
