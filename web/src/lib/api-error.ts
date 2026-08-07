import type { TFunction } from "i18next";

import { APIError } from "@/api/errors";

export function formatAPIError(error: unknown, t: TFunction) {
  if (error instanceof APIError) {
    return t(`errors.${error.code}`, error.parameters ?? {});
  }
  return t("errors.internal_error");
}
