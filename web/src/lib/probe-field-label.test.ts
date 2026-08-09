import { describe, expect, it } from "vitest";

import i18n from "@/i18n";
import {
  presentProbeField,
  presentProbeFieldGroup,
} from "@/lib/probe-field-label";

const directPaths = [
  "Head.IP",
  "Head.Command",
  "Head.GitHub",
  "Head.Time",
  "Head.Version",
  "Info.ASN",
  "Info.Organization",
  "Info.Latitude",
  "Info.Longitude",
  "Info.DMS",
  "Info.Map",
  "Info.TimeZone",
  "Info.City.Name",
  "Info.City.PostalCode",
  "Info.City.SubCode",
  "Info.City.Subdivisions",
  "Info.Region.Code",
  "Info.Region.Name",
  "Info.Continent.Code",
  "Info.Continent.Name",
  "Info.RegisteredRegion.Code",
  "Info.RegisteredRegion.Name",
  "Info.Type",
  "Mail.DNSBlacklist.Total",
  "Mail.DNSBlacklist.Clean",
  "Mail.DNSBlacklist.Marked",
  "Mail.DNSBlacklist.Blacklisted",
];

const patternedPaths = [
  "Type.Usage.IPinfo",
  "Type.Company.ipregistry",
  "Score.IPQS",
  "Factor.CountryCode.DBIP",
  "Factor.Proxy.IPQS",
  "Factor.Tor.IPQS",
  "Factor.VPN.IPQS",
  "Factor.Server.IPQS",
  "Factor.Abuser.IPQS",
  "Factor.Robot.IPQS",
  "Media.Reddit.Status",
  "Media.Netflix.Region",
  "Media.ChatGPT.Type",
  "Mail.Port25",
  "Mail.Gmail",
];

describe("probe field presentation", () => {
  it.each(["en", "zh-CN"])(
    "localizes every known field shape in %s",
    async (locale) => {
      await i18n.changeLanguage(locale);
      for (const path of [...directPaths, ...patternedPaths]) {
        const group = path.split(".")[0];
        const presentation = presentProbeField({ group, path }, i18n.t);
        expect(presentation.name, path).not.toBe(path);
        expect(presentation.description, path).not.toContain(
          "without a localized description",
        );
        expect(presentation.description, path).not.toContain("尚未提供");
      }
      for (const group of [
        "Head",
        "Info",
        "Type",
        "Score",
        "Factor",
        "Media",
        "Mail",
      ]) {
        const presentation = presentProbeFieldGroup(group, i18n.t);
        expect(presentation.name).not.toContain("fieldCatalog.groups");
        expect(presentation.description).not.toContain("fieldCatalog.groups");
      }
      await i18n.changeLanguage("en");
    },
  );

  it("keeps provider names while translating field meaning", async () => {
    await i18n.changeLanguage("zh-CN");
    expect(
      presentProbeField({ group: "Factor", path: "Factor.VPN.IPQS" }, i18n.t)
        .name,
    ).toBe("VPN 指标（IPQS）");
    expect(
      presentProbeField({ group: "Media", path: "Media.Reddit.Status" }, i18n.t)
        .name,
    ).toBe("Reddit 可用性");
    await i18n.changeLanguage("en");
  });

  it("keeps an unmapped upstream path as technical evidence", () => {
    const field = presentProbeField(
      { group: "Future", path: "Future.NewField" },
      i18n.t,
    );
    const group = presentProbeFieldGroup("Future", i18n.t);
    expect(field.name).toBe("Future.NewField");
    expect(group.name).toBe("Future");
    expect(field.description).not.toContain("fieldCatalog");
    expect(group.description).not.toContain("fieldCatalog");
  });
});
