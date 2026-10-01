import { msg } from "@/lib/i18n";
import {
  contractStatuses,
  contractTypes,
  entityName,
  getContracts,
  title,
  tradeDirections,
  type Contract,
  type Owner,
} from "./contracts-api";

export const exportLimit = 10_000;

// Never use the current UI page's cursor as the beginning of an export.
export async function collectContracts(
  owner: Owner,
  filters: URLSearchParams,
  signal: AbortSignal,
  progress: (count: number) => void = () => {},
) {
  const params = new URLSearchParams();
  for (const key of ["q", "type", "status"]) {
    const value = filters.get(key);
    if (value) params.set(key, value);
  }
  const contracts: Contract[] = [];
  let previous: bigint | undefined;
  for (;;) {
    signal.throwIfAborted();
    const batch = await getContracts(owner, params, signal);
    signal.throwIfAborted();
    for (const contract of batch.items) {
      const id = BigInt(contract.id);
      if (id <= 0n || (previous !== undefined && id >= previous)) {
        throw new Error(msg("合同分页已变化，请重新导出。"));
      }
      previous = id;
      contracts.push(contract);
      if (contracts.length > exportLimit)
        throw new Error(msg("合同超过 10,000 条，请缩小筛选范围。"));
    }
    progress(contracts.length);
    if (!batch.next_cursor) return contracts;
    if (!batch.items.length || batch.next_cursor !== batch.items.at(-1)?.id) {
      throw new Error(msg("合同分页异常，请重新导出。"));
    }
    if (contracts.length === exportLimit)
      throw new Error(msg("合同超过 10,000 条，请缩小筛选范围。"));
    params.set("before", batch.next_cursor);
  }
}

// Quotes alone do not prevent spreadsheet formulas. Prefix untrusted formula-like
// cells with a text marker; preserve numbers as source strings, never JS Number.
export function csvCell(value: string | null | undefined) {
  let text = value ?? "";
  if (/^[\s\uFEFF]*[=+\-@]/u.test(text) || /^[\t\r\n]/.test(text))
    text = "'" + text;
  return '"' + text.replaceAll('"', '""') + '"';
}

export function contractsCSV(owner: Owner, contracts: Contract[]) {
  const rows = [
    [
      msg("范围"),
      msg("所属名称"),
      msg("所属ID"),
      msg("合同ID"),
      msg("标题"),
      msg("原始描述"),
      msg("类型"),
      msg("状态"),
      msg("交易方向"),
      msg("发起人"),
      msg("发起人ID"),
      msg("接收人"),
      msg("接收人ID"),
      msg("指定对象"),
      msg("指定对象ID"),
      msg("实际接受者"),
      msg("实际接受者ID"),
      msg("起点"),
      msg("起点ID"),
      msg("终点"),
      msg("终点ID"),
      msg("价格ISK"),
      msg("报酬ISK"),
      msg("抵押ISK"),
      msg("买断ISK"),
      msg("体积m³"),
      msg("完成期限（天）"),
      msg("发布于（ISO 8601）"),
      msg("到期于（ISO 8601）"),
      msg("接受于（ISO 8601）"),
      msg("完成于（ISO 8601）"),
      msg("最近同步（ISO 8601）"),
    ],
  ];
  for (const c of contracts) {
    const receiver = c.acceptor.id !== "0" ? c.acceptor : c.assignee;
    rows.push([
      owner.kind === "character" ? msg("个人合同") : msg("军团合同"),
      owner.name,
      owner.id,
      c.id,
      title(c),
      c.title,
      contractTypes[c.type] ?? c.type,
      contractStatuses[c.status] ?? c.status,
      tradeDirections[c.trade_direction ?? "unknown"],
      entityName(c.issuer),
      c.issuer.id,
      entityName(
        receiver,
        c.availability === "public" ? msg("公开") : msg("未指定"),
      ),
      receiver.id,
      entityName(c.assignee),
      c.assignee.id,
      entityName(c.acceptor),
      c.acceptor.id,
      entityName(c.start),
      c.start.id,
      entityName(c.end),
      c.end.id,
      c.price ?? "",
      c.reward ?? "",
      c.collateral ?? "",
      c.buyout ?? "",
      c.volume ?? "",
      c.days_to_complete ?? "",
      c.date_issued,
      c.date_expired,
      c.date_accepted,
      c.date_completed,
      c.checked_at,
    ]);
  }
  return (
    "\uFEFF" +
    rows.map((row) => row.map(csvCell).join(",")).join("\r\n") +
    "\r\n"
  );
}

export function exportFilename(owner: Owner) {
  return `${owner.kind === "character" ? msg("个人合同") : msg("军团合同")}-${owner.id}-${new Date().toISOString().slice(0, 10)}.csv`;
}
