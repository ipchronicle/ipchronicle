import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type NodeNetworkState = components["schemas"]["NodeNetworkState"];
export type NetworkEgress = components["schemas"]["NetworkEgress"];
export type NetworkEgressCandidate =
  components["schemas"]["NetworkEgressCandidate"];
export type NetworkEgressCreate = components["schemas"]["NetworkEgressCreate"];
export type NetworkEgressUpdate = components["schemas"]["NetworkEgressUpdate"];
export type EgressDeletion = components["schemas"]["EgressDeletion"];
export type NetworkObservationSettings =
  components["schemas"]["NetworkObservationSettings"];
export type NetworkObservationSettingsUpdate =
  components["schemas"]["NetworkObservationSettingsUpdate"];

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
  update: NetworkEgressUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PATCH(
    "/api/v1/nodes/{nodeId}/egresses/{egressId}",
    {
      params: { path: { nodeId, egressId } },
      body: update,
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function getNetworkObservationSettings(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/network-observation-settings", {
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateNetworkObservationSettings(
  update: NetworkObservationSettingsUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/network-observation-settings", {
    body: update,
    headers: { "X-CSRF-Token": csrfToken },
  });
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
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
