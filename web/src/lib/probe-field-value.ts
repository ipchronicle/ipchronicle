import type { TFunction } from "i18next";

const classificationKeys: Record<string, string> = {
  business: "business",
  commercial: "business",
  com: "business",
  商业: "business",
  isp: "residential",
  "fixed line isp": "residential",
  "line isp": "residential",
  residential: "residential",
  "residential isp": "residential",
  家宽: "residential",
  hosting: "hosting",
  "data center": "hosting",
  "data center/web hosting/transit": "hosting",
  dch: "hosting",
  "dch/com": "hosting",
  机房: "hosting",
  education: "education",
  university: "education",
  "university/college/school": "education",
  edu: "education",
  教育: "education",
  government: "government",
  gov: "government",
  政府: "government",
  banking: "banking",
  bank: "banking",
  银行: "banking",
  organization: "organization",
  org: "organization",
  组织: "organization",
  military: "military",
  mil: "military",
  军队: "military",
  library: "library",
  lib: "library",
  图书馆: "library",
  "content delivery network": "cdn",
  cdn: "cdn",
  "mobile isp": "mobile",
  mobile: "mobile",
  "mobile network": "mobile",
  mob: "mobile",
  手机: "mobile",
  移动网络: "mobile",
  "search engine spider": "crawler",
  "search crawler": "crawler",
  "web spider": "crawler",
  spider: "crawler",
  ses: "crawler",
  蜘蛛: "crawler",
  搜索爬虫: "crawler",
  reserved: "reserved",
  rsv: "reserved",
  保留: "reserved",
  other: "other",
  其他: "other",
};

const mediaStatusKeys: Record<string, string> = {
  yes: "unlocked",
  true: "unlocked",
  available: "unlocked",
  unlock: "unlocked",
  unlocked: "unlocked",
  解锁: "unlocked",
  no: "blocked",
  false: "blocked",
  unavailable: "blocked",
  block: "blocked",
  blocked: "blocked",
  屏蔽: "blocked",
  failed: "checkFailed",
  failure: "checkFailed",
  "check failed": "checkFailed",
  失败: "checkFailed",
  检测失败: "checkFailed",
  pending: "pending",
  "not supported": "pending",
  "not yet supported": "pending",
  待支持: "pending",
  "nf.only": "originalsOnly",
  "nf only": "originalsOnly",
  "originals only": "originalsOnly",
  "only originals": "originalsOnly",
  仅自制: "originalsOnly",
  仅自制内容: "originalsOnly",
  china: "mainlandChina",
  "mainland china": "mainlandChina",
  中国: "mainlandChina",
  中国大陆: "mainlandChina",
  "noprem.": "premiumUnavailable",
  "no premium": "premiumUnavailable",
  "premium unavailable": "premiumUnavailable",
  禁会员: "premiumUnavailable",
  "premium 不可用": "premiumUnavailable",
  webonly: "webOnly",
  "web only": "webOnly",
  仅网页: "webOnly",
  仅网页可用: "webOnly",
  apponly: "appOnly",
  "app only": "appOnly",
  仅app: "appOnly",
  "仅 app 可用": "appOnly",
  idc: "dataCenter",
  "data center": "dataCenter",
  机房: "dataCenter",
};

const continentKeys: Record<string, string> = {
  AF: "africa",
  AN: "antarctica",
  AS: "asia",
  EU: "europe",
  NA: "northAmerica",
  OC: "oceania",
  SA: "southAmerica",
};

export function presentProbeFieldValue(
  path: string,
  value: string,
  t: TFunction,
  language?: string,
) {
  const normalized = normalize(value);
  if (path === "Info.Type") {
    if (
      ["geo-consistent", "native ip", "原生ip", "原生 ip"].includes(normalized)
    ) {
      return t("snapshot.report.basic.typeValues.native");
    }
    if (
      ["geo-discrepant", "broadcast ip", "广播ip", "广播 ip"].includes(
        normalized,
      )
    ) {
      return t("snapshot.report.basic.typeValues.broadcast");
    }
  }
  if (path.startsWith("Type.Usage.") || path.startsWith("Type.Company.")) {
    const key = classificationKeys[normalized];
    if (key) return t(`snapshot.report.type.values.${key}`);
  }
  if (path === "Info.Continent.Code") {
    const code = value.trim().toUpperCase();
    const key = continentKeys[code];
    if (key) {
      return formatCode(t(`snapshot.report.continents.${key}`), code, language);
    }
  }
  if (isCountryCodePath(path)) {
    return presentCountryCode(value, language);
  }
  if (path.startsWith("Media.") && path.endsWith(".Status")) {
    const key = mediaStatusKeys[normalized];
    if (key) return t(`snapshot.report.media.values.${key}`);
  }
  if (path.startsWith("Media.") && path.endsWith(".Type")) {
    if (["native", "direct", "原生"].includes(normalized)) {
      return t("snapshot.report.media.values.native");
    }
    if (["viadns", "via dns", "dns", "经 dns"].includes(normalized)) {
      return t("snapshot.report.media.values.viaDns");
    }
    if (["original", "originals", "原创", "自制内容"].includes(normalized)) {
      return t("snapshot.report.media.values.originals");
    }
    if (["web", "网页"].includes(normalized)) {
      return t("snapshot.report.media.values.web");
    }
  }
  return value;
}

function isCountryCodePath(path: string) {
  return (
    path === "Info.Region.Code" ||
    path === "Info.RegisteredRegion.Code" ||
    path.startsWith("Factor.CountryCode.") ||
    (path.startsWith("Media.") && path.endsWith(".Region"))
  );
}

function presentCountryCode(value: string, language?: string) {
  const code = value.trim().toUpperCase();
  if (!/^[A-Z]{2}$/.test(code)) return value;
  try {
    const name = new Intl.DisplayNames([language ?? "en"], {
      type: "region",
    }).of(code);
    if (name && name.toUpperCase() !== code) {
      return formatCode(name, code, language);
    }
  } catch {
    // Invalid browser locales keep the upstream code visible.
  }
  return value;
}

function formatCode(name: string, code: string, language?: string) {
  return language?.toLowerCase().startsWith("zh")
    ? `${name}（${code}）`
    : `${name} (${code})`;
}

function normalize(value: string) {
  return value.trim().toLowerCase();
}
