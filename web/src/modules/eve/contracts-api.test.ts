import { describe, expect, it } from "vitest";
import {
  amount,
  entityName,
  isContract,
  title,
  type Contract,
} from "./contracts-api";
describe("contract display boundaries", () => {
  it("preserves descriptions and uses summaries only for blank descriptions", () => {
    const c = {
      id: "123",
      type: "item_exchange",
      title: " 原始描述 ",
      summary: "三钛合金 × 10",
    } as Contract;
    expect(title(c)).toBe("原始描述");
    expect(title({ ...c, title: " \n " })).toBe("三钛合金 × 10");
    expect(title({ ...c, title: "", summary: "" })).toBe("物品交换");
    expect(title({ ...c, title: "", summary: undefined })).toBe("物品交换");
  });
  it("preserves decimal precision including exponent values", () => {
    expect(amount("123456789012345.67")).toBe("123,456,789,012,345.67");
    expect(amount("9007199254740993")).toBe("9,007,199,254,740,993");
    expect(amount("1.234e3")).toBe("1,234");
    expect(amount("1e-3")).toBe("0.001");
    expect(amount(null)).toBe("—");
    expect(amount("0")).toBe("0");
  });
  it("retains unknown IDs and distinguishes an unassigned party", () => {
    expect(entityName({ id: "123", name: "", category: "" })).toBe("#123");
    expect(entityName({ id: "0", name: "", category: "" }, "公开")).toBe(
      "公开",
    );
  });
  it("rejects raw upstream payloads and malformed contract responses", () => {
    expect(isContract({ contract_id: 123, price: 1.2 })).toBe(false);
    expect(isContract(null)).toBe(false);
  });
});
