import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";

export async function getSystemStatus(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/system/status", { signal });

  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }

  return result.data;
}
