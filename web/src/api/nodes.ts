import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type Node = components["schemas"]["Node"];
export type AgentEnrollmentSettings =
  components["schemas"]["AgentEnrollmentSettings"];

export async function listNodes(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/nodes", { signal });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data.items;
}

export async function getAgentEnrollment(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/agent-enrollment", { signal });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateAgentEnrollment(
  enabled: boolean,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/agent-enrollment", {
    body: { enabled },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function rotateAgentEnrollmentKey(csrfToken: string) {
  const result = await apiClient.POST("/api/v1/agent-enrollment/key", {
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
