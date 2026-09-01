import { useTranslation } from "react-i18next";

import type { HistoryOwner } from "@/api/history";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export function historicalNodeName(owner: HistoryOwner, unavailable: string) {
  return owner.nodeName ?? unavailable;
}

export function HistoricalNodeName({
  owner,
  className,
}: {
  owner: HistoryOwner;
  className?: string;
}) {
  const { t } = useTranslation();
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-1.5", className)}>
      <span className="truncate">
        {historicalNodeName(owner, t("history.nodeUnavailable"))}
      </span>
      {owner.nodeDeleted ? (
        <Badge variant="secondary">{t("history.nodeDeleted")}</Badge>
      ) : null}
    </span>
  );
}
