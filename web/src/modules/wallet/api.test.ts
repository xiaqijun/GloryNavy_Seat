import { describe, expect, it } from "vitest";
import { money, isRecords, isContext, isSummary, refLabel, contextLabel } from "./api";
describe("wallet precision and boundaries", () => {
  it("localizes less common official journal and context types without changing codes", () => {
    expect(refLabel("bounty_prize_corporation_tax")).toBe("追击赏金 军团税金");
    expect(refLabel("ess_escrow_transfer")).toBe("事件监测装置保证金支付");
    expect(refLabel("freelance_jobs_reward_corporation_tax")).toBe(
      "自由任务奖励军团税",
    );
    expect(contextLabel("industry_job_id")).toBe("Industry job");
    expect(contextLabel("future_context")).toBe("future_context");
  });
  it("formats large decimal money without floating point", () => {
    expect(money("123456789012345.67")).toBe("123,456,789,012,345.67");
    expect(money("1.234e5")).toBe("123,400.00");
    expect(money("-1e-2")).toBe("-0.01");
    expect(money("0")).toBe("0.00");
    expect(money(null)).toBe("—");
    expect(money("1e99999")).toBe("—");
  });
  it("rejects numeric IDs and keeps unknown official types", () => {
    const r = {
      id: "9007199254740993",
      division: 0,
      observed_at: "2026-09-16T00:00:00Z",
      amount: "0.01",
    };
    expect(isRecords({ items: [r], names: {}, next_cursor: "" })).toBe(true);
    expect(
      isRecords({ items: [{ ...r, id: 42 }], names: {}, next_cursor: "" }),
    ).toBe(false);
    expect(
      isRecords({
        items: [{ ...r, amount: 0.01 }],
        names: {},
        next_cursor: "",
      }),
    ).toBe(false);
    expect(
      isContext({
        owners: [
          {
            kind: "corporation",
            id: "10",
            name: "Corp",
            divisions: [8],
            journal: true,
            transactions: false,
          },
        ],
      }),
    ).toBe(false);
    expect(refLabel("future_official_type")).toBe("future_official_type");
  });
  it("validates batch summaries without converting exact balances to numbers", () => {
    const item = { owner_id: "9007199254740993", balance: "9007199254740993.12", observed_at: "2026-09-24T00:00:00Z", income: "0", expense: "0.01" };
    expect(isSummary({ items: [item] })).toBe(true);
    expect(isSummary({ items: [{ ...item, owner_id: 201 }] })).toBe(false);
    expect(isSummary({ items: [{ ...item, balance: 1.2 }] })).toBe(false);
  });
});
