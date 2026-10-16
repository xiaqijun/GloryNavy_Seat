import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { FormDialog } from "@/components/ui/form-dialog";
import { getLocale, msg } from "@/lib/i18n";
import { getContracts, type Contract, type Owner } from "@/modules/eve/contracts-api";
import { CollateralItems } from "./case-detail";
import * as api from "./api";

export function CollateralDelivery({ csrf, userId, request, haircut, close, saved }: { csrf: string; userId: string; request: api.CollateralRequest; haircut: number; close: () => void; saved: () => void }) {
  const [selected, setSelected] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const contracts = useQuery({ queryKey: ["loan", "delivery-contracts", userId, request.id], queryFn: async ({ signal }) => {
    const owner: Owner = { kind: "character", id: request.source_character_id, name: "" };
    const items: Contract[] = [];
    let before = "";
    do {
      const params = new URLSearchParams({ type: "item_exchange", status: "finished" });
      if (before) params.set("before", before);
      const page = await getContracts(owner, params, signal);
      items.push(...page.items.filter(c => !c.for_corporation && c.issuer.id === request.source_character_id && c.acceptor.id === request.recipient_character_id && c.type === "item_exchange" && c.status.startsWith("finished") && c.price !== null && Number(c.price) === 0 && c.reward !== null && Number(c.reward) === 0 && new Date(c.date_completed).getTime() >= Math.floor(new Date(request.created_at).getTime() / 1000) * 1000));
      before = page.next_cursor;
    } while (before);
    return items;
  }, refetchOnWindowFocus: false });
  return <FormDialog title={msg("关联交付合同")} size="form" close={close} busy={busy} disabled={!contracts.data?.some(c => c.id === selected) || contracts.isFetching || contracts.isError} submitLabel={msg("核验并提交抵押")} className="loan-asset-picker" onSubmit={async () => {
    setBusy(true); setError("");
    try { await api.createCollateral(csrf, request.case_id, { request_id: request.id, owner_account_id: userId, contract_kind: "character", contract_owner_id: Number(request.source_character_id), contract_id: Number(selected), valuation_minor: 0, haircut_bps: haircut, covered_minor: 0 }); saved(); }
    catch (e) { setError(e instanceof Error ? e.message : msg("抵押登记失败")); }
    finally { setBusy(false); }
  }}>
    <p className="loan-dialog-note">{msg("选择已完成的交付合同，系统会核对物品与数量并自动估值。")}</p>
    <CollateralItems items={request.items} />
    {contracts.isPending ? <p role="status">{msg("正在读取合同")}</p> : contracts.isError ? <p role="alert">{contracts.error.message}</p> : contracts.data?.length ? <label>{msg("交付合同")}<select value={selected} onChange={e => setSelected(e.target.value)} required autoFocus><option value="" disabled>{msg("请选择已完成合同")}</option>{contracts.data.map(c => <option key={c.id} value={c.id}>#{c.id} · {c.title || new Date(c.date_completed).toLocaleDateString(getLocale())}</option>)}</select></label> : <p role="status">{msg("暂无匹配方向的已完成合同。请完成游戏交付，等待合同同步后刷新。")}</p>}
    <Button type="button" variant="outline" disabled={busy || contracts.isFetching} onClick={() => void contracts.refetch()}>{msg("刷新合同")}</Button>
    {error && <p role="alert">{error}</p>}
  </FormDialog>;
}
