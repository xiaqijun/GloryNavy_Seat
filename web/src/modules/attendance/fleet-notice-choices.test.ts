import { describe, expect, it } from "vitest";
import { fittingDNA } from "./fleet-notice-choices";
import type { Fit } from "@/modules/fittings/model";

describe("fleet fitting link", () => {
  it("freezes fitted modules and cargo in ship DNA", () => {
    const fit: Fit = { name: "test", ship_type_id: "587", skill_mode: "all5", items: [
      { type_id: "34", slot: "high", index: 0, quantity: 1, state: "active", charge_id: "10" },
      { type_id: "34", slot: "high", index: 1, quantity: 1, state: "active", charge_id: "10" },
      { type_id: "35", slot: "cargo", index: 0, quantity: 20, state: "online" },
    ] };
    expect(fittingDNA(fit)).toBe("587:34;2:10;2:35;20::");
  });
  it("refuses invalid item identifiers instead of producing markup", () => {
    const fit: Fit = { name: "test", ship_type_id: "587", skill_mode: "all5", items: [
      { type_id: '34" onclick="bad', slot: "high", index: 0, quantity: 1, state: "active" },
    ] };
    expect(fittingDNA(fit)).toBe("");
  });
});
