// Fixed diagnostic metadata only. This module is loaded by development code.
export const diagnosticPath = "/__debug/interaction";
export const diagnosticPages = new Set([
  "/",
  "/system",
  "/attendance",
  "/exchange",
  "/fittings",
  "/skills",
  "/welfare",
  "/losses",
  "/contracts",
  "/sync",
  "/members",
  "/access",
  "/account",
  "/",
  "other",
]);
export const diagnosticKinds = new Set([
  "ready",
  "navigation",
  "orphan_pointer_lock",
  "main_thread_stall",
  "render_loop",
  "script_error",
  "unhandled_rejection",
]);
export type DiagnosticIdentity = {
  tab: string;
  browser: "edge" | "chromium" | "other";
};
export type DiagnosticReport = DiagnosticIdentity & {
  kind: string;
  page: string;
  source: "main" | "worker";
  duration_ms?: number;
  visible?: boolean;
  body_blocked?: boolean;
  navigation_hit?: boolean;
  overlays?: number;
};

export function reportInteraction(report: DiagnosticReport) {
  // No remote deployments, cookies, business payloads or raw URLs.
  if (
    !import.meta.env.DEV ||
    !["127.0.0.1", "localhost", "[::1]"].includes(location.hostname)
  )
    return;
  void fetch(diagnosticPath, {
    method: "POST",
    credentials: "omit",
    mode: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(report),
    signal: AbortSignal.timeout(1500),
  }).catch(() => {});
}
