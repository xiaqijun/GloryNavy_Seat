import test from "node:test";
import assert from "node:assert/strict";
import { classifyPage, papURL, inspectPAP, manualLoginArgs } from "./browser.mjs";

test("manual login uses a dedicated normal browser with no automation or debugging switches", () => {
  const directory = new URL("../../.local/winterco-browser/msedge/", import.meta.url);
  const args = manualLoginArgs(directory, "2123937477");
  assert.equal(args.length, 3);
  assert.match(args[0], /^--user-data-dir=.*winterco-browser[\\/]msedge[\\/]profile[\\/]?$/);
  assert.equal(args[1], "--new-window");
  assert.equal(args[2], papURL("2123937477"));
});

test("only the exact requested WinterCo PAP page is inspectable", () => {
  const id = "2123937477";
  assert.equal(classifyPage(papURL(id), id, 200), "inspectable");
  assert.equal(classifyPage(papURL("123"), id, 200), "unexpected_destination");
  assert.equal(
    classifyPage("https://example.org/character/view/paps/" + id, id, 200),
    "unexpected_destination",
  );
  assert.equal(
    classifyPage("https://seat.winterco.org/auth/login", id, 200),
    "login_required",
  );
  assert.equal(
    classifyPage("https://login.eveonline.com/v2/oauth/authorize", id, 200),
    "login_required",
  );
  assert.equal(classifyPage(papURL(id), id, 403), "permission_denied");
  assert.equal(classifyPage(papURL(id), id, 429), "rate_limited");
  assert.equal(classifyPage(papURL(id), id, 500), "upstream_error");
  assert.throws(() => papURL("../123"));
});

test("login redirect never reads DOM or returns zero points", async () => {
  const page = {
    goto: async () => ({ status: () => 200 }),
    url: () => "https://seat.winterco.org/auth/login",
    evaluate: () => {
      throw new Error("Must not read credentials");
    },
  };
  assert.deepEqual(await inspectPAP(page, "123"), { state: "login_required" });
});

test("raw page inspection is explicitly incomplete and cannot issue money", async () => {
  const page = {
    goto: async () => ({ status: () => 200 }),
    url: () => papURL("123"),
    locator: () => ({ waitFor: async () => {} }),
    evaluate: async () => ({ title: "PAP", text: "Loading", tables: [] }),
  };
  const snapshot = await inspectPAP(page, "123");
  assert.equal(snapshot.state, "awaiting_adapter");
  assert.equal(snapshot.complete, false);
  assert.equal(snapshot.importable, false);
  assert.equal("points" in snapshot, false);
});
