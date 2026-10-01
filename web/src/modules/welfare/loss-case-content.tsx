import { msg } from "@/lib/i18n";
import { EveImage } from "@/components/eve-image";
import { ChevronDown } from "lucide-react";
import { FulfillmentSummary } from "./fulfillment-summary";
import { LossCombatAndItems, LossEvidence } from "./loss-browser";
import { ValuationView } from "./valuation";
import * as api from "./api";

export function LossCaseContent({
  item: v,
  user,
  manage,
  csrf,
  refresh,
  history,
  error,
}: {
  item: api.Case;
  user: string;
  manage: boolean;
  csrf: string;
  refresh: boolean;
  history: { created_at: string; note: string }[];
  error?: string;
}) {
  const pending = ["submitted", "information", "external"].includes(v.state);
  const d = v.detail;
  return (
    <div className="welfare-loss-detail">
      <LossIdentity item={v} />
      <div className="welfare-loss-total">
        <div>
          <span>{pending ? msg("核价金额") : msg("核准金额")}</span>
          <strong>
            {pending
              ? v.kind === "solo"
                ? d.valuation?.state === "ready"
                  ? `${api.money(d.valuation.amount_minor)} ISK`
                  : msg("待核价")
                : d.base_minor > 0
                  ? `${api.money(d.base_minor)} ISK`
                  : msg("审核时确定")
              : v.award_minor
                ? `${api.money(v.award_minor)} ISK`
                : "—"}
          </strong>
        </div>
        <div>
          <span>{msg("提交时间")}</span>
          <time>{api.date(v.created_at)}</time>
        </div>
      </div>
      {!pending && v.award_minor > 0 && (
        <div className="welfare-loss-breakdown">
          <span>
            {msg("核价金额")} {api.money(d.base_minor)} ISK
          </span>
          <span>
            {msg("补损比例")} {(d.rule.loss_rate_bps ?? 10000) / 100}%
          </span>
          {!!d.rule.loss_cap_minor && (
            <span>
              {msg("单笔上限")} {api.money(d.rule.loss_cap_minor)} ISK
            </span>
          )}
        </div>
      )}
      <FulfillmentSummary item={v} user={user} manage={manage} />
      <LossCaseEvidence
        item={v}
        csrf={csrf}
        refresh={refresh}
        history={history}
        error={error}
      />
    </div>
  );
}

function LossIdentity({ item: v, onOpenEvidence }: { item: api.Case; onOpenEvidence?: () => void }) {
  const loss = v.detail.loss_evidence;
  return (
    <div className="welfare-loss-identity">
      <div className="welfare-loss-heading">
        {loss && <EveImage kind="type" id={loss.ship_type_id} />}
        <div>
          <strong>{loss?.ship_name || api.caseLabel(v)}</strong>
          <small>
            {api.caseLabel(v)} #{v.id} · {v.detail.character_name}
          </small>
        </div>
        <span className={`welfare-status state-${v.state}`}>
          {api.states[v.state]}
        </span>
      </div>
      {loss && (
        <div className="welfare-loss-facts">
          {onOpenEvidence ? (
            <button
              type="button"
              className="welfare-km-link"
              onClick={onOpenEvidence}
              aria-label={`${msg("查看KM详情")} ${loss.id}`}
            >
              KM #{loss.id} <ChevronDown size={12} aria-hidden="true" />
            </button>
          ) : (
            <span>KM #{loss.id}</span>
          )}
          <span>{loss.solar_system_name}</span>
          <time dateTime={loss.occurred_at}>{api.date(loss.occurred_at)}</time>
        </div>
      )}
    </div>
  );
}

export function LossReviewOverview({ item: v, onOpenEvidence }: { item: api.Case; onOpenEvidence: () => void }) {
  const valuation = v.detail.valuation;
  return (
    <section className="welfare-loss-review-overview" aria-label={msg("补损审查要点")}>
      <LossIdentity item={v} onOpenEvidence={onOpenEvidence} />
      {v.kind === "solo" && (
        <div className="welfare-loss-review-quote">
          <span>
            {valuation?.source === "purchase_contract"
              ? msg("购舰合同核价")
              : msg("市场中间价核价")}
          </span>
          <strong>
            {valuation?.state === "ready"
              ? `${api.money(valuation.amount_minor)} ISK`
              : msg("待核价")}
          </strong>
          {valuation?.reason && valuation.state !== "ready" && (
            <small>{msg(valuation.reason)}</small>
          )}
        </div>
      )}
    </section>
  );
}

export function LossCaseEvidence({
  item: v,
  csrf,
  refresh,
  history,
  error,
  review = false,
}: {
  item: api.Case;
  csrf: string;
  refresh: boolean;
  history: { created_at: string; note: string }[];
  error?: string;
  review?: boolean;
}) {
  const d = v.detail;
  const pending = ["submitted", "information", "external"].includes(v.state);
  return (
    <div className="welfare-loss-evidence-sections">
      {review && d.loss_evidence && <LossCombatAndItems loss={d.loss_evidence} />}
      {!review && d.loss_evidence && (
        <details className="welfare-loss-evidence">
          <summary>
            {d.loss_evidence.ship_name} · KM #{d.killmail_id}
          </summary>
          <LossEvidence loss={d.loss_evidence} pilotName={d.character_name} showHeader={false} />
        </details>
      )}
      {v.kind === "solo" && (
        <details open={pending}>
          <summary>{msg("核价明细")}</summary>
          <ValuationView item={v} csrf={csrf} refresh={refresh && pending} detailOnly={review} />
        </details>
      )}
      {(d.description || d.evidence) && (
        <details>
          <summary>{msg("申请说明")}</summary>
          {d.description && <p className="welfare-proof">{d.description}</p>}
          {d.evidence && d.evidence !== d.description && (
            <p className="welfare-proof">{d.evidence}</p>
          )}
        </details>
      )}
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
