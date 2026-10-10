import { lazy, Suspense, useEffect, useState, type ComponentType } from "react";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useSearchParams } from "react-router-dom";
import {
  ChevronDown,
  ChevronRight,
  ChevronUp,
  ChevronsUpDown,
  Check,
  ClipboardCheck,
  Copy,
  Layers3,
  RefreshCw,
  Search,
} from "lucide-react";
import { useSession } from "@/modules/identity";
import { Button } from "@/components/ui/button";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import { Modal } from "@/components/ui/dialog";
import { msg, getLocale } from "@/lib/i18n";
import * as welfare from "@/modules/welfare/api";
import type { Order } from "@/modules/exchange/rewards-api";
import type { PhysicalContent } from "@/modules/exchange/catalog-api";
import * as loan from "@/modules/loan/api";
import { getContext, getQueue, getItem, lossQueueAmount, type QueueItem } from "./api";
import * as settlement from "@/modules/welfare/settlement-api";
import { useToast } from "@/components/ui/toast-context";
import "./approval.css";

// The queue is used far more often than its detail forms. Fetch the existing
// business views only when an administrator opens an item.
const CaseView = lazy(() =>
  import("@/modules/welfare/page").then((module) => ({
    default: module.CaseView,
  })),
);
const ApprovalOrder = lazy(() =>
  import("@/modules/exchange/approval-order").then((module) => ({
    default: module.ApprovalOrder,
  })),
);
const preloadDetail = (source: string) => {
  if (source === "welfare") void import("@/modules/welfare/page");
  if (source === "exchange") void import("@/modules/exchange/approval-order");
};

const views = [
  { id: "pending", label: msg("待审批") },
  { id: "fulfillment", label: msg("待发放") },
  { id: "exceptions", label: msg("异常") },
  { id: "history", label: msg("已处理") },
];
const kinds = [
  { value: "", label: msg("全部类型") },
  { value: "srp", label: msg("军团补损") },
  { value: "solo", label: msg("PVP补损") },
  { value: "growth", label: msg("成长福利") },
  { value: "activity", label: msg("活动福利") },
  { value: "supercarrier", label: welfare.kinds.supercarrier },
  { value: "titan", label: welfare.kinds.titan },
  { value: "exchange", label: msg("奖励兑换") },
  { value: "loan", label: msg("贷款") },
];
const progress: Record<string, string> = {
  waiting_contract: msg("等待合同同步"),
  waiting_items: msg("等待合同明细"),
  awaiting_acceptance: msg("等待领取合同"),
  multiple_contracts: msg("存在多个交付合同"),
  mismatch: msg("合同内容不符"),
  issuer_unverified: msg("发放人未通过核验"),
  contract_claimed: msg("合同已用于其他发放"),
  contract_unavailable: msg("合同证据暂不可用"),
  evidence_unavailable: msg("合同证据暂不可用"),
  snapshot_required: msg("奖励快照不完整"),
  coins_review_required: msg("待确认发币"),
  coins_credited: msg("已发放"),
  finished: msg("合同已核对"),
  fulfilled: msg("合同已核对"),
};
const settlementStates: Record<string, string> = {
  pending: msg("批次待处理"),
  processing: msg("批次处理中"),
  completed: msg("批次已完成"),
  partial: msg("批次部分完成"),
  failed: msg("批次失败"),
};
const settlementStatusLabel = (batch: settlement.SettlementBatch) =>
  batch.delivery_status === "awaiting_acceptance"
    ? progress.awaiting_acceptance
    : settlementStates[batch.state] || batch.state;
const settlementIsProcessed = (batch: settlement.SettlementBatch) =>
  batch.state === "completed" || batch.delivery_status === "awaiting_acceptance";
const actionLabels: Record<string, string> = {
  approve: msg("已批准"),
  reject: msg("已驳回"),
  information: msg("要求补充"),
  approve_cancel: msg("已同意取消"),
  reject_cancel: msg("已驳回取消"),
  complete: msg("已完成"),
  release_coins: msg("已发放"),
  cancel: msg("已撤回"),
  void: msg("撤销批准"),
  delivery_check: msg("系统核验"),
  delivery: msg("系统核验"),
  pending: msg("已驳回取消"),
  cancelled: msg("已同意取消"),
};
const typeLabel = (v: QueueItem) =>
  v.source === "loan"
    ? msg("贷款")
    : v.source === "exchange"
    ? msg("奖励兑换")
    : v.kind.startsWith("growth_")
      ? msg("成长福利")
      : v.kind.startsWith("activity_")
        ? msg("活动福利")
      : welfare.kinds[v.kind] || v.kind;
const statusLabel = (v: QueueItem) =>
  v.source === "loan"
    ? ({ submitted: msg("待审核"), approved: msg("待放款"), rejected: msg("已驳回"), active: msg("还款中"), settled: msg("已结清"), defaulted: msg("已逾期") }[v.state] || v.state)
    : v.state === "cancel_requested"
    ? msg("取消待审核")
    : v.state === "pending" || ["approved", "executing"].includes(v.state)
      ? progress[v.status] || msg("等待合同同步")
      : welfare.states[v.state] ||
        { fulfilled: msg("已发放"), cancelled: msg("已取消") }[v.state] ||
        v.state;
const date = (v: string) =>
  new Date(v).toLocaleString(getLocale(), {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
const queueForView = (params: URLSearchParams, view: string) => {
  const next = new URLSearchParams(params);
  for (const key of ["cursor", "source", "id", "status", "mine"])
    next.delete(key);
  next.set("view", view);
  return next;
};
export default function ApprovalPage() {
  const s = useSession();
  if (s.isError) return <p role="alert">{s.error.message}</p>;
  if (!s.data) return <p role="status">{msg("正在读取")}</p>;
  if (!s.data.session) return <Navigate to="/" replace />;
  return (
    <Workspace user={s.data.session.user_id} csrf={s.data.session.csrf_token} />
  );
}
function Workspace({ user, csrf }: { user: string; csrf: string }) {
  const [params, setParams] = useSearchParams();
  const client = useQueryClient();
  const view = params.get("view") || "pending";
  const tab = view === "information" ? "pending" : view;
  const sort =
    ["time_desc", "time_asc", "id_desc", "id_asc"].includes(
      params.get("sort") || "",
    )
      ? params.get("sort")!
        : tab === "history"
          ? "time_desc"
          : "time_asc";
  const [search, setSearch] = useState(params.get("q") || "");
  const [moreOpen, setMoreOpen] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [settling, setSettling] = useState(false);
  const toast = useToast();
  const context = useQuery({
    queryKey: ["approval", "context", user],
    queryFn: ({ signal }) => getContext(false, signal),
    refetchInterval: 60000,
    staleTime: 30000,
    refetchOnWindowFocus: false,
    placeholderData: keepPreviousData,
  });
  const people = useQuery({
    queryKey: ["approval", "context", user, "people"],
    queryFn: ({ signal }) => getContext(true, signal),
    enabled: moreOpen && context.data?.allowed === true,
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
  });
  const query = new URLSearchParams(params);
  query.delete("source");
  query.delete("id");
  if (!query.has("view")) query.set("view", "pending");
  const q = useQuery({
    queryKey: ["approval", "queue", user, query.toString()],
    queryFn: ({ signal }) => getQueue(query, signal),
    enabled: context.data?.allowed === true,
    refetchInterval: 30000,
    staleTime: 15000,
    refetchOnWindowFocus: false,
  });
  const batches = useQuery({
    queryKey: ["welfare", "settlements", user],
    queryFn: ({ signal }) => settlement.list(signal),
    enabled: context.data?.allowed === true && (tab === "fulfillment" || tab === "history"),
    refetchInterval: tab === "fulfillment" || tab === "history" ? 30000 : false,
    staleTime: 15000,
    refetchOnWindowFocus: false,
  });
  // The cursor is bound to the current filters and user. Start the next page
  // while this one is visible, before warming unrelated views.
  useEffect(() => {
    if (
      context.data?.allowed !== true || q.isPlaceholderData || !q.data?.next_cursor ||
      q.data.unavailable.length > 0
    ) return;
    const next = new URLSearchParams(params);
    next.delete("cursor");
    next.delete("source");
    next.delete("id");
    next.set("cursor", q.data.next_cursor);
    if (!next.has("view")) next.set("view", "pending");
    void client.prefetchQuery({
      queryKey: ["approval", "queue", user, next.toString()],
      queryFn: ({ signal }) => getQueue(next, signal),
      staleTime: 30_000,
    });
  }, [client, context.data?.allowed, params, q.data, q.isPlaceholderData, user]);
  const warmView = (target: string) => {
    if (context.data?.allowed !== true || target === tab) return;
    const next = queueForView(params, target);
    void client.prefetchQuery({
      queryKey: ["approval", "queue", user, next.toString()],
      queryFn: ({ signal }) => getQueue(next, signal),
      staleTime: 30_000,
    });
  };
  const change = (key: string, value: string) => {
    setSelected(new Set());
    const p = new URLSearchParams(params);
    p.delete("cursor");
    p.delete("source");
    p.delete("id");
    if (value) p.set(key, value);
    else p.delete(key);
    if (key === "view") {
      p.delete("status");
      p.delete("mine");
    }
    setParams(p);
  };
  const close = () => {
    const p = new URLSearchParams(params);
    p.delete("source");
    p.delete("id");
    p.delete("batch");
    setParams(p, { replace: true });
  };
  const done = () => {
    close();
    for (const key of ["approval", "welfare", "exchange", "loan"])
      void client.invalidateQueries({ queryKey: [key] });
  };
  const visibleBatches =
    tab === "fulfillment"
      ? (batches.data?.items ?? []).filter((batch) => !settlementIsProcessed(batch))
      : tab === "history"
        ? (batches.data?.items ?? []).filter(settlementIsProcessed)
        : [];
  // Keep source records out of ordinary fulfillment rows for the whole batch
  // lifetime, even after an awaiting-acceptance batch moves to Processed.
  const batchedSourceBatches =
    tab === "fulfillment"
      ? (batches.data?.items ?? []).filter((batch) => batch.state !== "completed")
      : visibleBatches;
  const batchedKeys = new Set(
    batchedSourceBatches.flatMap((batch) =>
      (batch.entries ?? []).map((entry) => `${entry.source}:${entry.source_id}`),
    ),
  );
  const selectable =
    tab === "fulfillment"
      ? (q.data?.items ?? [])
          .filter((item) => item.source !== "loan" && !batchedKeys.has(`${item.source}:${item.id}`))
          .filter((item): item is QueueItem & { source: "welfare" | "exchange" } => item.source !== "loan")
      : [];
  const selectedItems = selectable.filter((v) => selected.has(`${v.source}:${v.id}`));
  const selectedAccount = selectedItems[0]?.account_id || "";
  const batchItems = selectedAccount
    ? selectable.filter((v) => v.account_id === selectedAccount)
    : selectable.length > 0
      ? selectable.filter((v) => v.account_id === selectable[0].account_id)
      : [];
  const allSelected = batchItems.length > 0 && batchItems.every((v) => selected.has(`${v.source}:${v.id}`));
  const submitSettlement = async () => {
    if (!selectedItems.length || settling) return;
    setSettling(true);
    try {
      await settlement.create(
        csrf,
        selectedItems.map((item) => ({ source: item.source, id: item.id })),
      );
      setSelected(new Set());
      toast.success(msg("已创建批次，请按合同编号合并发放"));
      void client.invalidateQueries({ queryKey: ["approval", "queue", user] });
      void client.invalidateQueries({ queryKey: ["welfare", "settlements", user] });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : msg("批量结算失败，请重试"));
    } finally {
      setSettling(false);
    }
  };
  const unavailable = [
    ...new Set([
      ...(context.data?.unavailable || []),
      ...(q.data?.unavailable || []),
    ]),
  ];
  const staleSources = Object.entries(q.data?.source_status || {})
    .filter(([, status]) => status === "stale")
    .map(([source]) => source);
  if (context.isError)
    return (
      <p role="alert">
        {context.error.message}
        <Button onClick={() => void context.refetch()}>{msg("重试")}</Button>
      </p>
    );
  if (!context.data) return <p role="status">{msg("正在读取")}</p>;
  if (!context.data.allowed)
    return (
      <p role="alert">
        {unavailable.length
          ? msg("审批中心暂不可用，请重试")
          : msg("无权访问审批中心")}
        <Button onClick={() => void context.refetch()}>{msg("重试")}</Button>
      </p>
    );
  return (
    <div className="approval-page">
      <header className="approval-heading">
        <span className="approval-mark">
          <ClipboardCheck size={22} />
        </span>
        <h1>{msg("审批中心")}</h1>
        <IconAction
          label={msg("刷新")}
          onClick={() => {
            void client.invalidateQueries({ queryKey: ["approval", "queue", user] });
            void context.refetch();
          }}
        >
          <RefreshCw size={17} />
        </IconAction>
      </header>
      <nav className="approval-tabs" aria-label={msg("审批视图")}>
        {views.map((v) => (
          <button
            key={v.id}
            type="button"
            className={tab === v.id ? "active" : ""}
            aria-current={tab === v.id ? "page" : undefined}
    onFocus={() => warmView(v.id)}
            onClick={() => change("view", v.id)}
          >
            {v.label}
            <span>
              {unavailable.length ? "—" : (q.data?.counts[v.id] ?? "—")}
            </span>
          </button>
        ))}
      </nav>
      <form
        className="approval-filters"
        onSubmit={(e) => {
          e.preventDefault();
          change("q", search.trim());
        }}
      >
        <div className="approval-search">
          <Search size={16} aria-hidden="true" />
          <input
            aria-label={msg("搜索编号或角色")}
            placeholder={msg("搜索编号或角色")}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <Button variant="ghost" type="submit">
            {msg("搜索")}
          </Button>
        </div>
        <Select
          label={msg("业务类型")}
          value={params.get("kind") || ""}
          onValueChange={(v) => change("kind", v)}
          options={kinds}
        />
        <Select
          label={msg("军团")}
          value={params.get("corporation") || ""}
          onValueChange={(v) => change("corporation", v)}
          options={[
            { value: "", label: msg("全部军团") },
            ...context.data.corporations.map((c) => ({
              value: c.id,
              label: c.name,
            })),
          ]}
        />
        {tab === "pending" && (
          <Select
            label={msg("待办类型")}
            value={
              view === "information"
                ? "information"
                : params.get("status") || ""
            }
            onValueChange={(v) => {
              const p = new URLSearchParams(params);
              p.delete("cursor");
              p.set("view", v === "information" ? "information" : "pending");
              if (v && v !== "information") p.set("status", v);
              else p.delete("status");
              setParams(p);
            }}
            options={[
              { value: "", label: msg("全部待审批") },
              { value: "submitted", label: msg("申请") },
              { value: "cancel_requested", label: msg("申请取消") },
              { value: "information", label: msg("待补充") },
            ]}
          />
        )}
        {tab === "history" && (
          <label className="approval-checkbox">
            <input
              type="checkbox"
              checked={params.get("mine") === "true"}
              onChange={(e) => change("mine", e.target.checked ? "true" : "")}
            />
            {msg("我处理的")}
          </label>
        )}
        {tab !== "pending" && (
          <Select
            label={msg("状态")}
            value={params.get("status") || ""}
            onValueChange={(v) => change("status", v)}
            options={[
              { value: "", label: msg("全部状态") },
              ...(tab === "history"
                ? [
                    { value: "approved", label: msg("已批准") },
                    { value: "completed", label: msg("已完成") },
                    { value: "fulfilled", label: msg("已发放") },
                    { value: "rejected", label: msg("已驳回") },
                    { value: "cancelled", label: msg("已取消") },
                  ]
                : Object.entries(progress)
                    .filter(([key]) =>
                      tab === "fulfillment"
                        ? [
                            "waiting_contract",
                            "waiting_items",
                            "awaiting_acceptance",
                            "coins_review_required",
                          ].includes(key)
                        : [
                            "mismatch",
                            "multiple_contracts",
                            "issuer_unverified",
                            "contract_claimed",
                            "contract_unavailable",
                            "evidence_unavailable",
                            "snapshot_required",
                          ].includes(key),
                    )
                    .map(([value, label]) => ({ value, label }))),
            ]}
          />
        )}
        <details className="approval-more" open={moreOpen} onToggle={(event) => setMoreOpen(event.currentTarget.open)}>
          <summary>{msg("更多筛选")}</summary>
          <div>
            <label>
              {msg("起始日期（UTC）")}
              <input
                type="date"
                value={params.get("from")?.slice(0, 10) || ""}
                onChange={(e) =>
                  change(
                    "from",
                    e.target.value ? `${e.target.value}T00:00:00Z` : "",
                  )
                }
              />
            </label>
            <label>
              {msg("截止日期（UTC，不含）")}
              <input
                type="date"
                value={params.get("until")?.slice(0, 10) || ""}
                onChange={(e) =>
                  change(
                    "until",
                    e.target.value ? `${e.target.value}T00:00:00Z` : "",
                  )
                }
              />
            </label>
            <Select
              label={msg("申请人")}
              value={params.get("account") || ""}
              onValueChange={(v) => change("account", v)}
              options={[
                { value: "", label: msg("全部人员") },
                ...((people.data?.people ?? []).map((p) => ({
                  value: p.id,
                  label: p.name,
                }))),
              ]}
            />
            {people.isFetching && <span className="approval-filter-hint" role="status">{msg("正在读取申请人")}</span>}
            {people.isError && <span className="approval-filter-hint approval-filter-error" role="alert">{msg("申请人筛选暂不可用")}</span>}
            <Button
              variant="outline"
              type="button"
              onClick={() => {
                setSearch("");
                setSelected(new Set());
                setParams({ view });
              }}
            >
              {msg("重置筛选")}
            </Button>
          </div>
        </details>
      </form>
      {unavailable.length > 0 && (
        <p role="alert">
          {msg(
            "部分来源不可用，计数不完整：{0}",
            unavailable
              .map((v) =>
                v === "welfare"
                  ? msg("军团福利")
                  : v === "exchange"
                    ? msg("奖励兑换")
                    : v === "loan"
                      ? msg("贷款")
                      : v,
              )
              .join("、"),
          )}
          <Button
            variant="outline"
            onClick={() => {
              void q.refetch();
              void context.refetch();
            }}
          >
            {msg("重试")}
          </Button>
        </p>
      )}
      {staleSources.length > 0 && unavailable.length === 0 && (
        <p role="status" className="approval-sync-status">
          {msg("部分来源正在同步，列表可能暂时落后：{0}", staleSources.join("、"))}
          <Button
            variant="outline"
            onClick={() => void q.refetch()}
          >
            {msg("刷新")}
          </Button>
        </p>
      )}
      {q.isError ? (
        <p role="alert">
          {q.error.message}
          <Button onClick={() => void q.refetch()}>{msg("重试")}</Button>
        </p>
      ) : !q.data ? (
        <p role="status">{msg("正在读取")}</p>
      ) : (
        <>
          {tab === "fulfillment" && (
            <>
              <div className="approval-batch-toolbar">
                <label className="approval-checkbox">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={(e) =>
                      setSelected(
                        e.target.checked
                          ? new Set(batchItems.map((v) => `${v.source}:${v.id}`))
                          : new Set(),
                      )
                    }
                    disabled={!batchItems.length || settling}
                  />
                  {msg("全选同组")}
                </label>
                <span className="approval-batch-count">{msg("已选 {0} 项", String(selectedItems.length))}</span>
                <Button disabled={!selectedItems.length || settling} onClick={() => void submitSettlement()}>
                  <Layers3 size={16} aria-hidden="true" />
                  {settling ? msg("提交中") : msg("批量结算")}
                </Button>
              </div>
            </>
          )}
          <div className="approval-table-wrap">
          <table className="approval-table">
            <thead>
              <tr>
                {tab === "fulfillment" && <th className="approval-select-cell"><span className="sr-only">{msg("选择")}</span></th>}
                <th
                  aria-sort={
                    sort.startsWith("id_")
                      ? sort === "id_desc"
                        ? "descending"
                        : "ascending"
                      : "none"
                  }
                >
                  <button
                    type="button"
                    className="approval-sort-button"
                    aria-label={msg("切换排序")}
                    onClick={() =>
                      change(
                        "sort",
                        sort.startsWith("id_")
                          ? sort === "id_desc"
                            ? "id_asc"
                            : "id_desc"
                          : "id_asc",
                      )
                    }
                  >
                    {msg("类型与编号")}
                    {sort.startsWith("id_") ? (
                      sort === "id_desc" ? (
                        <ChevronDown size={14} aria-hidden="true" />
                      ) : (
                        <ChevronUp size={14} aria-hidden="true" />
                      )
                    ) : (
                      <ChevronsUpDown size={14} aria-hidden="true" />
                    )}
                  </button>
                </th>
                <th>{msg("申请人")}</th>
                <th>{msg("项目或舰船")}</th>
                <th>{msg("金额与物品")}</th>
                <th>{msg("状态")}</th>
                <th
                  aria-sort={
                    sort.startsWith("time_")
                      ? sort === "time_desc"
                        ? "descending"
                        : "ascending"
                      : "none"
                  }
                >
                  <button
                    type="button"
                    className="approval-sort-button"
                    aria-label={msg("切换排序")}
                    onClick={() =>
                      change(
                        "sort",
                        sort.startsWith("time_")
                          ? sort === "time_desc"
                            ? "time_asc"
                            : "time_desc"
                          : "time_asc",
                      )
                    }
                  >
                    {tab === "history" ? msg("处理时间") : msg("申请时间")}
                    {sort.startsWith("time_") ? (
                      sort === "time_desc" ? (
                        <ChevronDown size={14} aria-hidden="true" />
                      ) : (
                        <ChevronUp size={14} aria-hidden="true" />
                      )
                    ) : (
                      <ChevronsUpDown size={14} aria-hidden="true" />
                    )}
                  </button>
                </th>
                <th>
                  <span className="sr-only">{msg("查看")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {visibleBatches.map((batch) => (
                <SettlementTableRow
                  key={`batch:${batch.id}`}
                  batch={batch}
                  selectable={tab === "fulfillment"}
                  mainAccount={
                    (q.data?.items ?? [])
                      .filter((item) =>
                        (batch.entries ?? []).some(
                          (entry) =>
                            entry.source === item.source && entry.source_id === item.id,
                        ),
                      )
                      .map((item) => item.applicant || item.recipient)
                      .find(Boolean) || "—"
                  }
                  onOpen={() => {
                    const p = new URLSearchParams(params);
                    p.delete("source");
                    p.delete("id");
                    p.set("batch", batch.id);
                    setParams(p);
                  }}
                />
              ))}
              {(tab === "fulfillment" ? selectable : q.data.items).map((v) => (
                <tr key={`${v.source}:${v.id}`}>
                  {tab === "fulfillment" && (
                    <td className="approval-select-cell">
                      <input
                        type="checkbox"
                        checked={selected.has(`${v.source}:${v.id}`)}
                        onChange={(e) => {
                          const key = `${v.source}:${v.id}`;
                          setSelected((current) => {
                            const next = new Set(current);
                            if (e.target.checked) next.add(key);
                            else next.delete(key);
                            return next;
                          });
                        }}
                        aria-label={msg("选择 {0} #{1}", typeLabel(v), v.id)}
                        disabled={settling || (!!selectedAccount && v.account_id !== selectedAccount)}
                      />
                    </td>
                  )}
                  <td>
                    <strong>{typeLabel(v)}</strong>
                    <small>#{v.id}</small>
                  </td>
                  <td>
                    <strong>{v.applicant || v.recipient || "—"}</strong>
                    {v.applicant && v.applicant !== v.recipient && (
                      <small>{v.recipient}</small>
                    )}
                  </td>
                  <td className="approval-title">{v.title || typeLabel(v)}</td>
                  <td>
                    <ApprovalValue item={v} />
                  </td>
                  <td>
                    <span
                      className={`approval-status ${tab === "exceptions" ? "exception" : ""}`}
                    >
                      {statusLabel(v)}
                    </span>
                    {tab === "history" && v.action && (
                      <small>{actionLabels[v.action] || msg("已处理")}</small>
                    )}
                  </td>
                  <td>
                    <time dateTime={v.time}>{date(v.time)}</time>
                  </td>
                  <td>
                    <IconAction
                      label={msg("查看 {0} #{1}", typeLabel(v), v.id)}
                      onPointerEnter={() => preloadDetail(v.source)}
                      onFocus={() => preloadDetail(v.source)}
                      onClick={() => {
                        preloadDetail(v.source);
                        const p = new URLSearchParams(params);
                        p.set("source", v.source);
                        p.set("id", v.id);
                        setParams(p);
                      }}
                    >
                      <ChevronRight size={18} />
                    </IconAction>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {!(tab === "fulfillment" ? selectable.length : q.data.items.length) && !visibleBatches.length && (
            <div className="approval-empty">
              <ClipboardCheck size={28} />
              <p>{msg("暂无符合条件的记录")}</p>
            </div>
          )}
          </div>
        </>
      )}
      {(params.has("cursor") || q.data?.next_cursor) && (
        <div className="approval-pagination">
          <Button
            variant="outline"
            disabled={!params.has("cursor")}
            onClick={() => change("cursor", "")}
          >
            {msg("返回首页")}
          </Button>
          <Button
            variant="outline"
            disabled={!q.data?.next_cursor || unavailable.length > 0}
            onClick={() => change("cursor", q.data!.next_cursor)}
          >
            {msg("下一页")}
          </Button>
        </div>
      )}
      {params.get("source") && params.get("id") && (
        <Suspense
          fallback={
            <p role="status">{msg("正在读取")}</p>
          }
        >
          <Detail
            key={`${params.get("source")}:${params.get("id")}`}
            source={params.get("source")!}
            id={params.get("id")!}
            initialItem={q.data?.items.find((item) => item.source === params.get("source") && item.id === params.get("id"))}
            user={user}
            csrf={csrf}
            close={close}
            done={done}
          />
        </Suspense>
      )}
      {params.get("batch") && (
        <SettlementDetail
          id={params.get("batch")!}
          user={user}
          close={close}
          retry={async (batchID) => {
            try {
              await settlement.retry(batchID, csrf);
              toast.success(msg("已加入重试队列"));
              void batches.refetch();
            } catch (e) {
              toast.error(e instanceof Error ? e.message : msg("重试失败，请重试"));
            }
          }}
        />
      )}
    </div>
  );
}
function Detail({
  source,
  id,
  initialItem,
  user,
  csrf,
  close,
  done,
}: {
  source: string;
  id: string;
  initialItem?: QueueItem;
  user: string;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const q = useQuery({
    queryKey: ["approval", "detail", user, source, id],
    queryFn: ({ signal }) => getItem(source, id, signal),
    placeholderData: initialItem,
    refetchInterval: 30000,
  });
  if (q.isError || !q.data)
    return (
      <Modal title={msg("审批详情")} close={close}>
        {q.isError ? (
          <p role="alert">
            {q.error.message}
            <Button onClick={() => void q.refetch()}>{msg("重试")}</Button>
          </p>
        ) : (
          <p role="status">{msg("正在读取")}</p>
        )}
      </Modal>
    );
  const Renderer = approvalDetailRegistry[q.data.detail_kind || q.data.source] || GenericDetail;
  return <Renderer item={q.data} user={user} csrf={csrf} close={close} done={done} />;
}

type ApprovalDetailProps = {
  item: QueueItem;
  user: string;
  csrf: string;
  close: () => void;
  done: () => void;
};
type ApprovalDetailRenderer = ComponentType<ApprovalDetailProps>;

// The central page resolves a source through this registry. Adding a future
// source only registers its detail renderer; list fetching, pagination and
// error handling remain source agnostic.
const approvalDetailRegistry: Record<string, ApprovalDetailRenderer> = {
  welfare: ({ item, user, csrf, close, done }) => (
    <WelfareDetail
      item={item.payload as welfare.Case}
      user={user}
      csrf={csrf}
      close={close}
      done={done}
    />
  ),
  loan: ({ item, csrf, close, done }) => (
    <LoanDetail item={item} csrf={csrf} close={close} done={done} />
  ),
  exchange: ({ item, user, csrf, close, done }) => (
    <ApprovalOrder item={item} user={user} csrf={csrf} close={close} done={done} />
  ),
};

function GenericDetail({ item, close }: ApprovalDetailProps) {
  return (
    <Modal title={msg("审批详情")} close={close}>
      <div className="approval-detail-grid">
        <div><span>{msg("类型")}</span><strong>{item.detail_kind || item.source}</strong></div>
        <div><span>{msg("编号")}</span><strong>{item.reference || item.id}</strong></div>
        <div><span>{msg("状态")}</span><strong>{item.status || item.state}</strong></div>
        {item.title && <div><span>{msg("项目")}</span><strong>{item.title}</strong></div>}
      </div>
    </Modal>
  );
}

function LoanDetail({
  item,
  csrf,
  close,
  done,
}: {
  item: QueueItem;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const payload = item.payload as unknown as loan.Case;
  const decide = async (state: "approved" | "rejected") => {
    setBusy(true);
    setMessage("");
    try {
      await loan.review(csrf, item.id, { state, version: Number(item.version) });
      done();
    } catch (error) {
      setMessage(error instanceof Error ? error.message : msg("贷款审核失败"));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal title={msg("贷款审批详情")} close={close}>
      <div className="approval-detail-grid">
        <div><span>{msg("编号")}</span><strong>{payload.public_id || item.reference}</strong></div>
        <div><span>{msg("贷款池")}</span><strong>{payload.pool_name || item.title}</strong></div>
        <div><span>{msg("本金")}</span><strong>{formatMinor(payload.principal_minor)}</strong></div>
        <div><span>{msg("总应还")}</span><strong>{formatMinor(payload.total_due_minor)}</strong></div>
        <div><span>{msg("期数")}</span><strong>{payload.installment_count}</strong></div>
        <div><span>{msg("间隔天数")}</span><strong>{payload.interval_days}</strong></div>
      </div>
      {message && <p role="alert">{message}</p>}
      <div className="approval-modal-actions">
        <Button variant="outline" disabled={busy} onClick={() => void decide("rejected")}>{msg("驳回")}</Button>
        <Button disabled={busy} onClick={() => void decide("approved")}>{msg("批准")}</Button>
      </div>
    </Modal>
  );
}
function WelfareDetail({
  item,
  user,
  csrf,
  close,
  done,
}: {
  item: welfare.Case;
  user: string;
  csrf: string;
  close: () => void;
  done: () => void;
}) {
  const q = useQuery({
    queryKey: ["welfare", "context", user, item.corporation_id],
    queryFn: ({ signal }) => welfare.context(item.corporation_id, signal),
    refetchInterval: 30000,
  });
  if (!q.data || q.isError)
    return (
      <Modal title={msg("审批详情")} close={close}>
        {q.isError ? (
          <p role="alert">
            {q.error.message}
            <Button onClick={() => void q.refetch()}>{msg("重试")}</Button>
          </p>
        ) : (
          <p role="status">{msg("正在读取")}</p>
        )}
      </Modal>
    );
  return (
    <CaseView
      item={item}
      initialItem={item}
      user={user}
      csrf={csrf}
      manage={
        !!q.data.corporations.find((c) => c.id === item.corporation_id)
          ?.can_manage
      }
      admin={q.data.administrator}
      personalActions={false}
      approvalEntry
      policy={q.data.policies.find((p) => p.kind === item.kind)}
      close={close}
      done={done}
    />
  );
}

function SettlementTableRow({
  batch,
  selectable,
  mainAccount,
  onOpen,
}: {
  batch: settlement.SettlementBatch;
  selectable: boolean;
  mainAccount: string;
  onOpen: () => void;
}) {
  const amount = batch.isk_minor > 0 ? formatMinor(batch.isk_minor) : "";
  return (
    <tr className="approval-batch-table-row">
      {selectable && (
        <td className="approval-select-cell">
          <span className="approval-batch-glyph" title={msg("批量结算")}>
            <Layers3 size={16} aria-hidden="true" />
          </span>
        </td>
      )}
      <td>
        {!selectable && (
          <span className="approval-batch-glyph" title={msg("批量结算")}>
            <Layers3 size={16} aria-hidden="true" />
          </span>
        )}
        <strong>{msg("批量结算")}</strong>
        <small>#{batch.id}</small>
      </td>
      <td>
        <strong>{mainAccount}</strong>
      </td>
      <td className="approval-title">
        {msg("{0} 条审批记录", String(batch.total_count))}
      </td>
      <td>
        <div className="approval-value">
          {amount && <strong>{amount} ISK</strong>}
          {batch.items.length > 0 && (
            <small className={!amount ? "approval-physical-primary" : undefined}>
              {msg("物品 {0} 类", String(batch.items.length))}
            </small>
          )}
          {!amount && batch.items.length === 0 && "—"}
        </div>
      </td>
      <td>
        <span className="approval-status">
          {settlementStatusLabel(batch)}
        </span>
        <small>
          {batch.completed_count}/{batch.total_count}
          {batch.failed_count ? ` · ${msg("失败 {0} 项", String(batch.failed_count))}` : ""}
        </small>
      </td>
      <td>
        <time dateTime={batch.created_at}>{date(batch.created_at)}</time>
      </td>
      <td>
        <IconAction label={msg("查看批量结算")} onClick={onOpen}>
          <ChevronRight size={18} />
        </IconAction>
      </td>
    </tr>
  );
}

function BatchCopyField({ label, value }: { label: string; value: string }) {
  const [status, setStatus] = useState<"idle" | "copied" | "failed">("idle");
  return (
    <div className="approval-batch-copy-field">
      <label>
        {label}
        <input readOnly value={value} onFocus={(e) => e.currentTarget.select()} />
      </label>
      <Button
        variant="outline"
        aria-label={msg("复制 {0}", label)}
        title={msg("复制 {0}", label)}
        disabled={!value}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value);
            setStatus("copied");
          } catch {
            setStatus("failed");
          }
        }}
      >
        {status === "copied" ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}
      </Button>
      {status === "copied" && <small role="status">{msg("已复制")}</small>}
      {status === "failed" && <small role="alert">{msg("复制失败，请手动复制")}</small>}
    </div>
  );
}

function SettlementDetail({
  id,
  user,
  close,
  retry,
}: {
  id: string;
  user: string;
  close: () => void;
  retry: (id: string) => Promise<void>;
}) {
  const q = useQuery({
    queryKey: ["welfare", "settlement", user, id],
    queryFn: ({ signal }) => settlement.detail(id, signal),
    refetchInterval: 10000,
  });
  if (q.isError || !q.data) {
    return (
      <Modal title={msg("批量结算")} close={close} size="compact">
        {q.isError ? (
          <p role="alert">
            {q.error.message}
            <Button onClick={() => void q.refetch()}>{msg("重试")}</Button>
          </p>
        ) : (
          <p role="status">{msg("正在读取")}</p>
        )}
      </Modal>
    );
  }
  const batch = q.data.batch;
  const amount = batch.isk_minor > 0 ? formatMinor(batch.isk_minor) : "";
  const recipients = batch.recipient_names?.join(", ") || "—";
  return (
    <Modal title={msg("批量结算")} close={close} size="compact">
      <div className="approval-settlement-detail">
        <div className="approval-settlement-heading">
          <strong>{batch.settlement_reference || `#${batch.id}`}</strong>
          <span className="approval-status">{settlementStatusLabel(batch)}</span>
        </div>
        <div className="approval-settlement-copy-grid">
          <BatchCopyField label={msg("合同接收角色")} value={recipients} />
          <BatchCopyField label={msg("支付金额 / ISK")} value={amount} />
          <BatchCopyField label={msg("合同结算 ID")} value={batch.settlement_reference || ""} />
          {batch.contract_id && <BatchCopyField label={msg("游戏合同 ID")} value={batch.contract_id} />}
        </div>
        <div className="approval-settlement-summary">
          <span>{msg("审批记录")} <strong>{batch.completed_count}/{batch.total_count}</strong></span>
          {batch.items.length > 0 && <span>{msg("物品 {0} 类", String(batch.items.length))}</span>}
        </div>
        {batch.items.length > 0 && (
          <details>
            <summary>{msg("物品明细")}</summary>
            <ul>
              {batch.items.map((item) => (
                <li key={`${item.type_id}:${item.quantity}`}>#{item.type_id} × {item.quantity}</li>
              ))}
            </ul>
          </details>
        )}
        {(batch.state === "partial" || batch.state === "failed") && (
          <Button variant="outline" onClick={() => void retry(batch.id)}>{msg("重试")}</Button>
        )}
      </div>
    </Modal>
  );
}

const formatMinor = (value: number) =>
  (value / 100).toLocaleString(getLocale(), { maximumFractionDigits: 2 });

function ApprovalValue({ item }: { item: QueueItem }) {
  const lossAmount = lossQueueAmount(item);
  const order = item.source === "exchange" ? (item.payload as Order) : null;
  const welfarePayload = item.source === "welfare"
    ? item.payload as welfare.Case
    : null;
  const welfareRewards = order || lossAmount
    ? undefined
    : welfarePayload?.detail?.rewards;
  const rewards = order ? order.content : welfareRewards;
  const content: PhysicalContent | undefined =
    rewards ||
    (order && order.type_id !== "0"
      ? {
          fittings: [],
          items: [
            { type_id: order.type_id, name: order.name, quantity: order.quantity },
          ],
        }
      : undefined);
  const cash =
    item.unit.toLowerCase() === "isk"
      ? (lossAmount?.amount ?? item.amount_minor)
      : (content?.isk_minor ?? 0);
  const coins =
    item.unit === "coin"
      ? item.amount_minor
      : (welfareRewards?.coins_minor ?? 0);
  const physical = [
    ...(content?.fittings || []).map(
      (f) => `${f.name || `#${f.fitting_id}`} ×${f.quantity.toLocaleString(getLocale())}`,
    ),
    ...(content?.items || []).map(
      (i) => `${i.name || `#${i.type_id}`} ×${i.quantity.toLocaleString(getLocale())}`,
    ),
  ];
  const summary = physical.slice(0, 2).join(" · ");
  const more = physical.length - 2;
  return (
    <div className="approval-value">
      {cash > 0 && (
        <strong>
          {lossAmount?.label === "quote" ? `${msg("核价金额")} · ` : ""}
          {formatMinor(cash)} ISK
        </strong>
      )}
      {coins > 0 && (
        <span className={cash ? "approval-value-secondary" : "approval-value-primary"}>
          {formatMinor(coins)} {msg("果壳币")}
        </span>
      )}
      {physical.length > 0 && (
        <small className={!cash && !coins ? "approval-physical-primary" : ""}>
          {summary}
          {more > 0 && msg(" · 另有 {0} 项", more)}
        </small>
      )}
      {!cash && !coins && !physical.length &&
        (lossAmount?.label === "pending"
          ? msg(item.kind === "solo" ? "待核价" : "审核时确定")
          : "—")}
    </div>
  );
}
