import { useEffect, useId, useState, type KeyboardEvent } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { getData } from "@/lib/http";
import { msg } from "@/lib/i18n";
import { list as fittingList } from "@/modules/fittings/library-api";
import type { Fit } from "@/modules/fittings/model";

type ChoiceMeta = { id?: string; dna?: string };
type Character = { id: string; name: string };

// DNA is a frozen snapshot: later library edits must not change a saved notice.
export function fittingDNA(fit: Fit): string {
  if (!/^[1-9]\d*$/.test(fit.ship_type_id)) return "";
  const items = new Map<string, number>();
  const add = (id: string, amount: number) => {
    if (!/^[1-9]\d*$/.test(id) || !Number.isSafeInteger(amount) || amount < 1) return false;
    const total = (items.get(id) ?? 0) + amount;
    if (!Number.isSafeInteger(total) || total > 1000000000) return false;
    items.set(id, total);
    return true;
  };
  for (const item of fit.items) {
    if (!add(item.type_id, item.quantity)) return "";
    if (item.charge_id && !add(item.charge_id, item.quantity)) return "";
  }
  const dna = `${fit.ship_type_id}${[...items].map(([id, count]) => `:${id};${count}`).join("")}::`;
  return dna.length <= 6000 ? dna : "";
}

type System = { id: string; name: string; name_zh: string; name_en: string };
const systems = (v: unknown): v is System[] => Array.isArray(v) && v.every(item =>
  item && typeof item === "object" && typeof item.id === "string" &&
  typeof item.name === "string" && typeof item.name_zh === "string" && typeof item.name_en === "string"
);

export function FleetChoice({ label, kind, corporationID, user, value, onChange, maxLength, required = false, characters = [] }: {
  label: string; kind: "system" | "doctrine" | "character"; corporationID: string; user: string; value: string;
  onChange: (value: string, meta?: ChoiceMeta) => void; maxLength: number; required?: boolean; characters?: Character[];
}) {
  const listID = useId();
  const inputID = useId();
  const [expanded, setExpanded] = useState(false);
  const [active, setActive] = useState(-1);
  const [filter, setFilter] = useState("");
  const [term, setTerm] = useState(value.trim());
  useEffect(() => {
    const timer = window.setTimeout(() => setTerm(value.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [value]);
  useEffect(() => {
    if (expanded && active >= 0) document.getElementById(`${listID}-${active}`)?.scrollIntoView({block: "nearest"});
  }, [expanded, active, listID]);
  const systemQuery = useQuery({
    queryKey: ["eve", "solar-system-search", term],
    queryFn: ({ signal }) => getData(`/api/v1/eve/solar-systems/search?q=${encodeURIComponent(term)}`, systems, signal),
    enabled: kind === "system",
    placeholderData: keepPreviousData,
    staleTime: 60_000,
  });
  const fitsQuery = useQuery({
    queryKey: ["fittings", "fleet-notice", user, corporationID],
    queryFn: ({ signal }) => fittingList(corporationID, signal),
    enabled: kind === "doctrine" && !!corporationID,
    staleTime: 60_000,
  });
  const error = kind === "system" ? systemQuery.isError : kind === "doctrine" && fitsQuery.isError;
  const searching = kind === "system" ? systemQuery.isFetching || term !== value.trim() : kind === "doctrine" && fitsQuery.isFetching;
  const options: ({ value: string; label: string } & ChoiceMeta)[] = (kind === "system"
    ? (systemQuery.data ?? []).map(item => ({ value: item.name, label: item.name_en && item.name_en !== item.name ? item.name_en : item.name_zh }))
    : kind === "doctrine" ? (fitsQuery.data ?? []).map(item => ({ value: item.name, label: item.ship_name, dna: fittingDNA(item.fit) }))
    : characters.map(item => ({ value: item.name, label: "", id: item.id })))
    .filter(option => `${option.value} ${option.label}`.toLocaleLowerCase().includes(filter.trim().toLocaleLowerCase()));
  const select = (index: number) => {
    if (!options[index]) return;
    onChange(options[index].value, options[index]);
    setFilter("");
    setExpanded(false);
    setActive(-1);
  };
  const keyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      setExpanded(true);
      setActive(current => event.key === "ArrowDown"
        ? Math.min(current + 1, options.length - 1)
        : current <= 0 ? options.length - 1 : current - 1);
    } else if (event.key === "Enter" && expanded && active >= 0 && active < options.length) {
      event.preventDefault();
      select(active);
    } else if (event.key === "Escape" && expanded) {
      event.preventDefault();
      event.stopPropagation();
      setExpanded(false);
      setActive(-1);
    }
  };
  return <div className="fleet-choice-field"><label htmlFor={inputID}>{label}</label><div className="fleet-choice">
    <input id={inputID} role="combobox" aria-expanded={expanded} aria-controls={listID} aria-autocomplete="list"
      aria-activedescendant={expanded && active >= 0 && active < options.length ? `${listID}-${active}` : undefined}
      value={value} maxLength={maxLength} required={required} autoComplete="off"
      placeholder={msg("输入搜索或自行填写")} onFocus={() => { setFilter(""); setExpanded(true); }} onClick={() => setExpanded(true)}
      onBlur={() => { setExpanded(false); setActive(-1); }}
      onKeyDown={keyDown} onChange={event => { onChange(event.target.value); setFilter(event.target.value); setExpanded(true); setActive(-1); }} />
    <button type="button" tabIndex={-1} aria-label={msg("显示选项")} onMouseDown={event => event.preventDefault()}
      onClick={() => { document.getElementById(inputID)?.focus(); setExpanded(true); }}><ChevronDown size={16} aria-hidden="true" /></button>
    {expanded && <div className="fleet-choice-menu" id={listID} role="listbox">
      {options.map((option, index) => <div id={`${listID}-${index}`} key={`${option.value}-${index}`} role="option"
        aria-selected={index === active} className={index === active ? "is-active" : undefined}
        onMouseDown={event => event.preventDefault()} onClick={() => select(index)}>
        <span>{option.value}</span>{option.label && option.label !== option.value && <small>{option.label}</small>}
      </div>)}
      {options.length === 0 && <div className="fleet-choice-empty">{msg(searching ? "正在读取" : "没有匹配的选项")}</div>}
    </div>}
  </div>{error && <span className="fleet-choice-error" role="status">{msg("选项读取失败，可手动填写")}</span>}</div>;
}
