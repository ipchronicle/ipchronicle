import { describe, expect, it } from "vitest";

import { browserTimeZone, supportedTimeZones } from "@/lib/time-zone";

describe("time zones", () => {
  it("uses an explicit browser IANA timezone", () => {
    expect(browserTimeZone()).not.toBe("agent-local");
    expect(browserTimeZone().length).toBeGreaterThan(0);
  });

  it("keeps UTC, the browser timezone, and the selected timezone searchable", () => {
    const values = supportedTimeZones("Asia/Shanghai");
    expect(values).toContain("UTC");
    expect(values).toContain(browserTimeZone());
    expect(values).toContain("Asia/Shanghai");
  });
});
