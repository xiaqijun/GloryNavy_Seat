// Rebuild the exact backend message subset from the shared UI English catalog.
import fs from "node:fs";
import path from "node:path";
import ts from "../web/node_modules/typescript/lib/typescript.js";

const root = path.resolve(import.meta.dirname, "..");
const source = fs.readFileSync(path.join(root, "web/src/lib/locales/en.ts"), "utf8");
const tree = ts.createSourceFile("en.ts", source, ts.ScriptTarget.Latest, true);
const english = {};
function visit(node) {
  if (ts.isPropertyAssignment(node) && ts.isStringLiteral(node.initializer)) {
    english[node.name.text] = node.initializer.text;
  }
  ts.forEachChild(node, visit);
}
visit(tree);
const messages = {};
function scan(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const file = path.join(dir, entry.name);
    if (entry.isDirectory()) scan(file);
    else if (entry.name.endsWith(".go") && !entry.name.endsWith("_test.go")) {
      for (const match of fs.readFileSync(file, "utf8").matchAll(/"(?:[^"\\\r\n]|\\.)*"/g)) {
        let key;
        try { key = JSON.parse(match[0]); } catch { continue; }
        if (/\p{Script=Han}/u.test(key) && Object.hasOwn(english, key)) messages[key] = english[key];
      }
    }
  }
}
scan(path.join(root, "internal"));
const text = JSON.stringify(Object.fromEntries(Object.entries(messages).sort(([a], [b]) => a.localeCompare(b, "en"))), null, 2) + "\n";
const target = path.join(root, "internal/platform/locale/messages.en.json");
if (process.argv.includes("--check")) {
  if (fs.readFileSync(target, "utf8") !== text) throw new Error("Run node scripts/backend-messages.mjs");
} else fs.writeFileSync(target, text);
console.log(`${Object.keys(messages).length} backend messages verified`);
