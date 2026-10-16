import { describe, expect, it } from "vitest";
import { formatCompactISK } from "./amount-format";

describe("loan summary amounts", () => {
  it.each(["zh-CN", "en-US"])("keeps M/B/T consistent in %s and small amounts exact", (locale) => {
    expect(formatCompactISK(0, locale)).toBe("0 ISK");
    expect(formatCompactISK(123456, locale)).toBe("1,234.56 ISK");
    expect(formatCompactISK(99999999, locale)).toBe("999,999.99 ISK");
    expect(formatCompactISK(100000000, locale)).toBe("1M ISK");
    expect(formatCompactISK(125678901, locale)).toBe("1.26M ISK");
    expect(formatCompactISK(100000000000, locale)).toBe("1B ISK");
    expect(formatCompactISK(125000000000000, locale)).toBe("1.25T ISK");
    expect(formatCompactISK(-125000000, locale)).toBe("-1.25M ISK");
    expect(formatCompactISK(99999999999, locale)).toBe("1B ISK");
  });
});
