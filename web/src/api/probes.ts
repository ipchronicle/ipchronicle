import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type NodeProbeState = components["schemas"]["NodeProbeState"];
export type NodeProbeSettingsUpdate =
  components["schemas"]["NodeProbeSettingsUpdate"];
export type CompleteProbeTaskCreate =
  components["schemas"]["CompleteProbeTaskCreate"];
export type ProbeTask = components["schemas"]["ProbeTask"];
export type ProbeRun = components["schemas"]["ProbeRun"];
export type ProbeRunSummary = components["schemas"]["ProbeRunSummary"];
export type ProbeExecution = components["schemas"]["ProbeExecution"];
export type ProbeSnapshot = components["schemas"]["ProbeSnapshot"];
export type HistoryState = components["schemas"]["HistoryState"];

export async function getNodeProbe(nodeId: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/nodes/{nodeId}/probe", {
    params: { path: { nodeId } },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateNodeProbeSettings(
  nodeId: string,
  update: NodeProbeSettingsUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/nodes/{nodeId}/probe", {
    params: { path: { nodeId } },
    body: update,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function createCompleteProbeTask(
  nodeId: string,
  input: CompleteProbeTaskCreate,
  csrfToken: string,
) {
  const result = await apiClient.POST("/api/v1/nodes/{nodeId}/probe/tasks", {
    params: { path: { nodeId } },
    body: input,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function getProbeRun(runId: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/probe-runs/{runId}", {
    params: { path: { runId } },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function getProbeSnapshot(
  snapshotId: string,
  signal?: AbortSignal,
) {
  const result = await apiClient.GET("/api/v1/probe-snapshots/{snapshotId}", {
    params: { path: { snapshotId } },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function getHistoryState(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/history", { signal });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function resetHistory(csrfToken: string) {
  const result = await apiClient.DELETE("/api/v1/history", {
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
