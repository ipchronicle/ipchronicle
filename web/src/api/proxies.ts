import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type NetworkProxy = components["schemas"]["NetworkProxy"];
export type NetworkProxyCreate = components["schemas"]["NetworkProxyCreate"];
export type NetworkProxyUpdate = components["schemas"]["NetworkProxyUpdate"];

export async function listNetworkProxies(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/network-proxies", { signal });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data.items;
}

export async function createNetworkProxy(
  input: NetworkProxyCreate,
  csrfToken: string,
) {
  const result = await apiClient.POST("/api/v1/network-proxies", {
    body: input,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateNetworkProxy(
  proxyId: string,
  input: NetworkProxyUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/network-proxies/{proxyId}", {
    params: { path: { proxyId } },
    body: input,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function deleteNetworkProxy(proxyId: string, csrfToken: string) {
  const result = await apiClient.DELETE("/api/v1/network-proxies/{proxyId}", {
    params: { path: { proxyId } },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok) {
    throwAPIError(result.response, result.error);
  }
}
