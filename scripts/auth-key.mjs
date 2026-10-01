import { randomBytes } from "node:crypto";
import { readFileSync, writeFileSync, existsSync, copyFileSync } from "node:fs";
import { parseEnv } from "node:util";
const file = new URL("../.env", import.meta.url);
if (!existsSync(file)) copyFileSync(new URL("../.env.example", import.meta.url), file);
const text = readFileSync(file, "utf8");
if (parseEnv(text).EVE_TOKEN_KEY) {
  console.log("EVE_TOKEN_KEY already exists; preserved.");
} else {
  const line = `EVE_TOKEN_KEY=${randomBytes(32).toString("base64")}`;
  const updated = /^EVE_TOKEN_KEY=.*$/m.test(text) ? text.replace(/^EVE_TOKEN_KEY=.*$/m, () => line) : `${text.trimEnd()}\n${line}\n`;
  writeFileSync(file, updated, { mode: 0o600 });
  console.log("EVE_TOKEN_KEY generated in .env; no secret printed.");
}
