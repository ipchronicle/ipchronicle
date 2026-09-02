import { useMemo, useState } from "react";
import { ChevronsUpDown } from "lucide-react";
import { useTranslation } from "react-i18next";

import type { NotificationProbeField } from "@/api/notifications";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  presentProbeField,
  presentProbeFieldGroup,
} from "@/lib/probe-field-label";

export const allProbeFieldsValue = "__all_probe_fields__";

export function ProbeFieldCombobox({
  id,
  fields,
  value,
  disabled = false,
  onValueChange,
}: {
  id: string;
  fields: NotificationProbeField[];
  value: string;
  disabled?: boolean;
  onValueChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const groups = useMemo(() => {
    const result = new Map<string, NotificationProbeField[]>();
    for (const field of fields) {
      const group = result.get(field.group) ?? [];
      group.push(field);
      result.set(field.group, group);
    }
    return [...result.entries()];
  }, [fields]);
  const selected = fields.find((field) => field.id === value);
  const selectedLabel = selected
    ? presentProbeField(selected, t).name
    : t("notifications.rules.allFields");

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={t("notifications.rules.field")}
          disabled={disabled}
          className="w-full justify-between font-normal"
        >
          <span className="truncate">{selectedLabel}</span>
          <ChevronsUpDown
            aria-hidden="true"
            className="text-muted-foreground"
          />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-(--radix-popover-trigger-width) p-0"
      >
        <Command>
          <CommandInput placeholder={t("notifications.rules.fieldSearch")} />
          <CommandList>
            <CommandEmpty>{t("notifications.rules.fieldEmpty")}</CommandEmpty>
            <CommandGroup>
              <CommandItem
                value={t("notifications.rules.allFields")}
                data-checked={value === allProbeFieldsValue}
                onSelect={() => {
                  onValueChange(allProbeFieldsValue);
                  setOpen(false);
                }}
              >
                {t("notifications.rules.allFields")}
              </CommandItem>
            </CommandGroup>
            {groups.map(([group, items]) => (
              <CommandGroup
                key={group}
                heading={presentProbeFieldGroup(group, t).name}
              >
                {items.map((field) => {
                  const presentation = presentProbeField(field, t);
                  return (
                    <CommandItem
                      key={field.id}
                      value={`${presentation.name} ${field.id}`}
                      data-checked={field.id === value}
                      onSelect={() => {
                        onValueChange(field.id);
                        setOpen(false);
                      }}
                    >
                      {presentation.name}
                    </CommandItem>
                  );
                })}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
