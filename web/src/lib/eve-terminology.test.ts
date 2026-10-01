import { describe, expect, it } from "vitest";
import { assetSlotLabel, gameTerm } from "./eve-terminology";

describe("game terminology boundaries", () => {
  it("keeps unknown protocol values verbatim and rejects prototype properties", () => {
    for (const value of ["future_ref_type", "toString", "__proto__"])
      expect(gameTerm("wallet", value, "zh-CN")).toBe(value);
    expect(gameTerm("roles", "Hangar_Take_8", "zh-CN")).toBe("Hangar_Take_8");
  });

  it("selects source language and uses English for an unverified protocol label", () => {
    expect(gameTerm("wallet", "ess_escrow_transfer", "zh-CN")).toBe(
      "事件监测装置保证金支付",
    );
    expect(gameTerm("wallet", "ess_escrow_transfer", "en")).toBe(
      "ESS Escrow Payment",
    );
    expect(gameTerm("contexts", "industry_job_id", "zh-CN")).toBe(
      "Industry job",
    );
  });

  it("uses one slot label across views without corrupting unknown location flags", () => {
    expect(assetSlotLabel("FighterBay")).toBe(gameTerm("slots", "fighter_bay"));
    expect(assetSlotLabel("HiSlot0", true)).toBe(
      `${gameTerm("slots", "high")} 1`,
    );
    expect(assetSlotLabel("HiSlot7")).toBe(gameTerm("slots", "high"));
    for (const flag of [
      "FighterBay99",
      "FutureBay",
      "HiSlot9007199254740992",
      "27",
      "constructor",
    ])
      expect(assetSlotLabel(flag, true)).toBe(flag);
  });
});
