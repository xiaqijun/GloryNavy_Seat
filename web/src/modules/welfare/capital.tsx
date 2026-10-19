import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Rocket } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EveImage } from "@/components/eve-image";
import { msg, getLocale } from "@/lib/i18n";
import {
  capitalCheck,
  capitalAmount,
  capitalMoney,
  type CapitalCheck,
} from "./capital-api";
import { FulfillmentSummary } from "./fulfillment-summary";
import * as api from "./api";
import "./capital.css";

const labels: Record<CapitalCheck["state"], string> = {
  closed: "未开放",
  unbound: "请先绑定本军团角色",
  claimed: "已领取",
  pending: "已有申请",
  contract_required: "可申请",
  contract_invalid: "请核对已完成的个人购舰合同及舰船类型，并确认合同已同步。",
  contract_used: "该购舰合同已用于补贴申请",
  ready: "合同核验通过",
};

export function useCapitalCheck(
  corp: string,
  kind: string,
  input: string,
  version?: string,
) {
  const contract = input.trim();
  const [debounced, setDebounced] = useState(contract);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(contract), 350);
    return () => clearTimeout(timer);
  }, [contract]);
  const valid = !contract || /^[1-9]\d{0,18}$/.test(contract);
  const q = useQuery({
    queryKey: ["welfare", "capital-check", corp, kind, debounced, version],
    queryFn: ({ signal }) => capitalCheck(corp, kind, debounced, signal),
    enabled:
      !!corp &&
      ["supercarrier", "titan"].includes(kind) &&
      valid &&
      debounced === contract,
    retry: false,
    staleTime: 0,
  });
  return {
    ...q,
    valid,
    current: debounced === contract,
    ready:
      valid &&
      debounced === contract &&
      !q.isFetching &&
      !q.isError &&
      q.data?.state === "ready" &&
      q.data.purchase?.contract_id === contract,
  };
}

export function CapitalOverview({
  corp,
  kind,
  policy,
  open,
}: {
  corp: string;
  kind: string;
  policy?: api.Policy;
  open: (v: api.Case) => void;
}) {
  const q = useCapitalCheck(corp, kind, "", policy?.version);
  const [opening, setOpening] = useState(false);
  const [error, setError] = useState("");
  return (
    <section className="welfare-capital-overview" aria-label={msg("领取状态")}>
      <Rocket size={21} aria-hidden="true" />
      <strong>{api.kinds[kind]}</strong>
      <span>
        {msg("补贴比例")}{" "}
        <b>
          {(
            (q.data?.rate_bps ||
              Number(policy?.config.subsidy_rate_bps) ||
              api.capitalRate(kind) * 100) / 100
          ).toLocaleString(getLocale())}
          %
        </b>
      </span>
      <span>{msg("每账号限领一次")}</span>
      <span role="status">
        {q.isError
          ? q.error.message
          : q.data
            ? msg(labels[q.data.state])
            : msg("正在读取")}
      </span>
      {q.isError && (
        <Button variant="outline" onClick={() => void q.refetch()}>
          {msg("重试")}
        </Button>
      )}
      {q.data?.case_id && (
        <Button
          variant="outline"
          disabled={opening}
          onClick={async () => {
            setOpening(true);
            setError("");
            try {
              open((await api.detail(q.data!.case_id!)).item);
            } catch (e) {
              setError(e instanceof Error ? e.message : msg("读取失败"));
            } finally {
              setOpening(false);
            }
          }}
        >
          {msg("查看申请")}
        </Button>
      )}
      {error && <p role="alert">{error}</p>}
    </section>
  );
}

function Amount({ value }: { value: number }) {
  return (
    <span
      title={`${(value / 100).toLocaleString(getLocale(), { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ISK`}
    >
      {capitalMoney(value)} ISK
    </span>
  );
}

export function CapitalPreview({
  check,
  kind,
}: {
  check: ReturnType<typeof useCapitalCheck>;
  kind: string;
}) {
  if (!check.valid)
    return (
      <p className="wide welfare-warning" role="alert">
        {msg("请输入有效的购舰合同 ID")}
      </p>
    );
  if (!check.current || check.isFetching)
    return (
      <p className="wide" role="status">
        {msg("正在核验合同")}
      </p>
    );
  if (check.isError)
    return (
      <div className="wide welfare-capital-error" role="alert">
        {check.error.message}
        <Button variant="outline" onClick={() => void check.refetch()}>
          {msg("重试")}
        </Button>
      </div>
    );
  const result = check.data;
  if (!result) return null;
  if (!result.purchase || result.state !== "ready")
    return (
      <p className="wide welfare-warning" role="status">
        {msg(
          result.state === "contract_required"
            ? "输入购舰合同 ID，核验后提交申请。"
            : labels[result.state],
        )}
      </p>
    );
  const p = result.purchase;
  return (
    <section
      className="wide welfare-capital-preview"
      aria-label={msg("购舰合同核验")}
    >
      <div className="welfare-capital-heading">
        <EveImage kind="type" id={p.ship_type_id} />
        <div>
          <strong>{p.ship_name || api.kinds[kind]}</strong>
          <span>{p.character_name}</span>
        </div>
        <span className="welfare-capital-verified">
          <Check size={16} aria-hidden="true" />
          {msg("合同核验通过")}
        </span>
      </div>
      <dl className="welfare-capital-facts">
        <div>
          <dt>{msg("合同价格")}</dt>
          <dd>
            <Amount value={p.price_minor} />
          </dd>
        </div>
        <div>
          <dt>{msg("补贴比例")}</dt>
          <dd>{(result.rate_bps / 100).toLocaleString(getLocale())}%</dd>
        </div>
        <div className="is-award">
          <dt>{msg("预计补贴")}</dt>
          <dd>
            <Amount value={p.award_minor} />
          </dd>
        </div>
        <div>
          <dt>{msg("购舰完成时间")}</dt>
          <dd>{api.date(p.completed_at)}</dd>
        </div>
      </dl>
    </section>
  );
}

export function CapitalCaseContent({
  item: v,
  user,
  manage,
  history,
  error,
}: {
  item: api.Case;
  user: string;
  manage: boolean;
  history: { created_at: string; note: string }[];
  error?: string;
}) {
  const d = v.detail;
  const name =
    d.purchase?.items.find((i) => i.type_id === d.ship_type_id)?.name ||
    api.kinds[v.kind];
  const amount = capitalAmount(v);
  const step =
    v.state === "completed"
      ? 4
      : d.delivery
        ? 3
        : ["approved", "executing", "cancel_requested"].includes(v.state)
          ? 2
          : 1;
  return (
    <div className="welfare-capital-detail">
      <div className="welfare-capital-heading">
        <EveImage kind="type" id={d.ship_type_id} />
        <div>
          <strong>{name}</strong>
          <span>
            {api.caseLabel(v)} · #{v.id}
          </span>
        </div>
        <span className={`welfare-status state-${v.state}`}>
          {api.states[v.state]}
        </span>
      </div>
      <dl className="welfare-capital-facts">
        <div>
          <dt>{msg("接收角色")}</dt>
          <dd>{d.character_name}</dd>
        </div>
        <div className="is-award">
          <dt>{msg(v.award_minor ? "核准金额" : "预计补贴")}</dt>
          <dd>{amount ? <Amount value={amount} /> : "—"}</dd>
        </div>
        <div>
          <dt>{msg("合同价格")}</dt>
          <dd>{d.base_minor > 0 ? <Amount value={d.base_minor} /> : "—"}</dd>
        </div>
        <div>
          <dt>{msg("补贴比例")}</dt>
          <dd>
            {api.capitalRate(v.kind, d.rule).toLocaleString(getLocale())}%
          </dd>
        </div>
      </dl>
      {!["rejected", "cancelled", "reversed"].includes(v.state) && (
        <ol className="welfare-capital-progress" aria-label={msg("补贴进度")}>
          {["申请已提交", "审批通过", "合同已关联", "补贴已发放"].map(
            (label, i) => (
              <li
                key={label}
                className={i < step ? "is-done" : ""}
                aria-current={i === step - 1 ? "step" : undefined}
              >
                <span>
                  {i < step ? <Check size={14} aria-hidden="true" /> : i + 1}
                </span>
                {msg(label)}
              </li>
            ),
          )}
        </ol>
      )}
      {!d.purchase &&
        ["submitted", "information", "external"].includes(v.state) && (
          <p className="welfare-warning">
            {msg("旧申请缺少购舰合同核验证据，请撤回后重新申请。")}
          </p>
        )}
      {d.review_note && (
        <p className="welfare-proof">
          <strong>{msg("审批意见")}</strong> · {d.review_note}
        </p>
      )}
      <FulfillmentSummary item={v} user={user} manage={manage} />
      <details>
        <summary>{msg("购舰凭证")}</summary>
        <dl className="welfare-capital-facts">
          <div>
            <dt>{msg("购舰合同 ID")}</dt>
            <dd>{d.contract_id}</dd>
          </div>
          <div>
            <dt>{msg("购舰完成时间")}</dt>
            <dd>{api.date(d.purchase?.completed || "")}</dd>
          </div>
          <div>
            <dt>{msg("提交时间")}</dt>
            <dd>{api.date(v.created_at)}</dd>
          </div>
        </dl>
        {d.description && <p className="welfare-proof">{d.description}</p>}
        {d.evidence && <p className="welfare-proof">{d.evidence}</p>}
        {d.receipt && (
          <p className="welfare-proof">
            {msg("交付凭证：")} {d.receipt}
          </p>
        )}
        {d.skill_evidence && (
          <p>
            {msg("技能检查：")}{" "}
            {msg(
              d.skill_evidence.state === "met"
                ? "达标"
                : ["unmet", "missing"].includes(d.skill_evidence.state)
                  ? "未达标"
                  : "需核对",
            )}
          </p>
        )}
      </details>
      <details>
        <summary>{msg("处理记录")}</summary>
        {history.map((h, i) => (
          <div className="welfare-audit" key={i}>
            <time>{api.date(h.created_at)}</time>
            <p>{h.note || msg("提交申请")}</p>
          </div>
        ))}
      </details>
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
