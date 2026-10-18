import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import { msg } from "@/lib/i18n";
import { getScopeOptions } from "./scope-api";
import type { Grant } from "./manage-api";

type Scope = Pick<Grant, "corporations" | "alliances">;

export function ScopePicker({ user, label, value, disabled, onChange }: {
  user: string; label: string; value: Scope; disabled: boolean; onChange: (value: Scope) => void;
}) {
  const query = useQuery({ queryKey: ["access", "scope-options", user], queryFn: ({ signal }) => getScopeOptions(signal) });
  return <div className="access-scope-picker">
    {query.isPending && <p role="status">{msg("正在读取")}</p>}
    {query.isError && <div role="alert" className="access-scope-error">
      <span>{msg("军团与联盟读取失败")}</span>
      <Button type="button" variant="outline" disabled={disabled || query.isFetching} onClick={() => void query.refetch()}>{msg("重试")}</Button>
    </div>}
    <div className="access-filter-fields">
      {(["corporations", "alliances"] as const).map((kind) => {
        const available = query.data?.[kind] ?? [];
        const title = kind === "corporations" ? msg("军团") : msg("联盟");
        const itemName = (id: string) => available.find((item) => item.id === id)?.name ||
          (kind === "corporations" ? msg("军团 #{0}", id) : msg("联盟 #{0}", id));
        return <div className="access-field" key={kind}>
          <span>{title}</span>
          <Select label={msg("{0}：{1}", label, title)} value="" disabled={disabled || !query.isSuccess || !available.some((item) => !value[kind].includes(item.id))}
            placeholder={available.length ? (kind === "corporations" ? msg("选择军团") : msg("选择联盟")) : msg("暂无可选项")}
            options={available.filter((item) => !value[kind].includes(item.id)).map((item) => ({ value: item.id, label: itemName(item.id) }))}
            onValueChange={(id) => onChange({ ...value, [kind]: [...new Set([...value[kind], id])] })} />
          {value[kind].length > 0 && <ul className="access-scope-selected">
            {value[kind].map((id) => <li key={id}>
              <span>{itemName(id)}</span>
              <IconAction type="button" label={msg("移除{0}", itemName(id))} disabled={disabled}
                onClick={() => onChange({ ...value, [kind]: value[kind].filter((item) => item !== id) })}><X aria-hidden="true" /></IconAction>
            </li>)}
          </ul>}
        </div>;
      })}
    </div>
  </div>;
}
