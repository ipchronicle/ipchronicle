import { History, TriangleAlert } from "lucide-react";
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

type TimelineItem =
  | {
      type: "event";
      id: string;
      time: string;
      event: NodeNetworkState["addressEvents"][number];
    }
  | {
      type: "gap";
      id: string;
      time: string;
      gap: NodeNetworkState["addressGaps"][number];
    };

export function NodeAddressHistory({ network }: { network: NodeNetworkState }) {
  const { i18n, t } = useTranslation();
  const items: TimelineItem[] = [
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
  const groups = groupByDay(items, i18n.resolvedLanguage);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <History aria-hidden="true" className="size-4" />
          {t("network.addressHistory.title")}
        </CardTitle>
        <CardDescription>{t("network.addressHistory.detail")}</CardDescription>
        <CardAction>
          <Badge variant="info">{items.length}</Badge>
        </CardAction>
      </CardHeader>
      <CardContent>
        {items.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            {t("network.addressHistory.empty")}
          </p>
        ) : (
          <div className="space-y-6">
            {groups.map((group) => (
              <section key={group.label}>
                <h3 className="mb-4 text-sm font-medium text-muted-foreground">
                  {group.label}
                </h3>
                <div>
                  {group.items.map((item, index) =>
                    item.type === "event" ? (
                      <EventTimelineItem
                        key={item.id}
                        event={item.event}
                        last={index === group.items.length - 1}
                      />
                    ) : (
                      <GapTimelineItem
                        key={item.id}
                        gap={item.gap}
                        last={index === group.items.length - 1}
                      />
                    ),
                  )}
                </div>
              </section>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function EventTimelineItem({
  event,
  last,
}: {
  event: NodeNetworkState["addressEvents"][number];
  last: boolean;
}) {
  const { i18n, t } = useTranslation();
  const positive =
    event.kind === "first-observation" ||
    event.kind === "address-added" ||
    event.kind === "recovery";
  const failure = event.kind === "check-failure";
  const markerClassName = failure
    ? "border-destructive bg-destructive"
    : positive
      ? "border-emerald-500 bg-emerald-500"
      : "border-amber-500 bg-amber-500";
  return (
    <div className="relative grid min-w-0 grid-cols-[22px_76px_minmax(0,1fr)] gap-3 pb-6 last:pb-0 sm:grid-cols-[22px_94px_minmax(0,1fr)_auto]">
      {!last ? (
        <span className="absolute top-3 bottom-0 left-[7px] w-px bg-border" />
      ) : null}
      <span
        className={`relative z-10 mt-1 size-4 rounded-full border-4 ring-1 ring-background ${markerClassName}`}
      />
      <time className="text-sm text-muted-foreground">
        {new Intl.DateTimeFormat(i18n.resolvedLanguage, {
          hour: "2-digit",
          minute: "2-digit",
        }).format(new Date(event.observedAt))}
      </time>
      <div className="min-w-0">
        <p
          className={
            positive
              ? "font-medium text-emerald-700 dark:text-emerald-300"
              : failure
                ? "font-medium text-destructive"
                : "font-medium text-amber-700 dark:text-amber-300"
          }
        >
          {t(`network.addressHistory.kind.${event.kind}`)}
        </p>
        <p className="mt-1 break-all font-mono text-sm">
          {event.publicAddress ?? t("network.observation.unknown")}
        </p>
        {event.failureReason ? (
          <p className="mt-1 text-sm text-muted-foreground">
            {t(`network.observation.failure.${event.failureReason}`)}
          </p>
        ) : null}
      </div>
      <Badge variant="outline" className="col-start-3 sm:col-start-4">
        {t(`network.family.${event.family}`)}
      </Badge>
    </div>
  );
}

function GapTimelineItem({
  gap,
  last,
}: {
  gap: NodeNetworkState["addressGaps"][number];
  last: boolean;
}) {
  const { i18n, t } = useTranslation();
  return (
    <div className="relative grid grid-cols-[22px_76px_minmax(0,1fr)] gap-3 pb-6 last:pb-0 sm:grid-cols-[22px_94px_minmax(0,1fr)]">
      {!last ? (
        <span className="absolute top-3 bottom-0 left-[7px] w-px bg-border" />
      ) : null}
      <span className="relative z-10 mt-1 flex size-4 items-center justify-center rounded-full bg-destructive text-destructive-foreground ring-2 ring-background">
        <TriangleAlert aria-hidden="true" className="size-2.5" />
      </span>
      <time className="text-sm text-muted-foreground">
        {new Intl.DateTimeFormat(i18n.resolvedLanguage, {
          hour: "2-digit",
          minute: "2-digit",
        }).format(new Date(gap.lastObservedAt))}
      </time>
      <div className="rounded-md border border-destructive/40 bg-destructive/5 p-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="destructive">{t("network.addressHistory.gap")}</Badge>
          <Badge variant="outline">
            {t("network.addressHistory.nodeLevel")}
          </Badge>
        </div>
        <p className="mt-2 text-sm">
          {t("network.addressHistory.gapDetail", {
            count: gap.droppedCount,
            first: gap.firstSequence,
            last: gap.lastSequence,
          })}
        </p>
      </div>
    </div>
  );
}

function groupByDay(items: TimelineItem[], locale: string | undefined) {
  const groups = new Map<string, TimelineItem[]>();
  const formatter = new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "long",
    day: "numeric",
  });
  for (const item of items) {
    const label = formatter.format(new Date(item.time));
    const group = groups.get(label);
    if (group) group.push(item);
    else groups.set(label, [item]);
  }
  return Array.from(groups, ([label, groupedItems]) => ({
    label,
    items: groupedItems,
  }));
}
