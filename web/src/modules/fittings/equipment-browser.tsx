import { msg, getLocale } from "@/lib/i18n";
import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plus, Search } from "lucide-react";
import { EveImage } from "@/components/eve-image";
import { Select } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { compatibleCharge, groupLabel } from "./catalog-ui";
import type { Catalog, TypeInfo } from "./sde";
import { slotNames, type Slot } from "./model";
import * as api from "./api";

export function EquipmentBrowser({
  catalog,
  pick,
  module,
  user,
  onSelect,
  onPick,
}: {
  catalog: Catalog;
  pick: Slot | "ship" | number;
  module?: TypeInfo;
  user: string;
  onSelect: (item: TypeInfo) => void;
  onPick?: (slot: Slot) => void;
}) {
  const [query, setQuery] = useState("");
  const [term, setTerm] = useState("");
  const [group, setGroup] = useState("all");
  const [limit, setLimit] = useState(60);
  const search = useRef<HTMLInputElement>(null);
  useEffect(() => {
    search.current?.focus();
  }, []);
  useEffect(() => {
    const timer = setTimeout(() => {
      setTerm(query.trim());
      setLimit(60);
    }, 220);
    return () => clearTimeout(timer);
  }, [query]);
  const q = useQuery({
    queryKey: ["fittings", user, "search", term],
    queryFn: ({ signal }) => api.search(term, signal),
    enabled: term.length >= 2,
  });
  const allowed = useMemo(
    () =>
      catalog.types.filter((t) =>
        pick === "ship"
          ? t.category === 6
          : typeof pick === "number"
            ? compatibleCharge(t, module, catalog)
            : pick === "cargo"
              ? t.category !== 6
              : t.slot === pick,
      ),
    [catalog, pick, module],
  );
  const groups = [...new Set(allowed.map((t) => t.group))]
    .map((id) => ({ value: String(id), label: groupLabel(id, catalog) }))
    .sort((a, b) => a.label.localeCompare(b.label, getLocale()));
  const remote = new Map((q.data ?? []).map((n) => [n.id, n.name]));
  const matches = allowed.filter(
    (t) =>
      (group === "all" || String(t.group) === group) &&
      (!term ||
        remote.has(String(t.id)) ||
        t.name.toLowerCase().includes(term.toLowerCase()) ||
        String(t.id) === term),
  );
  const visible = matches.slice(0, limit);
  const ids = visible.map((t) => String(t.id));
  const names = useQuery({
    queryKey: ["fittings", user, "names", ids],
    queryFn: ({ signal }) => api.names(ids, signal),
    enabled: ids.length > 0,
    staleTime: 3600000,
  });
  return (
    <div className="fit-equipment-browser">
      {onPick && typeof pick !== "number" && pick !== "ship" && (
        <Select
          label={msg("装备槽位")}
          value={pick}
          onValueChange={(v) => onPick(v as Slot)}
          options={(
            [
              "high",
              "medium",
              "low",
              "rig",
              "subsystem",
              "service",
              "drone_bay",
              "fighter_bay",
              "cargo",
            ] as Slot[]
          ).map((slot) => ({ value: slot, label: slotNames[slot] }))}
        />
      )}
      <label htmlFor="fit-search">{msg("物品名称")}</label>
      <div className="fit-search">
        <Search size={18} aria-hidden="true" />
        <input
          ref={search}
          id="fit-search"
          value={query}
          maxLength={80}
          placeholder={msg("搜索名称或 ID")}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>
      <Select
        label={pick === "ship" ? msg("舰船分类") : msg("装备分类")}
        value={group}
        onValueChange={(v) => {
          setGroup(v);
          setLimit(60);
        }}
        options={[{ value: "all", label: msg("全部分类") }, ...groups]}
      />
      {typeof pick === "number" && (
        <p className="fit-muted">{msg("匹配当前装备的弹药")}</p>
      )}
      {q.isError && (
        <p role="alert" className="fit-alert">
          {msg("中文搜索暂不可用，可搜索英文名称")}{" "}
        </p>
      )}
      <div
        className="fit-picker-results"
        aria-busy={q.isFetching || names.isFetching}
      >
        {visible.map((t) => (
          <button key={t.id} onClick={() => onSelect(t)}>
            <EveImage kind="type" id={String(t.id)} />
            <span>
              {names.data?.find((n) => n.id === String(t.id))?.name ??
                remote.get(String(t.id)) ??
                t.name}
              <small>{groupLabel(t.group, catalog)}</small>
            </span>
            <Plus size={16} aria-hidden="true" />
          </button>
        ))}
        {!matches.length && (
          <p role="status">
            {q.isFetching
              ? msg("正在搜索")
              : term.length < 2 && /[\u4e00-\u9fff]/.test(term)
                ? msg("请输入至少两个字搜索")
                : msg("未找到匹配物品")}
          </p>
        )}
      </div>
      {matches.length > visible.length && (
        <Button
          variant="outline"
          onClick={() => setLimit((n) => Math.min(n + 60, 480))}
          disabled={limit >= 480}
        >
          {limit >= 480
            ? msg("请缩小筛选范围")
            : msg("显示更多 · {0}/{1}", visible.length, matches.length)}
        </Button>
      )}
    </div>
  );
}
