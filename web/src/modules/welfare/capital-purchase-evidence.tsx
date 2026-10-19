import { EveImage } from "@/components/eve-image";
import { msg } from "@/lib/i18n";
import {
  amount,
  contractDate,
  contractStatuses,
} from "@/modules/eve/contracts-api";
import { capitalMoney } from "./capital-api";
import * as api from "./api";

export function CapitalPurchaseEvidence({ item }: { item: api.Case }) {
  const d = item.detail;
  const c = d.purchase;
  if (!c)
    return (
      <div className="welfare-capital-purchase">
        <p>
          {msg("购舰合同 #")}
          {d.contract_id}
        </p>
        <p role="status">{msg("未保存购舰合同明细")}</p>
      </div>
    );
  const price = amount(c.price);
  return (
    <section
      className="welfare-capital-purchase"
      aria-label={msg("购舰合同明细")}
    >
      <div className="welfare-contract-heading">
        <strong>
          {msg("合同 #")}
          {c.id}
        </strong>
        <span>{contractStatuses[c.status] || c.status}</span>
      </div>
      {c.title && <p>{c.title}</p>}
      <dl className="welfare-capital-facts">
        <div>
          <dt>{msg("购买角色")}</dt>
          <dd>{d.character_name || msg("角色 #{0}", c.owner_id)}</dd>
        </div>
        <div>
          <dt>{msg("卖方")}</dt>
          <dd>
            {c.issuer_name ||
              (c.for_corporation
                ? msg("军团 #{0}", c.issuer_corporation_id)
                : msg("角色 #{0}", c.issuer_id))}
          </dd>
        </div>
        <div>
          <dt>{msg("合同价格")}</dt>
          <dd>
            <span title={`${price} ISK`}>
              {price === "—"
                ? price
                : `${capitalMoney(Number(c.price) * 100)} ISK`}
            </span>
          </dd>
        </div>
        <div>
          <dt>{msg("购舰完成时间")}</dt>
          <dd>{contractDate(c.completed)}</dd>
        </div>
        <div>
          <dt>{msg("创建时间")}</dt>
          <dd>{contractDate(c.issued)}</dd>
        </div>
        <div>
          <dt>{msg("凭证核验时间")}</dt>
          <dd>{contractDate(c.checked_at)}</dd>
        </div>
      </dl>
      {!c.items_ready && <p role="status">{msg("等待合同明细同步")}</p>}
      {c.items.length > 0 ? (
        <ul
          className="welfare-contract-items"
          aria-label={msg("合同物品")}
          tabIndex={0}
        >
          {c.items.map((i, n) => (
            <li key={n}>
              <EveImage kind="type" id={i.type_id} />
              <span className="welfare-contract-item-text">
                {i.name || msg("物品 #{0}", i.type_id)}
                {!i.included && <small>{msg("（索取）")}</small>}
              </span>
              <strong>× {i.quantity}</strong>
            </li>
          ))}
        </ul>
      ) : c.items_ready ? (
        <p role="status">{msg("暂无物品明细")}</p>
      ) : null}
    </section>
  );
}
