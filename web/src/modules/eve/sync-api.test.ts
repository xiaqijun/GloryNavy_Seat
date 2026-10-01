import { describe, expect, it } from "vitest";
import {
  isSyncList,
  resourceLabel,
  syncLabel,
  type SyncTarget,
} from "./sync-api";
const target: SyncTarget = {
  id: "1",
  character_id: "123",
  name: "Pilot",
  resource: "profile",
  state: "queued",
  freshness: "never",
  reason: "",
  last_attempt_at: null,
  last_success_at: null,
  content_updated_at: null,
  next_due_at: "2026-09-14T00:00:00Z",
};
describe("sync contract", () => {
  it("accepts mixed production resources including the training queue", () => {
    const resources = [
      "profile",
      "authorization",
      "character_contracts",
      "corporation_contracts",
      "fittings",
      "skills",
      "skillqueue",
      "killmails",
      "online",
    ];
    const data = {
      available: true,
      targets: resources.map((resource) => ({ ...target, resource })),
    };
    expect(isSyncList(data)).toBe(true);
    expect(resourceLabel("skillqueue")).toBe("训练队列");
    expect(resourceLabel("killmails")).toBe("舰船损失");
    expect(
      isSyncList({
        available: true,
        targets: [{ ...target, resource: "unknown" }],
      }),
    ).toBe(false);
  });
  it("distinguishes contract lists from incomplete details", () => {
    const partial = {
      ...target,
      resource: "character_contracts" as const,
      state: "idle" as const,
      freshness: "fresh" as const,
      pending_details: 3,
    };
    expect(isSyncList({ available: true, targets: [partial] })).toBe(true);
    expect(syncLabel(partial)).toBe("列表已更新");
    expect(syncLabel({ ...partial, failed_details: 1 })).toBe("明细待处理");
    expect(
      isSyncList({
        available: true,
        targets: [{ ...partial, pending_details: -1 }],
      }),
    ).toBe(false);
  });
  it("keeps queued distinct from successful data", () => {
    expect(syncLabel(target)).toBe("已排队");
    expect(syncLabel({ ...target, state: "idle", freshness: "stale" })).toBe(
      "待更新",
    );
  });
  it("rejects invalid IDs, states and timestamps", () => {
    expect(isSyncList({ available: true, targets: [target] })).toBe(true);
    for (const change of [
      { id: 1 },
      { character_id: "0" },
      { state: "success" },
      { last_success_at: "yesterday" },
      { next_due_at: null },
    ]) {
      expect(
        isSyncList({ available: true, targets: [{ ...target, ...change }] }),
      ).toBe(false);
    }
  });
});
