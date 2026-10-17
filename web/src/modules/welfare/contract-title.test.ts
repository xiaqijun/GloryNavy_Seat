import { describe, expect, it } from "vitest";
import { settlementContractTitle } from "./contract-title";

describe("settlement contract title", () => {
  it("copies the game's exact title for historical batch references", () => {
    const legacy = "BATCH-20260930-21DE0416-40ED-4E85-9424-5CB1E8687F39";
    expect(settlementContractTitle(legacy)).toBe("BATCH-20260930-21DE0416-40ED-4E85-9424-5CB1E8687F3");
    expect(settlementContractTitle(legacy)).toHaveLength(50);
  });
  it("preserves new IDs and other reference formats exactly", () => {
    for (const ref of ["BATCH-20261018-000001", "EX-20261018-000002", "SRP-20261018-000003", "GNV-WF-123", "", "unrecognized-".repeat(5)]) {
      expect(settlementContractTitle(ref)).toBe(ref);
    }
  });
});
