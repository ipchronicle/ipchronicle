import { apiClient } from "@/api/client";
import { recordCSRFTokenState, throwAPIError } from "@/api/errors";
import type { components } from "@/api/schema";

export type Account = components["schemas"]["Account"];
export type AuthenticatedSession =
  components["schemas"]["AuthenticatedSession"];
export type LoginRequest = components["schemas"]["LoginRequest"];
export type AccountUpdateRequest =
  components["schemas"]["AccountUpdateRequest"];
export type TOTPEnrollment = components["schemas"]["TOTPEnrollment"];

let csrfToken = "";

function setCSRFToken(value: string) {
  csrfToken = value;
  recordCSRFTokenState(value !== "");
}

function mutationHeaders() {
  return { "X-CSRF-Token": csrfToken };
}

export async function getAuthenticatedSession(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/auth/session", { signal });
  if (result.response.status === 401) {
    setCSRFToken("");
    return null;
  }
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  setCSRFToken(result.data.csrfToken);
  return result.data;
}

export async function login(body: LoginRequest) {
  const result = await apiClient.POST("/api/v1/auth/login", { body });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  setCSRFToken(result.data.csrfToken);
  return result.data;
}

export async function logout() {
  const result = await apiClient.POST("/api/v1/auth/logout", {
    headers: mutationHeaders(),
  });
  if (!result.response.ok) {
    throwAPIError(result.response, result.error);
  }
  setCSRFToken("");
}

export async function updateAccount(body: AccountUpdateRequest) {
  const result = await apiClient.PATCH("/api/v1/account", {
    body,
    headers: mutationHeaders(),
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  if (result.data.sessionRevoked) {
    setCSRFToken("");
  }
  return result.data;
}

export async function updateAccountLocale(locale: Account["locale"]) {
  const result = await apiClient.PUT("/api/v1/account/locale", {
    body: { locale },
    headers: mutationHeaders(),
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function startTOTPEnrollment(currentPassword: string) {
  const result = await apiClient.POST("/api/v1/account/totp/enrollment", {
    body: { currentPassword },
    headers: mutationHeaders(),
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function confirmTOTPEnrollment(code: string) {
  const result = await apiClient.POST("/api/v1/account/totp", {
    body: { code },
    headers: mutationHeaders(),
  });
  if (!result.response.ok || result.data === undefined) {
    throwAPIError(result.response, result.error);
  }
  return result.data;
}

export async function disableTOTP(currentPassword: string, code: string) {
  const result = await apiClient.DELETE("/api/v1/account/totp", {
    body: { currentPassword, code },
    headers: mutationHeaders(),
  });
  if (!result.response.ok) {
    throwAPIError(result.response, result.error);
  }
  setCSRFToken("");
}

export async function revokeAllSessions() {
  const result = await apiClient.DELETE("/api/v1/account/sessions", {
    headers: mutationHeaders(),
  });
  if (!result.response.ok) {
    throwAPIError(result.response, result.error);
  }
  setCSRFToken("");
}
