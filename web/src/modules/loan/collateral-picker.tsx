import { useQuery } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { AccountCharacterSelect, type AccountCharacterOption } from "@/components/account-character-select";
import { Button } from "@/components/ui/button";
import { FormDialog } from "@/components/ui/form-dialog";
import { getLocale, msg } from "@/lib/i18n";
import { securityMoney } from "./security-format";
import * as api from "./api";

export function CollateralPicker({ csrf, item, security, characters, close, saved }: { csrf: string; item: api.Case; security: api.SecuritySummary; characters: AccountCharacterOption[]; close: () => void; saved: () => void }) {
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
  const selectionKey = JSON.stringify({ character_id: character, items: picked.map(a => ({ item_id: a.item_id, quantity: selected[a.item_id] })) });
  const [quoteKey, setQuoteKey] = useState("");
  useEffect(() => { const timer = setTimeout(() => setQuoteKey(selectionKey), 250); return () => clearTimeout(timer); }, [selectionKey]);
  const quote = useQuery({ queryKey: ["loan", "collateral-quote", item.id, quoteKey], queryFn: () => api.quoteCollateral(csrf, item.id, JSON.parse(quoteKey)), enabled: valid && quoteKey === selectionKey, retry: false, refetchOnWindowFocus: false, staleTime: 0 });
  const estimate = quoteKey === selectionKey ? quote.data : undefined;
  const canSubmit = valid && estimate?.can_submit && !quote.isFetching && !quote.isError;
  return <FormDialog title={msg("选择抵押物品")} size="form" close={close} busy={busy} disabled={!canSubmit || assets.isFetching || assets.isError} submitLabel={msg("保存并查看交付信息")} className="loan-asset-picker" onSubmit={async () => {
    if (!canSubmit || busy || !estimate) return;
    setBusy(true); setError("");
    try { await api.createCollateralRequest(csrf, item.id, { character_id: character, coverage_minor: estimate.required_minor, items: picked.map(a => ({ item_id: a.item_id, quantity: selected[a.item_id] })) }); saved(); }
    catch (e) { setError(e instanceof Error ? e.message : msg("保存抵押物失败")); void assets.refetch(); void quote.refetch(); }
    finally { setBusy(false); }
  }}>
    <p className="loan-dialog-note">{msg("选择同一地点的个人物品，保存后通过游戏合同交付。装配和容器内容一并抵押。")}</p>
    <AccountCharacterSelect label={msg("物品所属角色")} characters={characters} value={character} onChange={value => { setCharacter(value); setSelected({}); setSearch(""); setLimit(40); setError(""); }} autoFocus />
    <label>{msg("搜索物品")}<input type="search" value={search} onChange={e => { setSearch(e.target.value); setLimit(40); }} /></label>
    <div className="loan-security-quote" aria-live="polite">
      <label>{msg("抵押额度（ISK）")}<input readOnly value={String((estimate?.required_minor ?? security.unallocated_minor) / 100)} /></label>
      <dl className="loan-detail-facts"><div><dt>{msg("所选物品总估值")}</dt><dd>{estimate ? securityMoney(estimate.valuation_minor) : "—"}</dd></div><div><dt>{msg("八折可抵额度")}</dt><dd>{estimate ? securityMoney(estimate.covered_minor) : "—"}</dd></div></dl>
      {valid && (quoteKey !== selectionKey || quote.isFetching) ? <p role="status">{msg("正在估值")}</p> : quote.isError && quoteKey === selectionKey ? <div role="alert"><p>{quote.error.message}</p><Button type="button" variant="outline" onClick={() => void quote.refetch()}>{msg("重新估值")}</Button></div> : estimate && <p role="status">{!estimate.security.credit_ready ? msg("信用评估暂不可用，请等待更新后申请保障。") : estimate.security.legacy_pending ? msg("请先完成或取消已有待交付物品，再新增保障。") : estimate.required_minor === 0 ? msg("无需新增担保或抵押，请查看已有保障记录") : estimate.shortfall_minor > 0 ? msg("还差 {0} 可抵额度，请增加物品或数量。", securityMoney(estimate.shortfall_minor)) : msg("已达到抵押额度，可保存并交付物品。")}</p>}
      {!picked.length && <p className="loan-dialog-note">{msg("所选物品总估值 × 80% 须达到抵押额度。")}</p>}
    </div>
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
