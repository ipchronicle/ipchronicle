import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type Node = components["schemas"]["Node"];
export type AgentEnrollmentSettings =
  components["schemas"]["AgentEnrollmentSettings"];
export type NodeDeletion = components["schemas"]["NodeDeletion"];

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

export async function updateNode(
  nodeId: string,
  enabled: boolean,
  csrfToken: string,
) {
  const result = await apiClient.PATCH("/api/v1/nodes/{nodeId}", {
    params: { path: { nodeId } },
    body: { enabled },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function revokeNode(nodeId: string, csrfToken: string) {
  const result = await apiClient.POST("/api/v1/nodes/{nodeId}/revoke", {
    params: { path: { nodeId } },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function startNodeSyncSession(nodeId: string, csrfToken: string) {
  const result = await apiClient.POST("/api/v1/nodes/{nodeId}/sync-session", {
    params: { path: { nodeId } },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function stopNodeSyncSession(nodeId: string, csrfToken: string) {
  const result = await apiClient.DELETE("/api/v1/nodes/{nodeId}/sync-session", {
    params: { path: { nodeId } },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function deleteNode(nodeId: string, csrfToken: string) {
  const result = await apiClient.DELETE("/api/v1/nodes/{nodeId}", {
    params: { path: { nodeId } },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
