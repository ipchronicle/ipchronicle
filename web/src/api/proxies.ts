import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type NetworkProxy = components["schemas"]["NetworkProxy"];
export type NetworkProxyCreate = components["schemas"]["NetworkProxyCreate"];
export type NetworkProxyUpdate = components["schemas"]["NetworkProxyUpdate"];

export async function listNetworkProxies(nodeId: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/nodes/{nodeId}/network-proxies", {
    params: { path: { nodeId } },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data.items;
}

export async function createNetworkProxy(
  nodeId: string,
  input: NetworkProxyCreate,
  csrfToken: string,
) {
  const result = await apiClient.POST(
    "/api/v1/nodes/{nodeId}/network-proxies",
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

export async function updateNetworkProxy(
  nodeId: string,
  proxyId: string,
  input: NetworkProxyUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PUT(
    "/api/v1/nodes/{nodeId}/network-proxies/{proxyId}",
    {
      params: { path: { nodeId, proxyId } },
      body: input,
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function deleteNetworkProxy(
  nodeId: string,
  proxyId: string,
  csrfToken: string,
) {
  const result = await apiClient.DELETE(
    "/api/v1/nodes/{nodeId}/network-proxies/{proxyId}",
    {
      params: { path: { nodeId, proxyId } },
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
