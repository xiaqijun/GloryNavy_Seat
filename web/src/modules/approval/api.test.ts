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
});
