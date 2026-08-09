import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type HistoryFilters = {
  nodeId?: string;
  egressId?: string;
  from?: string;
  to?: string;
  page?: number;
  pageSize?: number;
};

export type ProbeHistoryFilters = HistoryFilters & {
  runStatus?: components["schemas"]["ProbeRunStatus"];
  trigger?: components["schemas"]["ProbeTrigger"];
  changed?: boolean;
  formatStatus?: components["schemas"]["ProbeFormatStatus"];
};

export type AddressHistoryFilters = HistoryFilters & {
  gapPage?: number;
  eventKind?: components["schemas"]["AddressEventKind"];
  family?: components["schemas"]["AddressFamily"];
};

export type ProbeSnapshotHistoryPage =
  components["schemas"]["ProbeSnapshotHistoryPage"];
export type AddressHistoryPage = components["schemas"]["AddressHistoryPage"];
export type ProbeHistoryGapPage = components["schemas"]["ProbeHistoryGapPage"];
export type ProbeFormatEventPage =
  components["schemas"]["ProbeFormatEventPage"];
export type ProbeSnapshotComparison =
  components["schemas"]["ProbeSnapshotComparison"];
export type HistoryRetentionUpdate =
  components["schemas"]["HistoryRetentionUpdate"];

export async function listHistoryProbeSnapshots(
  filters: ProbeHistoryFilters,
  signal?: AbortSignal,
) {
  const result = await apiClient.GET("/api/v1/history/probe-snapshots", {
    params: { query: filters },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function listHistoryAddressEvents(
  filters: AddressHistoryFilters,
  signal?: AbortSignal,
) {
  const result = await apiClient.GET("/api/v1/history/address-events", {
    params: { query: filters },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function listHistoryProbeGaps(
  filters: HistoryFilters,
  signal?: AbortSignal,
) {
  const result = await apiClient.GET("/api/v1/history/probe-gaps", {
    params: { query: filters },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function listHistoryFormatEvents(
  filters: HistoryFilters,
  signal?: AbortSignal,
) {
  const result = await apiClient.GET("/api/v1/history/format-events", {
    params: { query: filters },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function compareProbeSnapshots(
  beforeSnapshotId: string,
  afterSnapshotId: string,
  signal?: AbortSignal,
) {
  const result = await apiClient.GET("/api/v1/history/comparison", {
    params: { query: { beforeSnapshotId, afterSnapshotId } },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function setProbeSnapshotStarred(
  snapshotId: string,
  starred: boolean,
  csrfToken: string,
) {
  const options = {
    params: { path: { snapshotId } },
    headers: { "X-CSRF-Token": csrfToken },
  } as const;
  const result = starred
    ? await apiClient.PUT("/api/v1/probe-snapshots/{snapshotId}/star", options)
    : await apiClient.DELETE(
        "/api/v1/probe-snapshots/{snapshotId}/star",
        options,
      );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateHistoryRetention(
  update: HistoryRetentionUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/history/retention", {
    body: update,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function cleanupHistory(csrfToken: string) {
  const result = await apiClient.POST("/api/v1/history/cleanup", {
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
