import { History } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { NodeNetworkState } from "@/api/network";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export function NodeAddressHistory({ network }: { network: NodeNetworkState }) {
  const { i18n, t } = useTranslation();
  const items = [
    ...network.addressEvents.map((event) => ({
      type: "event" as const,
      id: event.id,
      time: event.observedAt,
      event,
    })),
    ...network.addressGaps.map((gap) => ({
      type: "gap" as const,
      id: gap.id,
      time: gap.lastObservedAt,
      gap,
    })),
  ].sort((left, right) => right.time.localeCompare(left.time));

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <History aria-hidden="true" className="size-4" />
          {t("network.addressHistory.title")}
        </CardTitle>
        <CardDescription>{t("network.addressHistory.detail")}</CardDescription>
        <CardAction>
          <Badge variant="secondary">{items.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-3">
        {items.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("network.addressHistory.empty")}
          </p>
        ) : (
          items.map((item) =>
            item.type === "event" ? (
              <div
                key={item.id}
                className="flex flex-wrap items-start justify-between gap-3 rounded-md border p-3"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge
                      variant={
                        item.event.kind === "check-failure"
                          ? "destructive"
                          : "outline"
                      }
                    >
                      {t(`network.addressHistory.kind.${item.event.kind}`)}
                    </Badge>
                    <Badge variant="secondary">
                      {item.event.family.toUpperCase()}
                    </Badge>
                    <span className="break-all font-mono text-xs">
                      {eventMapping(item.event, t)}
                    </span>
                  </div>
                  {item.event.failureReason ? (
                    <p className="mt-2 text-xs text-muted-foreground">
                      {t(
                        `network.observation.failure.${item.event.failureReason}`,
                      )}
                    </p>
                  ) : null}
                </div>
                <time className="text-xs whitespace-nowrap text-muted-foreground">
                  {new Date(item.event.observedAt).toLocaleString(
                    i18n.language,
                  )}
                </time>
              </div>
            ) : (
              <div
                key={item.id}
                className="flex flex-wrap items-start justify-between gap-3 rounded-md border border-destructive/40 p-3"
              >
                <div>
                  <Badge variant="destructive">
                    {t("network.addressHistory.gap")}
                  </Badge>
                  <Badge variant="secondary" className="ml-2">
                    {t("network.addressHistory.nodeLevel")}
                  </Badge>
                  <p className="mt-2 text-sm">
                    {t("network.addressHistory.gapDetail", {
                      count: item.gap.droppedCount,
                      first: item.gap.firstSequence,
                      last: item.gap.lastSequence,
                    })}
                  </p>
                </div>
                <time className="text-xs whitespace-nowrap text-muted-foreground">
                  {new Date(item.gap.lastObservedAt).toLocaleString(
                    i18n.language,
                  )}
                </time>
              </div>
            ),
          )
        )}
      </CardContent>
    </Card>
  );
}

function eventMapping(
  event: NodeNetworkState["addressEvents"][number],
  t: ReturnType<typeof useTranslation>["t"],
) {
  if (event.previousAddress && event.publicAddress) {
    return `${event.previousAddress} -> ${event.publicAddress}`;
  }
  return event.publicAddress ?? t("network.observation.unknown");
}
