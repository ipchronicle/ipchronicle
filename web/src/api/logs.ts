import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components, operations } from "@/api/schema";

export type LogEventSummary = components["schemas"]["LogEventSummary"];
export type LogEventDetail = components["schemas"]["LogEventDetail"];
export type LogEventPage = components["schemas"]["LogEventPage"];
export type LogLevel = components["schemas"]["LogLevel"];
export type LogRetentionState = components["schemas"]["LogRetentionState"];
export type LogRetentionUpdate = components["schemas"]["LogRetentionUpdate"];
export type LogFilters = NonNullable<
  operations["listLogs"]["parameters"]["query"]
>;

export async function listLogs(filters: LogFilters, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/logs", {
    params: { query: filters },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function getLog(logId: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/logs/{logId}", {
    params: { path: { logId } },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function getLogRetention(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/logs/retention", { signal });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateLogRetention(
  update: LogRetentionUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/logs/retention", {
    body: update,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function cleanupLogs(csrfToken: string) {
  const result = await apiClient.POST("/api/v1/logs/cleanup", {
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
