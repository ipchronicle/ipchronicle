import { describe, expect, it } from "vitest";

import i18n from "@/i18n";
import { presentProbeFieldValue } from "@/lib/probe-field-value";

describe("probe field value presentation", () => {
  it("localizes known Chinese and English probe values", async () => {
    await i18n.changeLanguage("zh-CN");
    expect(value("Type.Usage.ipapi", "Hosting", "zh-CN")).toBe("机房");
    expect(value("Media.Netflix.Status", "NF.only", "zh-CN")).toBe(
      "仅自制内容",
    );
    expect(value("Media.Netflix.Region", "US", "zh-CN")).toContain("US");
    expect(value("Type.Company.IPinfo", "Mobile network", "zh-CN")).toBe(
      "移动网络",
    );
    expect(value("Media.Netflix.Status", "检测失败", "zh-CN")).toBe("检测失败");

    await i18n.changeLanguage("en");
    expect(value("Type.Usage.ipapi", "家宽", "en")).toBe("Residential ISP");
    expect(value("Media.Netflix.Status", "仅自制", "en")).toBe(
      "Originals only",
    );
    expect(value("Media.Netflix.Type", "原生", "en")).toBe("Native");
    expect(value("Media.Netflix.Type", "经 DNS", "en")).toBe("Via DNS");
    expect(value("Media.Netflix.Status", "仅自制内容", "en")).toBe(
      "Originals only",
    );
    expect(value("Factor.CountryCode.IPQS", "US", "en")).toBe(
      "United States (US)",
    );
  });

  it("keeps free text and unknown upstream values unchanged", () => {
    expect(value("Info.Organization", "Example Network", "en")).toBe(
      "Example Network",
    );
    expect(value("Media.Future.Status", "FutureStatus", "en")).toBe(
      "FutureStatus",
    );
  });
});

function value(path: string, raw: string, language: string) {
  return presentProbeFieldValue(path, raw, i18n.t, language);
}
