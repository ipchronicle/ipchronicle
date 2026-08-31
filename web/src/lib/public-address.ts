import type { PublicAddress } from "@/api/network";

export type PublicAddressAvailability =
  "available" | "check-failed" | "unavailable";

export function publicAddressAvailability(
  address: Pick<
    PublicAddress,
    "available" | "lastCheckedAt" | "lastSucceededAt"
  >,
): PublicAddressAvailability {
  if (!address.available) return "unavailable";
  if (
    address.lastCheckedAt !== undefined &&
    (address.lastSucceededAt === undefined ||
      Date.parse(address.lastCheckedAt) > Date.parse(address.lastSucceededAt))
  ) {
    return "check-failed";
  }
  return "available";
}
