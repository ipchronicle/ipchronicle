import { describe, expect, it } from "vitest";

import { en } from "@/locales/en";
import { zhCN } from "@/locales/zh-CN";
import i18n from "@/i18n";

function keys(value: object, prefix = ""): string[] {
  return Object.entries(value).flatMap(([key, child]) => {
    const current = prefix === "" ? key : `${prefix}.${key}`;
    return typeof child === "object" && child !== null
      ? keys(child, current)
      : [current];
  });
}

describe("translation resources", () => {
  it("keeps Simplified Chinese and English keys in parity", () => {
    expect(keys(zhCN).sort()).toEqual(keys(en).sort());
  });

  it("falls back to English for an unsupported locale", async () => {
    await i18n.changeLanguage("fr");
    expect(i18n.t("status.title")).toBe("System status");
    await i18n.changeLanguage("en");
  });
});
