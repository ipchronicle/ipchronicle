import type { components } from "@/api/schema";

type ErrorResponse = components["schemas"]["ErrorResponse"];

export class APIError extends Error {
  readonly status: number;
  readonly code: ErrorResponse["code"];
  readonly parameters?: Record<string, string>;

  constructor(status: number, body?: ErrorResponse) {
    super(body?.code ?? `http_${status}`);
    this.name = "APIError";
    this.status = status;
    this.code = body?.code ?? "internal_error";
    this.parameters = body?.parameters;
  }
}

export function throwAPIError(
  response: Response,
  error?: ErrorResponse,
): never {
  if (response.status === 401 && csrfTokenIsSet()) {
    window.dispatchEvent(new Event("ipchronicle:unauthenticated"));
  }
  throw new APIError(response.status, error);
}

let hasCSRFToken = false;

export function recordCSRFTokenState(value: boolean) {
  hasCSRFToken = value;
}

function csrfTokenIsSet() {
  return hasCSRFToken;
}
