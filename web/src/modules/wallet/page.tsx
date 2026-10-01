import { msg, getLocale } from "@/lib/i18n";
import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useSearchParams } from "react-router-dom";
import {
  Wallet,
  RefreshCw,
  Search,
  SlidersHorizontal,
  ArrowDownLeft,
  ArrowUpRight,
  ChevronLeft,
  ChevronRight,
  List,
  ShoppingCart,
  Eye,
} from "lucide-react";
import { useSession } from "@/modules/identity";
import { EveImage } from "@/components/eve-image";
import { Select } from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Modal } from "@/components/ui/dialog";
import * as api from "./api";
import "./wallet.css";

const date = (v?: string) =>
  v ? new Date(v).toLocaleString(getLocale(), { hour12: false }) : "—";
export default function WalletPage() {
  const s = useSession();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.data) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/login" replace />;
  return (
    <Workspace key={s.data.session.user_id} user={s.data.session.user_id} />
  );
}
function Workspace({ user }: { user: string }) {
  const [params] = useSearchParams();
  const member = params.get("member") || "";
  const [selected, setSelected] = useState("");
  const q = useQuery({
    queryKey: ["wallet", "context", user, member],
    queryFn: ({ signal }) => api.context(member, signal),
  });
  const owners = q.isError ? [] : q.data?.owners || [];
  const owner =
    owners.find((o) => `${o.kind}:${o.id}` === selected) ||
    owners.find((o) => o.id === params.get("owner")) ||
    owners[0];
  return (
    <div className="wallet-page">
      <header className="wallet-heading">
        <span className="wallet-brand">
          <Wallet aria-hidden="true" />
        </span>
        <h1>{msg("钱包")}</h1>
        {owners.length > 0 && (
          <Select
            label={msg("钱包归属")}
            value={owner ? `${owner.kind}:${owner.id}` : ""}
            onValueChange={setSelected}
            options={owners.map((o) => ({
              value: `${o.kind}:${o.id}`,
              label: o.name,
              group: o.kind === "character" ? msg("个人钱包") : msg("军团钱包"),
              leading: (
                <EveImage
                  id={o.id}
                  kind={o.kind}
                  className="wallet-owner-image"
                />
              ),
            }))}
          />
        )}
      </header>
      {q.isError && (
        <p role="alert">
          {q.error.message}{" "}
          <Button variant="outline" onClick={() => void q.refetch()}>
            {msg("重试")}{" "}
          </Button>
        </p>
      )}
      {q.isError ? null : !q.data ? (
        !q.isError && <p role="status">{msg("正在读取")}</p>
      ) : owner ? (
        <Account
          key={`${user}:${member}:${owner.kind}:${owner.id}`}
          owner={owner}
          user={user}
        />
      ) : (
        <div className="wallet-empty">
          <Wallet size={32} aria-hidden="true" />
          <p>{msg("暂无可查看的钱包")}</p>
          <span>{msg("请检查角色授权或军团钱包权限。")}</span>
        </div>
      )}
    </div>
  );
}
function Account({ owner, user }: { owner: api.Owner; user: string }) {
  const client = useQueryClient();
  const [division, setDivision] = useState(owner.divisions[0]);
  const [part, setPart] = useState(owner.journal ? "journal" : "transactions");
  const current = owner.divisions.includes(division)
    ? division
    : owner.divisions[0];
  const active = (part === "journal" ? owner.journal : owner.transactions)
    ? part
    : owner.journal
      ? "journal"
      : "transactions";
  const balance = useQuery({
    queryKey: ["wallet", user, owner.kind, owner.id, current, "balance"],
    queryFn: ({ signal }) => api.records(owner, current, "balance", {}, signal),
    refetchInterval: 60_000,
  });
  const names = useQuery({
    queryKey: ["wallet", user, owner.kind, owner.id, current, "divisions"],
    queryFn: ({ signal }) =>
      api.records(owner, current, "divisions", {}, signal),
    enabled: owner.kind === "corporation",
  });
  useEffect(() => {
    if (!balance.data || !owner.journal || !owner.transactions) return;
    const alternate = active === "journal" ? "transactions" : "journal";
    void client.prefetchQuery({
      queryKey: ["wallet", user, owner.kind, owner.id, current, alternate, {}, ""],
      queryFn: ({ signal }) => api.records(owner, current, alternate, {}, signal),
      staleTime: 30_000,
    });
  }, [active, balance.data, client, current, owner, user]);
  const snapshot = balance.isError ? undefined : balance.data?.items[0];
  return (
    <>
      <section className="wallet-overview" aria-label={msg("钱包余额")}>
        <div className="wallet-balance">
          <span>
            {owner.kind === "character"
              ? msg("个人余额")
              : names.data?.items[0]?.name || msg("分部 {0} 余额", current)}
          </span>
          <strong>
            {api.money(snapshot?.balance)} <small>ISK</small>
          </strong>
          <time dateTime={snapshot?.observed_at}>
            {snapshot
              ? date(snapshot.observed_at)
              : balance.isPending
                ? msg("正在读取")
                : msg("暂无余额记录")}
          </time>
        </div>
        {owner.kind === "corporation" && (
          <Select
            label={msg("钱包分部")}
            value={String(current)}
            onValueChange={(v) => setDivision(Number(v))}
            options={owner.divisions.map((d) => ({
              value: String(d),
              label: msg("分部 {0}", d),
            }))}
          />
        )}
        <IconAction
          label={msg("刷新余额")}
          disabled={balance.isFetching}
          onClick={() => void balance.refetch()}
        >
          <RefreshCw size={18} />
        </IconAction>
        {balance.isError && <p role="alert">{balance.error.message}</p>}
      </section>
      <div className="wallet-tabs" aria-label={msg("记录类型")}>
        {owner.journal && (
          <Button
            variant={active === "journal" ? "default" : "outline"}
            aria-pressed={active === "journal"}
            onClick={() => setPart("journal")}
          >
            <List size={18} />
            {msg("收支流水")}{" "}
          </Button>
        )}
        {owner.transactions && (
          <Button
            variant={active === "transactions" ? "default" : "outline"}
            aria-pressed={active === "transactions"}
            onClick={() => setPart("transactions")}
          >
            <ShoppingCart size={18} />
            {msg("市场交易")}{" "}
          </Button>
        )}
      </div>
      <History
        key={`${current}:${active}`}
        user={user}
        owner={owner}
        division={current}
        part={active}
      />
    </>
  );
}
function History({
  user,
  owner,
  division,
  part,
}: {
  user: string;
  owner: api.Owner;
  division: number;
  part: string;
}) {
  const client = useQueryClient();
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [filters, setFilters] = useState<Record<string, string>>({});
  const [advanced, setAdvanced] = useState(false);
  const [pages, setPages] = useState([""]);
  const [selected, setSelected] = useState<api.Row | null>(null);
  const [error, setError] = useState("");
  const before = pages[pages.length - 1];
  const q = useQuery({
    queryKey: [
      "wallet",
      user,
      owner.kind,
      owner.id,
      division,
      part,
      filters,
      before,
    ],
    queryFn: ({ signal }) =>
      api.records(owner, division, part, { ...filters, before }, signal),
    refetchInterval: 60_000,
  });
  useEffect(() => {
    if (!q.data?.next_cursor) return;
    const next = q.data.next_cursor;
    void client.prefetchQuery({
      queryKey: ["wallet", user, owner.kind, owner.id, division, part, filters, next],
      queryFn: ({ signal }) =>
        api.records(owner, division, part, { ...filters, before: next }, signal),
      staleTime: 30_000,
    });
  }, [client, division, filters, owner, part, q.data?.next_cursor, user]);
  const names = q.data?.names || {};
  const party = (id?: string) => (id && id !== "0" ? names[id] || id : "—");
  const field = (k: string, v: string) => setDraft((d) => ({ ...d, [k]: v }));
  const journal = part === "journal";
  return (
    <section
      className="wallet-history"
      aria-label={journal ? msg("收支流水") : msg("市场交易")}
    >
      <form
        className="wallet-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          const next = { ...draft };
          if (next.from)
            next.from = new Date(`${next.from}T00:00:00`).toISOString();
          if (next.until) {
            const d = new Date(`${next.until}T00:00:00`);
            d.setDate(d.getDate() + 1);
            next.until = d.toISOString();
          }
          if (next.from && next.until && next.until <= next.from) {
            setError(msg("结束日期不能早于开始日期"));
            return;
          }
          setError("");
          setFilters(next);
          setPages([""]);
        }}
      >
        <label className="wallet-search">
          <Search size={18} aria-hidden="true" />
          <input
            aria-label={
              journal ? msg("搜索备注或流水编号") : msg("搜索交易编号")
            }
            placeholder={journal ? msg("备注 / 流水编号") : msg("交易编号")}
            value={draft.search || ""}
            onChange={(e) => field("search", e.target.value)}
            maxLength={200}
          />
        </label>
        <Select
          label={msg("收支方向")}
          value={draft.direction || ""}
          onValueChange={(v) => field("direction", v)}
          options={[
            { value: "", label: msg("全部") },
            ...(journal
              ? [
                  { value: "in", label: msg("收入") },
                  { value: "out", label: msg("支出") },
                ]
              : [
                  { value: "buy", label: msg("买入") },
                  { value: "sell", label: msg("卖出") },
                ]),
          ]}
        />
        <IconAction
          type="button"
          label={msg("更多筛选")}
          aria-expanded={advanced}
          onClick={() => setAdvanced(!advanced)}
        >
          <SlidersHorizontal size={18} />
        </IconAction>
        <IconAction label={msg("查询记录")} type="submit">
          <Search size={18} />
        </IconAction>
        <IconAction
          type="button"
          label={msg("刷新记录")}
          disabled={q.isFetching}
          onClick={() => void q.refetch()}
        >
          <RefreshCw size={18} />
        </IconAction>
        {advanced && (
          <div className="wallet-filters">
            <label>
              {msg("开始日期")}{" "}
              <input
                type="date"
                value={draft.from || ""}
                onChange={(e) => field("from", e.target.value)}
              />
            </label>
            <label>
              {msg("结束日期")}{" "}
              <input
                type="date"
                value={draft.until || ""}
                onChange={(e) => field("until", e.target.value)}
              />
            </label>
            <label>
              {msg("参与方 ID")}{" "}
              <input
                inputMode="numeric"
                pattern="[1-9][0-9]*"
                value={draft.party_id || ""}
                onChange={(e) => field("party_id", e.target.value)}
              />
            </label>
            {journal && (
              <label>
                {msg("流水类型")}{" "}
                <Select
                  label={msg("流水类型")}
                  value={draft.ref_type || ""}
                  onValueChange={(v) => field("ref_type", v)}
                  options={[
                    { value: "", label: msg("全部类型") },
                    ...Object.entries(api.refLabels).map(([value, label]) => ({
                      value,
                      label,
                    })),
                  ]}
                />
              </label>
            )}
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setDraft({});
                setFilters({});
                setPages([""]);
                setError("");
              }}
            >
              {msg("重置")}{" "}
            </Button>
          </div>
        )}
      </form>
      {error && <p role="alert">{error}</p>}
      {q.isError && (
        <p role="alert">
          {q.error.message}{" "}
          <Button variant="outline" onClick={() => void q.refetch()}>
            {msg("重试")}{" "}
          </Button>
        </p>
      )}
      {q.isError ? null : !q.data ? (
        !q.isError && (
          <p className="wallet-empty" role="status">
            {msg("正在读取")}{" "}
          </p>
        )
      ) : q.data.items.length === 0 ? (
        <p className="wallet-empty">{msg("暂无符合条件的记录")}</p>
      ) : (
        <div
          className="wallet-table-scroll"
          tabIndex={0}
          role="region"
          aria-label={msg("钱包记录表格")}
        >
          <table>
            <thead>
              <tr>
                <th>{msg("时间")}</th>
                <th>{journal ? msg("类型 / 备注") : msg("物品")}</th>
                <th>{journal ? msg("参与方") : msg("交易对象")}</th>
                {!journal && <th className="wallet-number">{msg("数量")}</th>}
                <th className="wallet-number">
                  {journal ? msg("收支（ISK）") : msg("单价（ISK）")}
                </th>
                <th className="wallet-number">
                  {journal ? msg("余额（ISK）") : msg("方向")}
                </th>
                <th>{msg("详情")}</th>
              </tr>
            </thead>
            <tbody>
              {q.data.items.map((row) => (
                <tr key={`${row.id}:${row.entry_key || ""}`}>
                  <td>
                    <time dateTime={row.date}>{date(row.date)}</time>
                  </td>
                  <td>
                    {journal ? (
                      <>
                        <strong className="wallet-journal-type">
                          {api.refLabel(row.ref_type)}
                        </strong>
                        {row.reason && (
                          <span className="wallet-note">{row.reason}</span>
                        )}
                      </>
                    ) : (
                      <div className="wallet-item">
                        <EveImage
                          id={row.type_id || ""}
                          kind="type"
                          className="wallet-type-image"
                        />
                        <strong>{row.type_name || row.type_id}</strong>
                      </div>
                    )}
                  </td>
                  <td>
                    {journal ? (
                      <>
                        <span>{party(row.first_party_id)}</span>
                        <span className="wallet-note">
                          {party(row.second_party_id)}
                        </span>
                      </>
                    ) : (
                      party(row.client_id)
                    )}
                  </td>
                  {!journal && (
                    <td className="wallet-number">{row.quantity}</td>
                  )}
                  <td
                    className={`wallet-number ${journal && row.amount ? (row.amount.startsWith("-") ? "wallet-out" : "wallet-in") : ""}`}
                  >
                    {journal &&
                      row.amount != null &&
                      (row.amount.startsWith("-") ? (
                        <ArrowUpRight size={14} aria-hidden="true" />
                      ) : (
                        <ArrowDownLeft size={14} aria-hidden="true" />
                      ))}
                    {api.money(journal ? row.amount : row.unit_price)}
                  </td>
                  <td className="wallet-number">
                    {journal ? (
                      api.money(row.balance)
                    ) : (
                      <span className="wallet-tag">
                        {row.is_buy ? msg("买入") : msg("卖出")}
                      </span>
                    )}
                  </td>
                  <td>
                    <IconAction
                      label={msg("查看记录 {0}", row.id)}
                      onClick={() => setSelected(row)}
                    >
                      <Eye size={18} />
                    </IconAction>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <footer className="wallet-pagination">
        <span>
          {msg(
            "第 {0} 页 · {1} 条",
            pages.length,
            q.data?.items.length ?? 0,
          )}{" "}
        </span>
        <IconAction
          label={msg("上一页")}
          disabled={pages.length === 1 || q.isFetching}
          onClick={() => setPages((p) => p.slice(0, -1))}
        >
          <ChevronLeft size={18} />
        </IconAction>
        <IconAction
          label={msg("下一页")}
          disabled={!q.data?.next_cursor || q.isFetching}
          onClick={() => setPages((p) => [...p, q.data!.next_cursor])}
        >
          <ChevronRight size={18} />
        </IconAction>
      </footer>
      {selected && !q.isError && (
        <Modal
          title={journal ? msg("流水详情") : msg("交易详情")}
          close={() => setSelected(null)}
        >
          <dl className="wallet-detail">
            {(
              [
                [msg("记录编号"), selected.id],
                [msg("时间"), date(selected.date)],
                [
                  msg("类型"),
                  journal
                    ? api.refLabel(selected.ref_type)
                    : selected.is_buy
                      ? msg("买入")
                      : msg("卖出"),
                ],
                [
                  msg("金额（ISK）"),
                  journal
                    ? api.money(selected.amount)
                    : api.money(selected.unit_price),
                ],
                [
                  msg("余额（ISK）"),
                  journal ? api.money(selected.balance) : undefined,
                ],
                [
                  msg("参与方一"),
                  selected.first_party_id
                    ? party(selected.first_party_id)
                    : undefined,
                ],
                [
                  msg("参与方二"),
                  selected.second_party_id
                    ? party(selected.second_party_id)
                    : undefined,
                ],
                [
                  msg("交易对象"),
                  selected.client_id ? party(selected.client_id) : undefined,
                ],
                [msg("物品"), selected.type_name || selected.type_id],
                [msg("数量"), selected.quantity],
                [msg("地点 ID"), selected.location_id],
                [
                  msg("关联流水"),
                  selected.journal_ref_id && selected.journal_ref_id !== "-1"
                    ? selected.journal_ref_id
                    : undefined,
                ],
                [
                  msg("关联对象"),
                  selected.context_id
                    ? `${api.contextLabel(selected.context_id_type)} ${selected.context_id}`
                    : undefined,
                ],
                [
                  msg("税额（ISK）"),
                  selected.tax != null ? api.money(selected.tax) : undefined,
                ],
                [msg("原始类型"), selected.ref_type],
                [msg("原始说明"), selected.description],
                [msg("备注"), selected.reason],
                [msg("采集时间"), date(selected.observed_at)],
              ] as [string, string | undefined][]
            )
              .filter(([, value]) => value !== undefined && value !== "")
              .map(([label, value]) => (
                <div key={label}>
                  <dt>{label}</dt>
                  <dd>{value}</dd>
                </div>
              ))}
          </dl>
        </Modal>
      )}
    </section>
  );
}
