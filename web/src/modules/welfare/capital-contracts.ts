import { msg } from "@/lib/i18n";
import {
  getContracts,
  type Contract,
  type Owner,
  type Page,
} from "@/modules/eve/contracts-api";

// The API paginates all completed exchanges. Fill sparse buyer pages without
// dropping the raw cursor or issuing unbounded reads for long histories.
export async function capitalContracts(
  owner: Owner,
  search: string,
  before = "",
  signal?: AbortSignal,
): Promise<Page<Contract>> {
  const items: Contract[] = [];
  const seen = new Set<string>();
  let cursor = before;
  for (let page = 0; page < 5; page++) {
    const result = await getContracts(
      owner,
      new URLSearchParams({
        type: "item_exchange",
        status: "finished",
        q: search,
        before: cursor,
      }),
      signal,
    );
    for (const c of result.items) {
      // for_corporation describes the issuer, not the buyer. A corporation
      // selling to this character is still a personal purchase for the owner.
      if (
        c.type === "item_exchange" &&
        c.status === "finished" &&
        c.acceptor.id === owner.id &&
        c.issuer.id !== owner.id &&
        c.issuer.id !== "0" &&
        (c.assignee.id === "0" || c.assignee.id === owner.id) &&
        Number(c.price) > 0 &&
        c.reward !== null &&
        Number(c.reward) === 0 &&
        !seen.has(c.id)
      ) {
        seen.add(c.id);
        items.push(c);
      }
    }
    const next = result.next_cursor;
    if (next && (next === "0" || (cursor && BigInt(next) >= BigInt(cursor))))
      throw new Error(msg("分页信息异常"));
    cursor = next;
    if (!cursor || items.length >= 25) break;
  }
  return { items, next_cursor: cursor };
}
