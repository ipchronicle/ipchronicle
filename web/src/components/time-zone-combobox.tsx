import { useMemo, useState } from "react";
import { ChevronsUpDown } from "lucide-react";
import { useTranslation } from "react-i18next";

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
import { supportedTimeZones } from "@/lib/time-zone";

export function TimeZoneCombobox({
  id,
  value,
  disabled = false,
  onValueChange,
}: {
  id: string;
  value: string;
  disabled?: boolean;
  onValueChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const timeZones = useMemo(() => supportedTimeZones(value), [value]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label={t("probe.settings.timezone")}
          disabled={disabled}
          className="w-full justify-between font-normal"
        >
          <span className="truncate">{value}</span>
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
          <CommandInput placeholder={t("probe.settings.timezoneSearch")} />
          <CommandList>
            <CommandEmpty>{t("probe.settings.timezoneEmpty")}</CommandEmpty>
            <CommandGroup>
              {timeZones.map((timeZone) => (
                <CommandItem
                  key={timeZone}
                  value={timeZone}
                  data-checked={timeZone === value}
                  onSelect={() => {
                    onValueChange(timeZone);
                    setOpen(false);
                  }}
                >
                  {timeZone}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
