import { describe, it, expect } from "vitest";
import { validateDetails, isProfile } from "./api";
describe("community details", () => {
  it("validates local input rules without treating a nickname as verified identity", () => {
    expect(validateDetails(" 123456 ", " 舰长 🚀 ")).toEqual({
      qq: "",
      kook: "",
    });
    expect(validateDetails("012345", "valid").qq).not.toBe("");
    expect(validateDetails("１２３４５６", "valid").qq).not.toBe("");
    expect(validateDetails("123456", "a\nb").kook).not.toBe("");
    expect(validateDetails("123456", "🚀".repeat(64)).kook).toBe("");
    expect(validateDetails("123456", "🚀".repeat(65)).kook).not.toBe("");
  });
  it("rejects malformed confirmation data", () => {
    const p = {
      version: "1",
      complete: true,
      qq: { value: "123456", version: "1", confirmation: "pending" },
      kook: { value: "舰长", version: "1", confirmation: "pending" },
    };
    expect(isProfile(p)).toBe(true);
    expect(isProfile({ ...p, qq: { ...p.qq, confirmation: true } })).toBe(
      false,
    );
  });
});
