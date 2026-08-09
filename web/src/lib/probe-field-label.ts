import type { TFunction } from "i18next";

export type ProbeFieldIdentity = {
  group: string;
  path: string;
};

export type ProbeFieldPresentation = {
  name: string;
  description: string;
};

const directFields: Record<string, string> = {
  "Head.IP": "head.ip",
  "Head.Command": "head.command",
  "Head.GitHub": "head.github",
  "Head.Time": "head.time",
  "Head.Version": "head.version",
  "Info.ASN": "info.asn",
  "Info.Organization": "info.organization",
  "Info.Latitude": "info.latitude",
  "Info.Longitude": "info.longitude",
  "Info.DMS": "info.dms",
  "Info.Map": "info.map",
  "Info.TimeZone": "info.timeZone",
  "Info.City.Name": "info.cityName",
  "Info.City.PostalCode": "info.cityPostalCode",
  "Info.City.SubCode": "info.citySubCode",
  "Info.City.Subdivisions": "info.citySubdivisions",
  "Info.Region.Code": "info.regionCode",
  "Info.Region.Name": "info.regionName",
  "Info.Continent.Code": "info.continentCode",
  "Info.Continent.Name": "info.continentName",
  "Info.RegisteredRegion.Code": "info.registeredRegionCode",
  "Info.RegisteredRegion.Name": "info.registeredRegionName",
  "Info.Type": "info.type",
  "Mail.DNSBlacklist.Total": "mail.dnsTotal",
  "Mail.DNSBlacklist.Clean": "mail.dnsClean",
  "Mail.DNSBlacklist.Marked": "mail.dnsMarked",
  "Mail.DNSBlacklist.Blacklisted": "mail.dnsBlacklisted",
};

const factorKeys: Record<string, string> = {
  CountryCode: "countryCode",
  Proxy: "proxy",
  Tor: "tor",
  VPN: "vpn",
  Server: "server",
  Abuser: "abuser",
  Robot: "robot",
};

const mediaKeys: Record<string, string> = {
  Status: "status",
  Region: "region",
  Type: "type",
};

const groupKeys: Record<string, string> = {
  Head: "head",
  Info: "info",
  Type: "type",
  Score: "score",
  Factor: "factor",
  Media: "media",
  Mail: "mail",
};

export function presentProbeField(
  field: ProbeFieldIdentity,
  t: TFunction,
): ProbeFieldPresentation {
  const directKey = directFields[field.path];
  if (directKey) return translated(t, directKey);

  const segments = field.path.split(".");
  if (segments.length === 3 && segments[0] === "Type") {
    if (segments[1] === "Usage") {
      return translated(t, "classification.usage", { provider: segments[2] });
    }
    if (segments[1] === "Company") {
      return translated(t, "classification.company", {
        provider: segments[2],
      });
    }
  }
  if (segments.length === 2 && segments[0] === "Score") {
    return translated(t, "score.risk", { provider: segments[1] });
  }
  if (segments.length === 3 && segments[0] === "Factor") {
    const key = factorKeys[segments[1]];
    if (key) return translated(t, `factor.${key}`, { provider: segments[2] });
  }
  if (segments.length === 3 && segments[0] === "Media") {
    const key = mediaKeys[segments[2]];
    if (key) return translated(t, `media.${key}`, { service: segments[1] });
  }
  if (
    segments.length === 2 &&
    segments[0] === "Mail" &&
    segments[1] !== "DNSBlacklist"
  ) {
    return translated(t, "mail.connectivity", { service: segments[1] });
  }
  return {
    name: field.path,
    description: t("snapshot.fieldCatalog.unmappedDescription"),
  };
}

export function presentProbeFieldGroup(group: string, t: TFunction) {
  const key = groupKeys[group];
  if (!key) {
    return {
      name: group,
      description: t("snapshot.fieldCatalog.unmappedDescription"),
    };
  }
  return {
    name: t(`snapshot.fieldCatalog.groups.${key}.name`),
    description: t(`snapshot.fieldCatalog.groups.${key}.description`),
  };
}

function translated(
  t: TFunction,
  key: string,
  values?: Record<string, string>,
): ProbeFieldPresentation {
  const prefix = `snapshot.fieldCatalog.fields.${key}`;
  return {
    name: t(`${prefix}.name`, values),
    description: t(`${prefix}.description`, values),
  };
}
