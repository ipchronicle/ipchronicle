export function browserTimeZone(): string {
  const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return timeZone || "UTC";
}

export function supportedTimeZones(selected?: string): string[] {
  const values = new Set(Intl.supportedValuesOf("timeZone"));
  values.add("UTC");
  values.add(browserTimeZone());
  if (selected) values.add(selected);
  return [...values].sort((left, right) => left.localeCompare(right));
}
