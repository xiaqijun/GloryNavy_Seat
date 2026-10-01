import { beforeEach, describe, expect, it, vi } from "vitest";
import { getContracts, type Contract, type Owner } from "./contracts-api";
import {
  collectContracts,
  contractsCSV,
  csvCell,
  exportLimit,
} from "./contracts-export";
vi.mock("./contracts-api", async (original) => ({
  ...(await original<typeof import("./contracts-api")>()),
  getContracts: vi.fn(),
}));
const owner: Owner = { kind: "character", id: "123", name: "舰长" };
const entity = { id: "0", name: "", category: "" };
const sample: Contract = {
  id: "3",
  title: "",
  summary: "三钛合金 × 100",
  trade_direction: "sell",
  type: "item_exchange",
  status: "outstanding",
  availability: "public",
  for_corporation: false,
  issuer: { ...entity, id: "123", name: "舰长" },
  assignee: entity,
  acceptor: entity,
  start: entity,
  end: entity,
  price: "9007199254740993.123456789",
  reward: null,
  collateral: null,
  buyout: null,
  volume: null,
  days_to_complete: null,
  date_issued: "2026-09-14T00:00:00Z",
  date_expired: "",
  date_accepted: "",
  date_completed: "",
  checked_at: "2026-09-14T01:00:00Z",
};
beforeEach(() => vi.resetAllMocks());
describe("contract export scope and failure boundaries", () => {
  it("starts from the first page and preserves only business filters", async () => {
    const requests: string[] = [];
    vi.mocked(getContracts).mockImplementation(
      async (actualOwner, params, signal) => {
        expect(actualOwner).toEqual(owner);
        expect(signal).toBeDefined();
        requests.push(params.toString());
        return params.has("before")
          ? { items: [{ ...sample, id: "1" }], next_cursor: "" }
          : { items: [sample, { ...sample, id: "2" }], next_cursor: "2" };
      },
    );
    const result = await collectContracts(
      owner,
      new URLSearchParams({
        q: "舰队",
        type: "item_exchange",
        status: "outstanding",
        before: "1",
        member: "other-user",
        kind: "corporation",
      }),
      new AbortController().signal,
    );
    expect(result.map((c) => c.id)).toEqual(["3", "2", "1"]);
    expect(requests).toEqual([
      "q=%E8%88%B0%E9%98%9F&type=item_exchange&status=outstanding",
      "q=%E8%88%B0%E9%98%9F&type=item_exchange&status=outstanding&before=2",
    ]);
  });
  it("discards partial results when a later page loses authorization", async () => {
    vi.mocked(getContracts)
      .mockResolvedValueOnce({ items: [sample], next_cursor: "3" })
      .mockRejectedValueOnce(new Error("没有查看权限"));
    await expect(
      collectContracts(
        owner,
        new URLSearchParams(),
        new AbortController().signal,
      ),
    ).rejects.toThrow("没有查看权限");
  });
  it("honors cancellation even if the transport resolves after abort", async () => {
    const controller = new AbortController();
    const progress = vi.fn();
    vi.mocked(getContracts).mockImplementation(async () => {
      controller.abort();
      return { items: [sample], next_cursor: "" };
    });
    await expect(
      collectContracts(
        owner,
        new URLSearchParams(),
        controller.signal,
        progress,
      ),
    ).rejects.toThrow();
    expect(progress).not.toHaveBeenCalled();
  });
  it("rejects duplicate or non-advancing pages instead of exporting duplicates", async () => {
    vi.mocked(getContracts).mockResolvedValue({
      items: [sample],
      next_cursor: "3",
    });
    await expect(
      collectContracts(
        owner,
        new URLSearchParams(),
        new AbortController().signal,
      ),
    ).rejects.toThrow("分页已变化");
    expect(getContracts).toHaveBeenCalledTimes(2);
  });
  it("rejects oversized exports rather than silently truncating", async () => {
    vi.mocked(getContracts).mockResolvedValue({
      items: Array.from({ length: exportLimit + 1 }, (_, i) => ({
        ...sample,
        id: String(exportLimit + 1 - i),
      })),
      next_cursor: "",
    });
    await expect(
      collectContracts(
        owner,
        new URLSearchParams(),
        new AbortController().signal,
      ),
    ).rejects.toThrow("10,000");
  });
});
describe("CSV data integrity", () => {
  it("keeps Chinese labels, summaries, exact decimals and recipient precedence", () => {
    const csv = contractsCSV(owner, [
      {
        ...sample,
        assignee: { ...entity, id: "456", name: "指定方" },
        acceptor: { ...entity, id: "789", name: "接受方" },
      },
    ]);
    expect(csv.startsWith("\uFEFF")).toBe(true);
    expect(csv).toContain('"三钛合金 × 100"');
    expect(csv).toContain('"出售"');
    expect(csv).toContain('"9007199254740993.123456789"');
    expect(csv).toContain('"接受方","789","指定方","456","接受方","789"');
    expect(contractsCSV(owner, [sample])).toContain('"公开","0"');
  });
  it("escapes delimiters, quotes and multiline text and neutralizes formulas", () => {
    expect(csvCell('舰长,"甲"\n补给')).toBe('"舰长,""甲""\n补给"');
    for (const value of [
      "=1+1",
      " +SUM(A1)",
      "-2+3",
      "@cmd",
      "\t=1",
      "\r=1",
      "\n=1",
    ])
      expect(csvCell(value)).toBe("\"'" + value + '"');
    expect(csvCell(null)).toBe('""');
  });
});
