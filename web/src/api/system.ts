import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type SystemSettings = components["schemas"]["SystemSettings"];

export async function getSystemStatus(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/system/status", { signal });

  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }

  return result.data;
}

export async function getSystemSettings(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/system/settings", { signal });

  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }

  return result.data;
}

export async function updateSystemSettings(
  externalOrigin: string,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/system/settings", {
    body: { externalOrigin },
    headers: { "X-CSRF-Token": csrfToken },
  });

  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }

  return result.data;
}
