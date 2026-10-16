import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { AccountCharacterSelect, type AccountCharacterOption } from "@/components/account-character-select";
import { Button } from "@/components/ui/button";
import { FormDialog } from "@/components/ui/form-dialog";
import { getLocale, msg } from "@/lib/i18n";
import * as api from "./api";

export function CollateralPicker({ csrf, item, characters, close, saved }: { csrf: string; item: api.Case; characters: AccountCharacterOption[]; close: () => void; saved: () => void }) {
  const [character, setCharacter] = useState(characters.find(c => c.id === item.borrower_character_id)?.id || characters[0]?.id || "");
  const [selected, setSelected] = useState<Record<string, string>>({});
  const [search, setSearch] = useState("");
  const [limit, setLimit] = useState(40);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const assets = useQuery({ queryKey: ["loan", "collateral-assets", item.id, character], queryFn: ({ signal }) => api.collateralAssets(item.id, character, signal), enabled: !!character, staleTime: 30_000, refetchOnWindowFocus: false });
  const choices = assets.data?.items ?? [];
  const picked = choices.filter(a => selected[a.item_id] !== undefined);
  const location = picked[0]?.location_id;
  const filtered = choices.filter(a => `${a.name} ${a.type_id} ${a.location_id}`.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()));
  const valid = picked.length > 0 && picked.length === Object.keys(selected).length && picked.every(a => Number.isSafeInteger(Number(selected[a.item_id])) && Number(selected[a.item_id]) > 0 && Number(selected[a.item_id]) <= Number(a.available) && a.location_id === location);
  return <FormDialog title={msg("选择抵押物品")} size="form" close={close} busy={busy} disabled={!valid || assets.isFetching || assets.isError} submitLabel={msg("保存并查看交付信息")} className="loan-asset-picker" onSubmit={async () => {
    if (!valid || busy) return;
    setBusy(true); setError("");
    try { await api.createCollateralRequest(csrf, item.id, { character_id: character, items: picked.map(a => ({ item_id: a.item_id, quantity: selected[a.item_id] })) }); saved(); }
    catch (e) { setError(e instanceof Error ? e.message : msg("保存抵押物失败")); void assets.refetch(); }
    finally { setBusy(false); }
  }}>
    <p className="loan-dialog-note">{msg("选择同一地点的个人物品，保存后通过游戏合同交付。装配和容器内容一并抵押。")}</p>
    <AccountCharacterSelect label={msg("物品所属角色")} characters={characters} value={character} onChange={value => { setCharacter(value); setSelected({}); setSearch(""); setLimit(40); setError(""); }} autoFocus />
    <label>{msg("搜索物品")}<input type="search" value={search} onChange={e => { setSearch(e.target.value); setLimit(40); }} /></label>
    {assets.isPending ? <p role="status">{msg("正在读取个人资产")}</p> : assets.isError ? <div role="alert"><p>{assets.error.message}</p><Button type="button" variant="outline" onClick={() => void assets.refetch()}>{msg("重试")}</Button></div> : <>
      <p className="loan-asset-meta">{msg("资产更新时间：{0}", assets.data ? new Date(assets.data.observed_at).toLocaleString(getLocale()) : "—")} · {msg("已选 {0} 项", picked.length)}</p>
      <div className="loan-asset-list">
        {filtered.slice(0, limit).map(asset => {
          const checked = selected[asset.item_id] !== undefined;
          const disabled = !checked && (Number(asset.available) <= 0 || asset.items.length > 1 && Number(asset.available) < Number(asset.quantity) || !!location && location !== asset.location_id);
          return <div className={`loan-asset-row${checked ? " is-selected" : ""}`} key={asset.item_id}>
            <label className="loan-asset-choice"><input type="checkbox" checked={checked} disabled={disabled || busy} onChange={e => setSelected(old => { const next = { ...old }; if (e.target.checked) next[asset.item_id] = asset.items.length > 1 ? asset.quantity : "1"; else delete next[asset.item_id]; return next; })} />
              <span><strong>{asset.name || msg("物品 {0}", asset.type_id)}</strong><small>{msg("可选 {0} · 位置 {1}", new Intl.NumberFormat(getLocale()).format(Number(asset.available)), asset.location_id)}</small></span>
            </label>
            {checked && <label className="loan-asset-quantity">{msg("数量")}<input aria-label={msg("{0}的数量", asset.name || asset.type_id)} type="number" min="1" max={asset.available} step="1" value={selected[asset.item_id]} disabled={busy || asset.items.length > 1} onChange={e => setSelected(old => ({ ...old, [asset.item_id]: e.target.value }))} required /></label>}
            {asset.items.length > 1 && <details><summary>{msg("含 {0} 项装配或内容物", asset.items.length - 1)}</summary><ul>{asset.items.filter(i => i.item_id !== asset.item_id).map(i => <li key={i.item_id}>{i.name || msg("物品 {0}", i.type_id)} × {i.quantity}</li>)}</ul></details>}
          </div>;
        })}
      </div>
      {!filtered.length && <p role="status">{search ? msg("没有匹配的物品") : msg("暂无可交付的个人物品，请检查角色资产与授权。")}</p>}
      {filtered.length > limit && <Button type="button" variant="outline" onClick={() => setLimit(limit + 40)}>{msg("显示更多")}</Button>}
    </>}
    {error && <p role="alert">{error}</p>}
  </FormDialog>;
}
