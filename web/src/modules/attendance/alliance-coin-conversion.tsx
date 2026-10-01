import { Coins } from "lucide-react";
import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FormDialog } from "@/components/ui/form-dialog";
import { Button } from "@/components/ui/button";
import { getData } from "@/lib/http";
import { getLocale, msg } from "@/lib/i18n";
import { write } from "./api";
import { formatPAPUnits, isCoinQuote } from "./coin-api";
import { isSaved } from "./pap-api";
import { getAlliancePAPConversions, type AlliancePAPConversionMonth } from "./alliance-pap-api";

export function AllianceCoinConversion({ user, csrf, version }: { user: string; csrf: string; version: string }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [selectedMonth, setSelectedMonth] = useState("");
  const [saved, setSaved] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const attempt = useRef<{ month: string; version: string; request_key: string; token: string; reason: string } | null>(null);
  const client = useQueryClient();
  const months = useQuery({
    queryKey: ["attendance", "alliance-conversions", user, version],
    queryFn: ({ signal }) => getAlliancePAPConversions(signal),
    enabled: open,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const effectiveMonth = selectedMonth || months.data?.months[0]?.month || "";
  const selected = months.data?.months.find((item) => item.month === effectiveMonth);
  const q = useQuery({
    queryKey: ["attendance", "alliance-conversion", user, effectiveMonth, selected?.version],
    queryFn: ({ signal }) => getData(`/api/v1/attendance/alliance-pap/conversion?month=${encodeURIComponent(effectiveMonth)}`, isCoinQuote, signal),
    enabled: open && !!effectiveMonth,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const m = useMutation({
    mutationFn: () => {
      if (!selected || !q.data) throw new Error("conversion unavailable");
      attempt.current ??= {
        month: selected.month,
        version: selected.version,
        request_key: crypto.randomUUID(),
        token: q.data.token,
        reason: reason.trim(),
      };
      return write("/alliance-pap/conversion", csrf, attempt.current, isSaved);
    },
    onSuccess: () => {
      setOpen(false);
      setSaved(true);
      attempt.current = null;
      void client.invalidateQueries({ queryKey: ["attendance"] });
      void client.invalidateQueries({ queryKey: ["exchange"] });
      trigger.current?.focus();
    },
  });
  return (
    <div className="attendance-pap-history">
      <div className="attendance-toolbar">
        <Button ref={trigger} variant="outline" aria-expanded={open} disabled={m.isPending} onClick={() => { setOpen((v) => !v); setReason(""); setSelectedMonth(""); attempt.current = null; setSaved(false); m.reset(); }}>
          <Coins aria-hidden="true" />{msg("兑换联盟 PAP")} {" "}
        </Button>
        {saved && <span role="status">{msg("已兑换")}</span>}
      </div>
      {open && <FormDialog title={msg("兑换联盟 PAP")} close={() => setOpen(false)} busy={m.isPending}
        showSubmit={!!q.data && q.data.pending > 0}
        disabled={months.isError || months.isFetching || q.isError || q.isFetching || !selected || !q.data || q.data.pending <= 0 || !reason.trim()}
        submitLabel={msg("确认兑换")} className="attendance-form attendance-pap-form" onSubmit={() => m.mutate()}
        extraActions={<Button type="button" variant="outline" disabled={m.isPending || q.isFetching || months.isFetching} onClick={() => { attempt.current = null; m.reset(); void months.refetch(); void q.refetch(); }}>{msg("重新核对")}</Button>}
      >
        {months.isError ? <div role="alert">{months.error.message}<Button type="button" variant="outline" onClick={() => void months.refetch()}>{msg("重试")}</Button></div>
          : months.isFetching || !months.data ? <p role="status">{msg("正在核对兑换记录")}</p>
          : months.data.months.length === 0 ? <p role="status">{msg("暂无可兑换的联盟 PAP")}</p>
          : <>
            <label>{msg("兑换月份")}
              <select value={effectiveMonth} disabled={m.isPending} onChange={(e) => { setSelectedMonth(e.target.value); setReason(""); attempt.current = null; m.reset(); }}>
                {months.data.months.map((item: AlliancePAPConversionMonth) => <option key={item.month} value={item.month}>{item.month}</option>)}
              </select>
            </label>
            {q.isError ? <div role="alert">{q.error.message}<Button type="button" variant="outline" onClick={() => void q.refetch()}>{msg("重试")}</Button></div>
              : !q.data || q.isFetching ? <p role="status">{msg("正在核对兑换记录")}</p>
              : <><p className="attendance-note">{msg("已兑换")} {formatPAPUnits(q.data.converted, q.data.unit_scale, getLocale())} {msg("分 · 待兑换")} {formatPAPUnits(q.data.pending, q.data.unit_scale, getLocale())} {msg("分")}</p>
                {q.data.pending === 0 ? <p role="status">{msg("本次积分已全部兑换")}</p> : <><strong>{q.data.characters} {msg("个角色 · 本次发放")} {(q.data.coins_minor / 100).toLocaleString(getLocale())} {msg("果壳币")}</strong>
                  <label>{msg("兑换说明")}<textarea rows={3} autoFocus required maxLength={200} value={reason} disabled={m.isPending} onChange={(e) => { setReason(e.target.value); attempt.current = null; m.reset(); }} /></label>
                  <small>{msg("联盟 PAP 按所选月份快照结算，已兑换部分不重复发币。")}</small>
                  {m.isError && <p role="alert">{m.error.message}</p>}</>}
              </>}
          </>}
      </FormDialog>}
    </div>
  );
}
