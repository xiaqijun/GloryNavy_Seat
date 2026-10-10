#!/usr/bin/env node

import { readFileSync } from "node:fs";

const input = process.argv[2]
  ? readFileSync(process.argv[2], "utf8")
  : readFileSync(0, "utf8");

const groups = new Map();
let ignored = 0;
for (const line of input.split(/\r?\n/)) {
  if (!line.trim()) continue;
  let row;
  try {
    row = JSON.parse(line);
  } catch {
    ignored += 1;
    continue;
  }
  if (row.msg !== "approval source timing" && row.msg !== "approval context timing" && row.msg !== "approval list timing") continue;
  if (!Number.isFinite(row.duration_ms)) {
    ignored += 1;
    continue;
  }
  const key = row.msg === "approval source timing"
    ? `${row.source ?? ""}\t${row.stage ?? ""}\t${row.indexed === true ? "indexed" : "legacy"}`
    : `${row.msg}\t${row.view ?? ""}\t${row.indexed === true ? "indexed" : row.indexed === false ? "legacy" : ""}`;
  const values = groups.get(key) ?? [];
  values.push(row.duration_ms);
  groups.set(key, values);
}

function percentile(values, p) {
  const index = Math.min(values.length - 1, Math.max(0, Math.ceil(values.length * p) - 1));
  return values[index];
}

const report = [...groups.entries()].sort(([a], [b]) => a.localeCompare(b)).map(([key, values]) => {
  values.sort((a, b) => a - b);
  const [sourceOrMessage, stageOrView, path] = key.split("\t");
  return {
    source: sourceOrMessage,
    stage_or_view: stageOrView,
    path: path || undefined,
    samples: values.length,
    p50_ms: percentile(values, 0.5),
    p95_ms: percentile(values, 0.95),
    max_ms: values.at(-1),
  };
});

process.stdout.write(`${JSON.stringify({groups: report, ignored_lines: ignored}, null, 2)}\n`);
