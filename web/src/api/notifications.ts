import { apiClient } from "@/api/client";
import { throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type NotificationSender = components["schemas"]["NotificationSender"];
export type NotificationSenderCreate =
  components["schemas"]["NotificationSenderCreate"];
export type NotificationSenderUpdate =
  components["schemas"]["NotificationSenderUpdate"];
export type TelegramSenderCreate =
  components["schemas"]["TelegramSenderCreate"];
export type NotificationRule = components["schemas"]["NotificationRule"];
export type NotificationRuleWrite =
  components["schemas"]["NotificationRuleWrite"];
export type NotificationEventType =
  components["schemas"]["NotificationEventType"];
export type NotificationProbeField =
  components["schemas"]["NotificationProbeField"];
export type NotificationDelivery =
  components["schemas"]["NotificationDelivery"];
export type NotificationDeliveryStatus =
  components["schemas"]["NotificationDeliveryStatus"];

export async function listNotificationSenders(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/notification-senders", {
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data.items;
}

export async function listNotificationProbeFields(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/notification-probe-fields", {
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data.items;
}

export async function createNotificationSender(
  input: NotificationSenderCreate,
  csrfToken: string,
) {
  const result = await apiClient.POST("/api/v1/notification-senders", {
    body: input,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function testTelegramNotificationSender(
  input: TelegramSenderCreate,
  csrfToken: string,
) {
  const result = await apiClient.POST("/api/v1/notification-telegram-tests", {
    body: input,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok) {
    throwAPIError(result.response, result.error);
  }
}

export async function updateNotificationSender(
  senderId: string,
  input: NotificationSenderUpdate,
  csrfToken: string,
) {
  const result = await apiClient.PUT(
    "/api/v1/notification-senders/{senderId}",
    {
      params: { path: { senderId } },
      body: input,
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function deleteNotificationSender(
  senderId: string,
  csrfToken: string,
) {
  const result = await apiClient.DELETE(
    "/api/v1/notification-senders/{senderId}",
    {
      params: { path: { senderId } },
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok) {
    throwAPIError(result.response, result.error);
  }
}

export async function createNotificationTestDelivery(
  senderId: string,
  csrfToken: string,
) {
  const result = await apiClient.POST(
    "/api/v1/notification-senders/{senderId}/test-deliveries",
    {
      params: { path: { senderId } },
      headers: { "X-CSRF-Token": csrfToken },
    },
  );
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function listNotificationRules(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/notification-rules", {
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data.items;
}

export async function createNotificationRule(
  input: NotificationRuleWrite,
  csrfToken: string,
) {
  const result = await apiClient.POST("/api/v1/notification-rules", {
    body: input,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function updateNotificationRule(
  ruleId: string,
  input: NotificationRuleWrite,
  csrfToken: string,
) {
  const result = await apiClient.PUT("/api/v1/notification-rules/{ruleId}", {
    params: { path: { ruleId } },
    body: input,
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function deleteNotificationRule(
  ruleId: string,
  csrfToken: string,
) {
  const result = await apiClient.DELETE("/api/v1/notification-rules/{ruleId}", {
    params: { path: { ruleId } },
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (!result.response.ok) {
    throwAPIError(result.response, result.error);
  }
}

export async function listNotificationDeliveries(
  filters: {
    senderId?: string;
    status?: NotificationDeliveryStatus;
    page?: number;
    pageSize?: number;
  },
  signal?: AbortSignal,
) {
  const result = await apiClient.GET("/api/v1/notification-deliveries", {
    params: { query: filters },
    signal,
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}
