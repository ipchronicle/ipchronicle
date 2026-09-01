import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type Overview = components["schemas"]["Overview"];
export type OverviewNode = components["schemas"]["OverviewNode"];
export type OverviewPublicAddress =
  components["schemas"]["OverviewPublicAddress"];

export async function getOverview(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/overview", { signal });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
