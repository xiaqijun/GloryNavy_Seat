import { beforeEach, expect, it, vi } from "vitest";
import {
  getContracts,
  type Contract,
  type Owner,
} from "@/modules/eve/contracts-api";
import { capitalContracts } from "./capital-contracts";

vi.mock("@/modules/eve/contracts-api", () => ({ getContracts: vi.fn() }));
const fetchPage = vi.mocked(getContracts);
const owner: Owner = { kind: "character", id: "123", name: "Pilot" };
const purchase = (id: string, extra = {}): Contract =>
  ({
    id,
    type: "item_exchange",
    status: "finished",
    for_corporation: false,
    acceptor: { id: "123" },
    assignee: { id: "123" },
    issuer: { id: "456" },
    price: "100",
    reward: "0",
    ...extra,
  }) as Contract;
beforeEach(() => fetchPage.mockReset());

it("fills sparse raw pages and includes a corporation selling to the personal buyer", async () => {
  fetchPage
    .mockResolvedValueOnce({
      items: [purchase("950"), purchase("940", { acceptor: { id: "999" } })],
      next_cursor: "900",
    })
    .mockResolvedValueOnce({
      items: [purchase("850", { for_corporation: true })],
      next_cursor: "800",
    })
    .mockResolvedValueOnce({ items: [purchase("750")], next_cursor: "" });
  const signal = new AbortController().signal;
  const result = await capitalContracts(owner, "Aeon", "", signal);
  expect(result.items.map((c) => c.id)).toEqual(["950", "850", "750"]);
  expect(result.next_cursor).toBe("");
  expect(fetchPage.mock.calls.map((c) => c[1].get("before"))).toEqual([
    "",
    "900",
    "800",
  ]);
  expect(
    fetchPage.mock.calls.every(
      (c) => c[1].get("q") === "Aeon" && c[2] === signal,
    ),
  ).toBe(true);
});

it("retains the cursor after a bounded scan even when every page is excluded", async () => {
  for (let i = 0; i < 5; i++)
    fetchPage.mockResolvedValueOnce({
      items: [purchase(String(1000 - i), { acceptor: { id: "999" } })],
      next_cursor: String(900 - i),
    });
  const result = await capitalContracts(owner, "");
  expect(fetchPage).toHaveBeenCalledTimes(5);
  expect(result).toEqual({ items: [], next_cursor: "896" });
  fetchPage.mockResolvedValueOnce({
    items: [purchase("800")],
    next_cursor: "",
  });
  expect(
    (await capitalContracts(owner, "", result.next_cursor)).items.map(
      (c) => c.id,
    ),
  ).toEqual(["800"]);
});

it("stops at a full candidate page without losing later contracts", async () => {
  fetchPage.mockResolvedValueOnce({
    items: Array.from({ length: 25 }, (_, i) => purchase(String(1000 - i))),
    next_cursor: "975",
  });
  const result = await capitalContracts(owner, "");
  expect(fetchPage).toHaveBeenCalledTimes(1);
  expect(result.items).toHaveLength(25);
  expect(result.next_cursor).toBe("975");
});

it("rejects broken cursors and fails the whole batch when a later page fails", async () => {
  fetchPage.mockResolvedValueOnce({ items: [], next_cursor: "900" });
  await expect(capitalContracts(owner, "", "900")).rejects.toThrow();
  fetchPage
    .mockResolvedValueOnce({ items: [purchase("1000")], next_cursor: "900" })
    .mockRejectedValueOnce(new Error("second page failed"));
  await expect(capitalContracts(owner, "")).rejects.toThrow(
    "second page failed",
  );
});
