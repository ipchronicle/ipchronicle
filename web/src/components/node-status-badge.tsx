import { Trash2, Wifi, WifiOff } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { Node } from "@/api/nodes";
import { Badge } from "@/components/ui/badge";

export function NodeStatusBadge({ node }: { node: Node }) {
  const { t } = useTranslation();
  const labels = {
    online: t("nodes.status.online"),
    offline: t("nodes.status.offline"),
    disabled: t("nodes.status.disabled"),
    revoked: t("nodes.status.revoked"),
  };
  if (node.deletionStatus !== undefined) {
    return (
      <Badge variant="destructive">
        <Trash2 aria-hidden="true" />
        {node.deletionStatus === "failed"
          ? t("nodes.deletion.failed")
          : t("nodes.deletion.pending")}
      </Badge>
    );
  }
  return (
    <Badge
      variant={
        node.status === "online"
          ? "outline"
          : node.status === "offline"
            ? "secondary"
            : "destructive"
      }
    >
      {node.status === "online" ? (
        <Wifi aria-hidden="true" />
      ) : (
        <WifiOff aria-hidden="true" />
      )}
      {labels[node.status]}
    </Badge>
  );
}
