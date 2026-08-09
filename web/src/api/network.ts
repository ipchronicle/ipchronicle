import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type NodeNetworkState = components["schemas"]["NodeNetworkState"];
export type NetworkEgress = components["schemas"]["NetworkEgress"];
export type NetworkEgressCandidate =
  components["schemas"]["NetworkEgressCandidate"];
export type NetworkEgressCreate = components["schemas"]["NetworkEgressCreate"];

export async function getNodeNetwork(nodeId: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/nodes/{nodeId}/network", {
    params: { path: { nodeId } },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function createNodeEgress(
  nodeId: string,
  selector: NetworkEgressCreate,
  csrfToken: string,
) {
  const result = await apiClient.POST("/api/v1/nodes/{nodeId}/egresses", {
    params: { path: { nodeId } },
    body: selector,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateNodeEgress(
  nodeId: string,
  egressId: string,
  enabled: boolean,
  csrfToken: string,
) {
  const result = await apiClient.PATCH(
    "/api/v1/nodes/{nodeId}/egresses/{egressId}",
    {
      params: { path: { nodeId, egressId } },
      body: { enabled },
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function deleteNodeEgress(
  nodeId: string,
  egressId: string,
  csrfToken: string,
) {
  const result = await apiClient.DELETE(
    "/api/v1/nodes/{nodeId}/egresses/{egressId}",
    {
      params: { path: { nodeId, egressId } },
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok) {
    throwAPIError(result.response, result.error);
  }
}
