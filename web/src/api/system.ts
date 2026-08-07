import { apiClient } from "@/api/client";

export async function getSystemStatus(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/system/status", { signal });

  if (!result.response.ok || result.data === undefined) {
    throw new Error(
      `center status request failed with HTTP ${result.response.status}`,
    );
  }

  return result.data;
}
