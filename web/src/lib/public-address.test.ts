import { describe, expect, it } from "vitest";

import { publicAddressAvailability } from "@/lib/public-address";

describe("publicAddressAvailability", () => {
  it("reports a current successful check as available", () => {
    expect(
      publicAddressAvailability({
        available: true,
        lastCheckedAt: "2026-08-31T09:03:14Z",
        lastSucceededAt: "2026-08-31T09:03:14Z",
      }),
    ).toBe("available");
  });

  it("exposes a failed check without discarding the confirmed baseline", () => {
    expect(
      publicAddressAvailability({
        available: true,
        lastCheckedAt: "2026-08-31T08:59:44Z",
        lastSucceededAt: "2026-08-31T03:10:54Z",
      }),
    ).toBe("check-failed");
  });

  it("keeps an address without an available path unavailable", () => {
    expect(publicAddressAvailability({ available: false })).toBe("unavailable");
  });
});
