import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type AgentUpdateState = components["schemas"]["AgentUpdateState"];
export type AgentUpdateTask = components["schemas"]["AgentUpdateTask"];
export type AgentUpdateBatchResult =
  components["schemas"]["AgentUpdateBatchResult"];
export type ReleaseChannel = components["schemas"]["ReleaseChannel"];

export async function getAgentUpdateState(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/agent-updates", { signal });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateReleaseChannel(
  channel: ReleaseChannel,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/agent-updates/channel", {
    body: { channel },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function createAgentUpdateTasks(
  nodeIds: string[],
  targetVersion: string,
  csrfToken: string,
) {
  const result = await apiClient.POST("/api/v1/agent-updates", {
    body: { nodeIds, targetVersion },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
