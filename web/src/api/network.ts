import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type NodeNetworkState = components["schemas"]["NodeNetworkState"];
export type PublicAddress = components["schemas"]["PublicAddress"];
export type PublicAddressUpdate = components["schemas"]["PublicAddressUpdate"];
export type ProxyDiscoveryPath = components["schemas"]["ProxyDiscoveryPath"];
export type ProxyDiscoveryPathCreate =
  components["schemas"]["ProxyDiscoveryPathCreate"];
export type ProxyDiscoveryPathDeletion =
  components["schemas"]["ProxyDiscoveryPathDeletion"];
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

export async function updatePublicAddress(
  nodeId: string,
  publicAddressId: string,
  update: PublicAddressUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PATCH(
    "/api/v1/nodes/{nodeId}/public-addresses/{publicAddressId}",
    {
      params: { path: { nodeId, publicAddressId } },
      body: update,
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function createNodeProxyDiscoveryPath(
  nodeId: string,
  input: ProxyDiscoveryPathCreate,
  csrfToken: string,
) {
  const result = await apiClient.POST(
    "/api/v1/nodes/{nodeId}/proxy-discovery-paths",
    {
      params: { path: { nodeId } },
      body: input,
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

export async function deleteNodeProxyDiscoveryPath(
  nodeId: string,
  pathId: string,
  csrfToken: string,
) {
  const result = await apiClient.DELETE(
    "/api/v1/nodes/{nodeId}/proxy-discovery-paths/{pathId}",
    {
      params: { path: { nodeId, pathId } },
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
