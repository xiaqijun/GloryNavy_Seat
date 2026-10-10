import { describe, expect, it } from "vitest";
import { isItem, lossQueueAmount, type QueueItem } from "./api";

const item = (overrides: Partial<QueueItem> = {}): QueueItem => ({
  source: "future-review",
  id: "7",
  version: "1",
  account_id: "account-1",
  applicant: "",
  corporation_id: "",
  kind: "future_kind",
  state: "submitted",
  status: "",
  recipient: "Pilot",
  title: "Future review",
  reference: "FR-7",
  amount_minor: 100,
  unit: "isk",
  time: "2026-10-10T00:00:00Z",
  action: "",
  actions: [],
  payload: { reference: "FR-7" },
  ...overrides,
});

describe("approval source contract", () => {
  it("accepts a registered future source without changing the central validator", () => {
    expect(isItem(item())).toBe(true);
  });

  it("does not treat an incomplete welfare payload as a renderable loss quote", () => {
    expect(
      lossQueueAmount(item({ source: "welfare", kind: "solo", payload: {} })),
    ).toBeNull();
  });

  it("preserves incomplete and unavailable appraisal states for the UI", () => {
    const valuation = {
      source: "market",
      state: "incomplete",
      reason: "部分物品缺少双边报价，不能按小计核准",
      at: "2026-10-10T00:00:00Z",
      amount_minor: 0,
      settings_version: "1",
    };
    const payload = { detail: { valuation } } as QueueItem["payload"];
    expect(lossQueueAmount(item({ source: "welfare", kind: "solo", payload }))).toEqual({ amount: 0, label: "incomplete" });
    expect(lossQueueAmount(item({ source: "welfare", kind: "solo", payload: { detail: { valuation: { ...valuation, state: "unavailable" } } } as QueueItem["payload"] }))).toEqual({ amount: 0, label: "unavailable" });
  });
});
