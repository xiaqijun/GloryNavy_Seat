import { msg } from "@/lib/i18n";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  ArrowRightLeft,
  Building2,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock3,
  FileText,
  Gavel,
  MapPin,
  Package,
  RefreshCw,
  Search,
  Truck,
  UserRound,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, Navigate, useSearchParams } from "react-router-dom";
import { Card } from "@/components/ui/card";
import { IconAction } from "@/components/ui/icon-action";
import { EveImage } from "@/components/eve-image";
import { useSession } from "@/modules/identity/use-session";
import {
  amount,
  contractDate,
  contractStatuses,
  contractTypes,
  entityName,
  getBids,
  getContractOwners,
  getItems,
  title,
  tradeDirections,
  type Contract,
  type DetailState,
  type Entity,
  type Owner,
} from "./contracts-api";
import "./contracts.css";
import { ContractExportButton } from "./contract-export-button";
import { contractDetailQuery, contractListQuery } from "./contracts-queries";
import { Select } from "@/components/ui/select";

function Feedback({
  error,
  text,
  retry,
}: {
  error?: Error | null;
  text?: string;
  retry?: () => void;
}) {
  return (
    <div className="contract-feedback" role={error ? "alert" : "status"}>
      <FileText aria-hidden="true" size={28} />
      <p>{error?.message ?? text}</p>
      {retry && (
        <IconAction label={msg("重试读取合同")} onClick={retry}>
          <RefreshCw />
        </IconAction>
      )}
    </div>
  );
}
export default function ContractsPage() {
  const session = useSession();
  if (session.isSuccess && !session.data.session)
    return <Navigate to="/login" replace />;
  if (session.isError)
    return (
      <Feedback error={session.error} retry={() => void session.refetch()} />
    );
  if (!session.data?.session) return <Feedback text={msg("正在读取账号")} />;
  return <ContractWorkspace user={session.data.session.user_id} />;
}
function ContractWorkspace({ user }: { user: string }) {
  const [params, setParams] = useSearchParams();
  const member = params.get("member") ?? "";
  const owners = useQuery({
    queryKey: ["eve", "contracts", "owners", user, member],
    queryFn: ({ signal }) => getContractOwners(signal, member),
    refetchInterval: 60_000,
  });
  const kind = params.get("kind") ?? "character";
  const choices = owners.data?.owners.filter((o) => o.kind === kind) ?? [];
  const owner = params.has("owner")
    ? choices.find((o) => o.id === params.get("owner"))
    : choices[0];
  const contract = params.get("contract");
  // A URL-selected object is independently authorized by each API endpoint.
  // Start its read alongside owners, but render only after owners confirms it.
  const requestedID = params.get("owner") ?? "";
  const explicitOwner =
    (kind === "character" || kind === "corporation") &&
    /^[1-9]\d{0,18}$/.test(requestedID);
  const requestedOwner: Owner = {
    kind: kind === "corporation" ? "corporation" : "character",
    id: requestedID,
    name: "",
  };
  useQuery({
    ...contractListQuery(user, requestedOwner, params),
    enabled: owners.isPending && explicitOwner && !contract,
  });
  useQuery({
    ...contractDetailQuery(user, requestedOwner, contract ?? ""),
    enabled:
      owners.isPending &&
      explicitOwner &&
      /^[1-9]\d{0,18}$/.test(contract ?? ""),
  });
  const patch = (values: Record<string, string>) => {
    const next = new URLSearchParams(params);
    for (const [k, v] of Object.entries(values)) {
      if (v) next.set(k, v);
      else next.delete(k);
    }
    setParams(next);
  };
  return (
    <div className="contracts-page">
      <div className="page-heading">
        <div className="contract-page-title">
          {contract && (
            <IconAction
              label={msg("返回合同列表")}
              onClick={() => patch({ contract: "" })}
            >
              <ArrowLeft />
            </IconAction>
          )}
          {!contract && member && (
            <Link
              className="contract-member-back"
              aria-label={msg("返回成员资料")}
              to={`/members?${new URLSearchParams({ member })}`}
            >
              <ArrowLeft size={18} />
            </Link>
          )}
          <h1>{contract ? msg("合同详情") : msg("合同")}</h1>
        </div>
      </div>
      {owners.isPending ? (
        <Feedback text={msg("正在读取合同范围")} />
      ) : owners.isError ? (
        <>
          <Feedback error={owners.error} retry={() => void owners.refetch()} />
          <Link to="/account">{msg("查看我的角色")}</Link>
        </>
      ) : (
        <>
          {!contract && (
            <div className="contract-scope-bar">
              <div
                className="contract-tabs"
                role="group"
                aria-label={msg("合同范围")}
              >
                {(["character", "corporation"] as const).map((k) => (
                  <button
                    key={k}
                    type="button"
                    aria-pressed={kind === k}
                    onClick={() =>
                      patch({ kind: k, owner: "", before: "", trail: "" })
                    }
                  >
                    {k === "character" ? (
                      <UserRound size={17} />
                    ) : (
                      <Building2 size={17} />
                    )}
                    <span>
                      {k === "character" ? msg("个人合同") : msg("军团合同")}
                    </span>
                  </button>
                ))}
              </div>
              {choices.length > 0 && (
                <div className="contract-owner">
                  <Select
                    label={
                      kind === "character" ? msg("查看角色") : msg("查看军团")
                    }
                    value={owner?.id ?? ""}
                    onValueChange={(id) =>
                      patch({ owner: id, before: "", trail: "" })
                    }
                    options={choices.map((o) => ({
                      value: o.id,
                      label: o.name || o.id,
                      leading: (
                        <EveImage
                          id={o.id}
                          kind={o.kind}
                          className="contract-select-avatar"
                        />
                      ),
                    }))}
                  />
                </div>
              )}
            </div>
          )}
          {!owner ? (
            <Card className="py-0">
              <Feedback
                text={
                  contract || params.has("owner")
                    ? msg("该合同范围不可用")
                    : kind === "corporation"
                      ? msg("暂无可查看的军团合同")
                      : msg("暂无可查看的角色")
                }
              />
              {contract && (
                <Link
                  className="contract-empty-link"
                  to={
                    member
                      ? `/members?${new URLSearchParams({ member })}`
                      : "/contracts"
                  }
                >
                  {member ? msg("返回成员资料") : msg("返回个人合同")}
                </Link>
              )}
            </Card>
          ) : contract ? (
            <ContractDetail
              key={`${owner.kind}:${owner.id}:${contract}`}
              owner={owner}
              id={contract}
              user={user}
            />
          ) : (
            <ContractList
              key={`${owner.kind}:${owner.id}`}
              owner={owner}
              user={user}
              params={params}
              patch={patch}
            />
          )}
        </>
      )}
    </div>
  );
}
const TypeIcon = ({ type }: { type: string }) => {
  const Icon =
    type === "courier" ? Truck : type === "auction" ? Gavel : ArrowRightLeft;
  return <Icon aria-hidden="true" size={20} />;
};
function NumberText({ value }: { value: string | null }) {
  return (
    <>
      {amount(value)
        .split(",")
        .map((part, i, parts) => (
          <span key={i}>
            <span className="contract-number-group">
              {part}
              {i < parts.length - 1 ? "," : ""}
            </span>
            {i < parts.length - 1 && <wbr />}
          </span>
        ))}
    </>
  );
}
function Status({ value }: { value: string }) {
  const partiallyFinished =
    value === "finished_issuer" || value === "finished_contractor";
  return (
    <span
      title={value}
      className={`contract-status ${value === "finished" ? "is-done" : value === "failed" ? "is-failed" : value === "in_progress" || partiallyFinished ? "is-active" : ""}`}
    >
      {value === "finished" ? (
        <CheckCircle2 size={14} aria-hidden="true" />
      ) : (
        <Clock3 size={14} aria-hidden="true" />
      )}
      {contractStatuses[value] ?? value}
    </span>
  );
}
function TradeDirection({
  value = "unknown",
}: {
  value: Contract["trade_direction"];
}) {
  return (
    <span
      className={`contract-trade is-${value}`}
      title={msg("以合同发起方为准")}
    >
      {tradeDirections[value]}
    </span>
  );
}
function Person({ entity: e }: { entity: Entity }) {
  return (
    <span className="contract-person">
      {e.id !== "0" && (
        <EveImage
          id={e.id}
          kind={e.category === "corporation" ? "corporation" : "character"}
          className="contract-mini-avatar"
        />
      )}
      <span>{entityName(e)}</span>
    </span>
  );
}
function Recipient({ contract: c }: { contract: Contract }) {
  const recipient = c.acceptor.id !== "0" ? c.acceptor : c.assignee;
  return recipient.id !== "0" ? (
    <Person entity={recipient} />
  ) : (
    <span>{c.availability === "public" ? msg("公开") : msg("未指定")}</span>
  );
}
function Pager({
  index,
  next,
  back,
  forward,
  label = msg("合同"),
}: {
  index: number;
  next: boolean;
  back: () => void;
  forward: () => void;
  label?: string;
}) {
  return (
    <div className="contract-pager">
      <span>
        {msg("第")} {index} {msg("页")}
      </span>
      <IconAction
        label={msg("上一页{0}", label)}
        disabled={index === 1}
        onClick={back}
      >
        <ChevronLeft />
      </IconAction>
      <IconAction
        label={msg("下一页{0}", label)}
        disabled={!next}
        onClick={forward}
      >
        <ChevronRight />
      </IconAction>
    </div>
  );
}
function ContractList({
  owner,
  user,
  params,
  patch,
}: {
  owner: Owner;
  user: string;
  params: URLSearchParams;
  patch: (v: Record<string, string>) => void;
}) {
  const search = params.get("q") ?? "";
  const type = params.get("type") ?? "";
  const status = params.get("status") ?? "";
  const [draft, setDraft] = useState(search);
  const cursors = [
    "",
    ...(params.get("trail") ?? "")
      .split(",")
      .filter((v) => /^[1-9]\d*$/.test(v)),
  ];
  const query = useQuery(contractListQuery(user, owner, params));
  const client = useQueryClient();
  useEffect(() => {
    if (!query.data?.next_cursor) return;
    const next = new URLSearchParams(params);
    next.set("before", query.data.next_cursor);
    void client.prefetchQuery({
      ...contractListQuery(user, owner, next),
      staleTime: 30_000,
    });
  }, [client, owner, params, query.data?.next_cursor, user]);
  const warmDetail = (id: string) => {
    void client.prefetchQuery(contractDetailQuery(user, owner, id));
  };
  const update = (values: Record<string, string>) => {
    patch({ ...values, before: "", trail: "" });
  };
  return (
    <Card className="contract-list-card py-0">
      <div className="contract-toolbar">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            update({ q: draft.trim() });
          }}
        >
          <label className="sr-only" htmlFor="contract-search">
            {msg("搜索合同")}{" "}
          </label>
          <input
            id="contract-search"
            placeholder={msg("描述或合同 ID")}
            value={draft}
            maxLength={100}
            onChange={(e) => setDraft(e.target.value)}
          />
          <IconAction label={msg("搜索合同")} type="submit">
            <Search />
          </IconAction>
        </form>
        <Select
          className="contract-type-select"
          label={msg("合同类型")}
          value={type}
          onValueChange={(value) => update({ type: value })}
          options={[
            { value: "", label: msg("全部类型") },
            ...["item_exchange", "courier", "auction"].map((value) => ({
              value,
              label: contractTypes[value],
            })),
            ...["loan", "unknown"].map((value) => ({
              value,
              label: contractTypes[value],
              group: msg("兼容数据"),
            })),
          ]}
        />
        <Select
          className="contract-status-select"
          label={msg("合同状态")}
          value={status}
          onValueChange={(value) => update({ status: value })}
          options={[
            { value: "", label: msg("全部状态") },
            ...Object.entries(contractStatuses).map(([value, label]) => ({
              value,
              label,
            })),
          ]}
        />
        <IconAction
          label={msg("刷新合同列表")}
          className="contract-refresh"
          disabled={query.isFetching}
          aria-busy={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw />
        </IconAction>
        <ContractExportButton
          key={`${user}:${owner.kind}:${owner.id}:${new URLSearchParams({ q: search, type, status })}`}
          owner={owner}
          filters={new URLSearchParams({ q: search, type, status }).toString()}
          disabled={query.isPending || query.isError}
        />
      </div>
      {query.isPending ? (
        <Feedback text={msg("正在读取合同")} />
      ) : query.isError ? (
        <Feedback error={query.error} retry={() => void query.refetch()} />
      ) : (
        <>
          {query.data.items.length === 0 ? (
            <Feedback
              text={
                search || type || status
                  ? msg("没有匹配的合同")
                  : msg("暂无已同步合同")
              }
            />
          ) : (
            <div
              className="contract-table"
              role="table"
              aria-label={msg("合同列表")}
            >
              <div className="contract-table-head" role="row">
                {[
                  msg("合同"),
                  msg("交易方向"),
                  msg("发起方"),
                  msg("接收人"),
                  msg("金额 · ISK"),
                  msg("到期时间"),
                  "",
                ].map((label, i) => (
                  <span key={i} role="columnheader">
                    {label || <span className="sr-only">{msg("操作")}</span>}
                  </span>
                ))}
              </div>
              {query.data.items.map((c) => (
                <div className="contract-table-row" role="row" key={c.id}>
                  <div role="cell" className="contract-subject">
                    <span className="contract-type-icon">
                      <TypeIcon type={c.type} />
                    </span>
                    <div>
                      <button
                        className="contract-title-link"
                        onPointerEnter={() => warmDetail(c.id)}
                        onFocus={() => warmDetail(c.id)}
                        onClick={() =>
                          patch({
                            kind: owner.kind,
                            owner: owner.id,
                            contract: c.id,
                          })
                        }
                      >
                        {title(c)}
                      </button>
                      <div className="contract-row-meta">
                        <span>#{c.id}</span>
                        <span>{contractTypes[c.type] ?? c.type}</span>
                        <Status value={c.status} />
                      </div>
                    </div>
                  </div>
                  <div role="cell" className="contract-row-trade">
                    <TradeDirection value={c.trade_direction} />
                  </div>
                  <div role="cell" className="contract-issuer">
                    <span className="contract-mobile-label">
                      {msg("发起方")}
                    </span>
                    <Person entity={c.issuer} />
                  </div>
                  <div role="cell" className="contract-recipient">
                    <span className="contract-mobile-label">
                      {msg("接收人")}
                    </span>
                    <Recipient contract={c} />
                  </div>
                  <div role="cell" className="contract-row-amount">
                    <span className="contract-mobile-label">
                      {c.type === "courier" ? msg("报酬") : msg("金额")}
                    </span>
                    <strong>
                      <NumberText
                        value={c.type === "courier" ? c.reward : c.price}
                      />
                    </strong>
                    {c.type === "courier" && <small>{msg("报酬")}</small>}
                  </div>
                  <time
                    role="cell"
                    className="contract-expiry"
                    dateTime={c.date_expired}
                  >
                    {contractDate(c.date_expired)}
                  </time>
                  <div role="cell" className="contract-row-action">
                    <IconAction
                      label={msg("查看合同 {0}", c.id)}
                      onPointerEnter={() => warmDetail(c.id)}
                      onFocus={() => warmDetail(c.id)}
                      onClick={() =>
                        patch({
                          kind: owner.kind,
                          owner: owner.id,
                          contract: c.id,
                        })
                      }
                    >
                      <ArrowRight />
                    </IconAction>
                  </div>
                </div>
              ))}
            </div>
          )}
          <Pager
            index={cursors.length}
            next={!!query.data.next_cursor}
            back={() => {
              const previous = cursors.slice(0, -1);
              patch({
                before: previous.at(-1) ?? "",
                trail: previous.slice(1).join(","),
              });
            }}
            forward={() => {
              patch({
                before: query.data.next_cursor,
                trail: [...cursors.slice(1), query.data.next_cursor].join(","),
              });
            }}
          />
        </>
      )}
    </Card>
  );
}
function ContractDetail({
  owner,
  id,
  user,
}: {
  owner: Owner;
  id: string;
  user: string;
}) {
  const query = useQuery({
    ...contractDetailQuery(user, owner, id),
    refetchInterval: 30_000,
  });
  const client = useQueryClient();
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    heading.current?.focus();
  }, [query.isSuccess]);
  if (query.isPending) return <Feedback text={msg("正在读取合同详情")} />;
  if (query.isError)
    return <Feedback error={query.error} retry={() => void query.refetch()} />;
  const c = query.data.contract;
  const money =
    c.type === "courier"
      ? [
          [msg("报酬"), c.reward],
          [msg("抵押金"), c.collateral],
        ]
      : [
          [c.type === "auction" ? msg("起拍价") : msg("价格"), c.price],
          ...(c.type === "auction" ? [[msg("一口价"), c.buyout]] : []),
        ];
  return (
    <div className="contract-detail">
      <Card className="contract-overview py-0">
        <div className="contract-overview-top">
          <span className="contract-type-icon is-large">
            <TypeIcon type={c.type} />
          </span>
          <div className="contract-overview-title">
            <h2 ref={heading} tabIndex={-1}>
              {title(c)}
            </h2>
            <div className="contract-row-meta">
              <span>#{c.id}</span>
              <span>{contractTypes[c.type] ?? c.type}</span>
              <TradeDirection value={c.trade_direction} />
              <Status value={c.status} />
            </div>
          </div>
          <IconAction
            label={msg("刷新合同详情")}
            disabled={query.isFetching}
            onClick={() =>
              void client.invalidateQueries({ queryKey: ["eve", "contracts"] })
            }
          >
            <RefreshCw />
          </IconAction>
        </div>
        <div className="contract-money-strip">
          {money.map(([label, value]) => (
            <div key={label}>
              <span>{label}</span>
              <strong>
                <NumberText value={value} /> <small>ISK</small>
              </strong>
            </div>
          ))}
          {c.volume !== null && (
            <div>
              <span>{msg("体积")}</span>
              <strong>
                {amount(c.volume)} <small>m³</small>
              </strong>
            </div>
          )}
        </div>
        <dl className="contract-facts">
          <div>
            <dt>{msg("发起方")}</dt>
            <dd>
              <Person entity={c.issuer} />
            </dd>
          </div>
          <div>
            <dt>{msg("指定对象")}</dt>
            <dd>
              {entityName(
                c.assignee,
                c.availability === "public" ? msg("公开") : msg("未指定"),
              )}
            </dd>
          </div>
          <div>
            <dt>{msg("接受方")}</dt>
            <dd>{entityName(c.acceptor, msg("尚未接受"))}</dd>
          </div>
          <div>
            <dt>
              {msg("所属")}
              {owner.kind === "character" ? msg("角色") : msg("军团")}
            </dt>
            <dd>{owner.name}</dd>
          </div>
          <div>
            <dt>{msg("创建时间")}</dt>
            <dd>{contractDate(c.date_issued)}</dd>
          </div>
          <div>
            <dt>{msg("到期时间")}</dt>
            <dd>{contractDate(c.date_expired)}</dd>
          </div>
          {c.date_accepted && (
            <div>
              <dt>{msg("接受时间")}</dt>
              <dd>{contractDate(c.date_accepted)}</dd>
            </div>
          )}
          {c.date_completed && (
            <div>
              <dt>{msg("完成时间")}</dt>
              <dd>{contractDate(c.date_completed)}</dd>
            </div>
          )}
          {c.days_to_complete !== null && (
            <div>
              <dt>{msg("运输时限")}</dt>
              <dd>
                {amount(c.days_to_complete)} {msg("天")}
              </dd>
            </div>
          )}
        </dl>
        {(c.start.id !== "0" || c.end.id !== "0") && (
          <div className="contract-route">
            <MapPin size={18} aria-hidden="true" />
            <div>
              <span>
                {c.type === "courier" ? msg("起点") : msg("交付地点")}
              </span>
              <strong>{entityName(c.start, msg("未提供"))}</strong>
            </div>
            {c.type === "courier" && (
              <>
                <ArrowRight size={18} aria-hidden="true" />
                <div>
                  <span>{msg("终点")}</span>
                  <strong>{entityName(c.end, msg("未提供"))}</strong>
                </div>
              </>
            )}
          </div>
        )}
        <p className="contract-checked">
          {msg("更新于")} {contractDate(c.checked_at)}
        </p>
      </Card>
      {c.type !== "courier" && c.type !== "unknown" && (
        <DetailRows
          owner={owner}
          user={user}
          id={id}
          part="items"
          state={query.data.details.find((d) => d.part === "items")}
        />
      )}
      {c.type === "auction" && (
        <DetailRows
          owner={owner}
          user={user}
          id={id}
          part="bids"
          state={query.data.details.find((d) => d.part === "bids")}
        />
      )}
    </div>
  );
}
function DetailRows({
  owner,
  user,
  id,
  part,
  state,
}: {
  owner: Owner;
  user: string;
  id: string;
  part: "items" | "bids";
  state?: DetailState;
}) {
  const [cursors, setCursors] = useState<string[]>([""]);
  const cursor = cursors.at(-1) ?? "";
  const items = useQuery({
    queryKey: [
      "eve",
      "contracts",
      "items",
      user,
      owner.kind,
      owner.id,
      id,
      cursor,
    ],
    queryFn: ({ signal }) => getItems(owner, id, cursor, signal),
    enabled: part === "items",
    refetchInterval: state?.state !== "ready" ? 30_000 : false,
  });
  const bids = useQuery({
    queryKey: [
      "eve",
      "contracts",
      "bids",
      user,
      owner.kind,
      owner.id,
      id,
      cursor,
    ],
    queryFn: ({ signal }) => getBids(owner, id, cursor, signal),
    enabled: part === "bids",
    refetchInterval: 30_000,
  });
  const q = part === "items" ? items : bids;
  const next = q.data?.next_cursor ?? "";
  const ready = state?.state === "ready";
  const stateLabel = ready
    ? msg("已同步")
    : state?.state === "blocked" || state?.state === "failed"
      ? msg("同步异常")
      : state?.state === "running"
        ? msg("同步中")
        : msg("待同步");
  return (
    <Card className="contract-lines-card py-0">
      <div className="contract-section-heading">
        <h2>
          {part === "items" ? <Package size={19} /> : <Gavel size={19} />}{" "}
          {part === "items" ? msg("物品明细") : msg("拍卖出价")}
        </h2>
        <span className={`contract-status ${ready ? "is-done" : ""}`}>
          {stateLabel}
        </span>
      </div>
      {!ready && (
        <p className="contract-sync-note">
          {stateLabel === msg("同步异常")
            ? msg("明细同步异常，当前内容可能不完整。")
            : msg("明细尚未同步完成，当前内容可能不完整。")}
        </p>
      )}
      {q.isPending ? (
        <Feedback text={msg("正在读取明细")} />
      ) : q.isError ? (
        <Feedback error={q.error} retry={() => void q.refetch()} />
      ) : (
        <>
          {q.data?.items.length === 0 ? (
            <Feedback
              text={
                !ready
                  ? msg("等待明细同步")
                  : part === "items"
                    ? msg("没有物品明细")
                    : msg("暂无出价")
              }
            />
          ) : part === "items" ? (
            <div className="contract-item-list">
              {items.data?.items.map((item) => (
                <div className="contract-item" key={item.id}>
                  <EveImage
                    id={item.type.id}
                    kind="type"
                    className="contract-item-image"
                  />
                  <div className="contract-item-name">
                    <strong>{entityName(item.type)}</strong>
                    <span>
                      {item.raw_quantity === -2
                        ? msg("蓝图复制品")
                        : item.raw_quantity === -1
                          ? msg("蓝图原件")
                          : item.singleton
                            ? msg("单件物品")
                            : ""}
                    </span>
                  </div>
                  <span
                    className={`contract-direction ${item.included ? "" : "is-requested"}`}
                  >
                    {item.included ? msg("提供") : msg("要求")}
                  </span>
                  <strong className="contract-quantity">
                    × {amount(item.quantity)}
                  </strong>
                </div>
              ))}
            </div>
          ) : (
            <div className="contract-bids">
              {bids.data?.items.map((bid) => (
                <div className="contract-bid" key={bid.id}>
                  <Person entity={bid.bidder} />
                  <strong>
                    {amount(bid.amount)} <small>ISK</small>
                  </strong>
                  <time dateTime={bid.date}>{contractDate(bid.date)}</time>
                </div>
              ))}
            </div>
          )}
          <Pager
            label={part === "items" ? msg("物品") : msg("出价")}
            index={cursors.length}
            next={!!next}
            back={() => setCursors((c) => c.slice(0, -1))}
            forward={() => setCursors((c) => [...c, next])}
          />
        </>
      )}
    </Card>
  );
}
