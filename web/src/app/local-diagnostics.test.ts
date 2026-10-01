import { createServer, type Server } from "node:http";
import { afterAll, beforeAll, expect, test } from "vitest";
import { interactionMiddleware } from "../../tooling/interaction-diagnostics";

let server: Server;
let origin: string;
const report = {
  kind: "main_thread_stall",
  page: "/losses",
  source: "worker",
  browser: "edge",
  tab: "00000000-0000-4000-8000-000000000001",
  duration_ms: 5500,
};
beforeAll(async () => {
  server = createServer(interactionMiddleware());
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  if (!address || typeof address === "string") throw Error("No test listener");
  origin = `http://127.0.0.1:${address.port}`;
});
afterAll(
  () =>
    new Promise<void>((resolve, reject) =>
      server.close((e) => (e ? reject(e) : resolve())),
    ),
);
function post(value: unknown, headers: Record<string, string> = {}) {
  return fetch(origin, {
    method: "POST",
    headers: { Origin: origin, "Content-Type": "application/json", ...headers },
    body: JSON.stringify(value),
  });
}
test("accepts only fixed metadata and does not retain extra fields", async () => {
  expect(
    (
      await post({
        ...report,
        secret: "never-store",
        url: "https://private.test/?token=secret",
      })
    ).status,
  ).toBe(204);
  const result = await fetch(origin);
  expect(result.headers.get("cache-control")).toBe("no-store");
  const text = await result.text();
  expect(text).not.toContain("secret");
  expect(JSON.parse(text).entries.at(-1)).toMatchObject(report);
});
test("rejects cross-origin writes and reads", async () => {
  expect(
    (await post(report, { Origin: "https://elsewhere.test" })).status,
  ).toBe(403);
  expect(
    (await fetch(origin, { headers: { Origin: "https://elsewhere.test" } }))
      .status,
  ).toBe(403);
  expect((await post(report, { "Sec-Fetch-Site": "cross-site" })).status).toBe(
    403,
  );
});
test("rejects raw paths, invalid categories and oversized payloads", async () => {
  for (const value of [
    { ...report, page: "/losses?member=private" },
    { ...report, kind: "anything" },
    { ...report, duration_ms: -1 },
  ]) {
    expect((await post(value)).status).toBe(400);
  }
  expect((await post({ ...report, extra: "x".repeat(2000) })).status).toBe(413);
});
test("retains at most 80 records", async () => {
  for (let i = 0; i < 85; i++) await post(report);
  expect((await (await fetch(origin)).json()).entries).toHaveLength(80);
});

test("heartbeat updates latest status without retaining repeated samples", async () => {
  for (let i = 0; i < 3; i++)
    await post({
      ...report,
      kind: "heartbeat",
      visible: true,
      body_blocked: false,
      navigation_hit: true,
      overlays: 0,
    });
  const data = await (await fetch(origin)).json();
  expect(
    data.entries.filter((e: { kind: string }) => e.kind === "heartbeat"),
  ).toHaveLength(1);
  expect((await post({ ...report, overlays: "invalid" })).status).toBe(400);
});
