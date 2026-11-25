import { useEffect, useId, useState, type KeyboardEvent } from "react";
import { Check, ChevronDown } from "lucide-react";
import { Popover } from "radix-ui";
import { SearchInput } from "@/components/search-input";
import { msg } from "@/lib/i18n";
import { normalizeSearch } from "@/lib/use-search-draft";
import "./select.css";

export function SearchableSelect({ label, value, options, onValueChange }: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onValueChange: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [active, setActive] = useState(-1);
  const id = useId();
  const listID = `${id}-options`;
  const searchLabel = msg("搜索{0}", label);
  const term = normalizeSearch(search).toLocaleLowerCase();
  const matches = options.filter((option) =>
    option.label.toLocaleLowerCase().includes(term) || option.value.toLocaleLowerCase().includes(term),
  );
  const activeID = open && matches[active] ? `${listID}-${active}` : undefined;

  useEffect(() => {
    if (activeID) document.getElementById(activeID)?.scrollIntoView({ block: "nearest" });
  }, [activeID]);

  const changeOpen = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setSearch("");
      setActive(-1);
    }
  };
  const updateSearch = (next: string) => {
    setSearch(next);
    setActive(-1);
  };
  const select = (next: string) => {
    changeOpen(false);
    if (next !== value) onValueChange(next);
  };
  const handleKey = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const last = matches.length - 1;
      setActive(event.key === "ArrowDown" ? Math.min(active + 1, last) : active <= 0 ? last : active - 1);
    } else if (event.key === "Enter") {
      event.preventDefault();
      if (matches[active]) select(matches[active].value);
    }
  };

  return (
    <Popover.Root open={open} onOpenChange={changeOpen}>
      <Popover.Trigger type="button" aria-label={label} aria-describedby={`${id}-value`} className="ui-select-trigger">
        <span id={`${id}-value`} className="ui-select-value">
          {options.find((option) => option.value === value)?.label ?? label}
        </span>
        <ChevronDown className="ui-select-chevron" size={16} aria-hidden="true" />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content className="ui-select-content ui-searchable-select" aria-label={label} align="start" sideOffset={6} collisionPadding={12}>
          <div className="ui-searchable-select-search">
            <SearchInput
              role="combobox"
              aria-label={searchLabel}
              aria-expanded={open}
              aria-controls={listID}
              aria-activedescendant={activeID}
              aria-autocomplete="list"
              placeholder={searchLabel}
              autoComplete="off"
              value={search}
              onChange={(event) => updateSearch(event.target.value)}
              onClear={() => updateSearch("")}
              onKeyDown={handleKey}
            />
          </div>
          <div id={listID} className="ui-select-viewport ui-searchable-select-options" role="listbox" aria-label={label}>
            {matches.map((option, index) => (
              <div
                id={`${listID}-${index}`}
                key={option.value}
                role="option"
                aria-selected={option.value === value}
                data-highlighted={index === active ? "" : undefined}
                data-state={option.value === value ? "checked" : undefined}
                className="ui-select-item"
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => select(option.value)}
              >
                {option.label}
                {option.value === value && <span className="ui-select-check"><Check size={16} aria-hidden="true" /></span>}
              </div>
            ))}
          </div>
          {matches.length === 0 && <div role="status" className="ui-searchable-select-empty">{msg("没有匹配的选项")}</div>}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
