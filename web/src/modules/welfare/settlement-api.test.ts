import { afterEach, expect, it, vi } from "vitest";
import { create, detail, list } from "./settlement-api";

afterEach(() => vi.unstubAllGlobals());
const batch = { id: "14", request_key: "request", actor_id: "actor", note: "", state: "pending", total_count: 1, completed_count: 0, failed_count: 0, created_at: "2026-10-18T00:00:00Z", next_run_at: "2026-10-18T00:00:00Z", version: "1", settlement_reference: "BATCH-20261013-000001", account_id: "account", isk_minor: 19563315493, items: [], recipient_ids: [2117334386], contract_id: 237156587 };

for (const delivery_status of ["awaiting_acceptance", "waiting_contract", "contract_deleted", "contract_cancelled", "contract_rejected", "contract_failed", "contract_mismatch", "multiple_contracts"]) {
  it(`settlement API accepts ${delivery_status} in lists, details and create responses`, async () => {
    const current = { ...batch, delivery_status };
    const fetcher = vi.fn().mockResolvedValueOnce(Response.json({ data: { items: [current] } }))
      .mockResolvedValueOnce(Response.json({ data: { batch: current, items: [] } }))
      .mockResolvedValueOnce(Response.json({ data: { batch: current, items: [] } }));
    vi.stubGlobal("fetch", fetcher);
    expect((await list()).items[0].delivery_status).toBe(delivery_status);
    expect((await detail("14")).batch.contract_id).toBe(237156587);
    expect((await create("csrf", [{ source: "welfare", id: "89" }])).batch.delivery_status).toBe(delivery_status);
    expect(fetcher.mock.calls[2][1].method).toBe("POST");
  });
}
it("rejects unknown delivery statuses", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(Response.json({ data: { batch: { ...batch, delivery_status: "paid_without_verification" }, items: [] } })));
  await expect(detail("14")).rejects.toThrow();
});
