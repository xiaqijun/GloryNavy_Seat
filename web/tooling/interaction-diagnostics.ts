import type { IncomingMessage, ServerResponse } from "node:http";
import type { Plugin } from "vite";

const pages = new Set([
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
  "/login",
  "other",
]);
const kinds = new Set([
  "heartbeat",
  "ready",
  "navigation",
  "orphan_pointer_lock",
  "main_thread_stall",
  "render_loop",
  "script_error",
  "unhandled_rejection",
]);
type Entry = {
  kind: string;
  page: string;
  source: string;
  browser: string;
  tab: string;
  duration_ms?: number;
  at: string;
  visible?: boolean;
  body_blocked?: boolean;
  navigation_hit?: boolean;
  overlays?: number;
};

export function interactionMiddleware() {
  const entries: Entry[] = [];
  return async (req: IncomingMessage, res: ServerResponse) => {
    res.setHeader("Cache-Control", "no-store");
    const reject = (code: number) => {
      res.statusCode = code;
      res.end();
    };
    const address = req.socket.remoteAddress ?? "";
    if (!["127.0.0.1", "::1", "::ffff:127.0.0.1"].includes(address))
      return reject(403);
    const host = req.headers.host ?? "";
    if (!/^(?:127\.0\.0\.1|localhost|\[::1\]):\d+$/.test(host))
      return reject(403);
    const origin = req.headers.origin;
    if (origin && origin !== `http://${host}`) return reject(403);
    if (
      req.headers["sec-fetch-site"] &&
      !["same-origin", "none"].includes(String(req.headers["sec-fetch-site"]))
    )
      return reject(403);
    if (req.method === "GET") {
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify({ entries }));
      return;
    }
    if (req.method !== "POST") return reject(405);
    if (
      origin !== `http://${host}` ||
      req.headers["content-type"] !== "application/json"
    )
      return reject(403);
    if (Number(req.headers["content-length"] ?? 0) > 1024) return reject(413);
    try {
      let body = "";
      for await (const chunk of req) {
        body += chunk.toString();
        if (Buffer.byteLength(body) > 1024) return reject(413);
      }
      const value = JSON.parse(body);
      if (
        !value ||
        !kinds.has(value.kind) ||
        !pages.has(value.page) ||
        !["main", "worker"].includes(value.source) ||
        !["edge", "chromium", "other"].includes(value.browser) ||
        typeof value.tab !== "string" ||
        !/^[a-f0-9-]{36}$/.test(value.tab) ||
        (value.duration_ms !== undefined &&
          (!Number.isFinite(value.duration_ms) ||
            value.duration_ms < 0 ||
            value.duration_ms > 86400000))
      )
        return reject(400);
      for (const key of ["visible", "body_blocked", "navigation_hit"]) {
        if (value[key] !== undefined && typeof value[key] !== "boolean")
          return reject(400);
      }
      if (
        value.overlays !== undefined &&
        (!Number.isInteger(value.overlays) ||
          value.overlays < 0 ||
          value.overlays > 100)
      )
        return reject(400);
      if (value.kind === "heartbeat") {
        const old = entries.findIndex(
          (e) =>
            e.kind === "heartbeat" &&
            e.tab === value.tab &&
            e.source === value.source,
        );
        if (old >= 0) entries.splice(old, 1);
      }
      // Explicit projection prevents extra request fields from entering diagnostic output.
      entries.push({
        kind: value.kind,
        page: value.page,
        source: value.source,
        browser: value.browser,
        tab: value.tab,
        ...(value.visible === undefined ? {} : { visible: value.visible }),
        ...(value.body_blocked === undefined
          ? {}
          : { body_blocked: value.body_blocked }),
        ...(value.navigation_hit === undefined
          ? {}
          : { navigation_hit: value.navigation_hit }),
        ...(value.overlays === undefined ? {} : { overlays: value.overlays }),
        ...(value.duration_ms === undefined
          ? {}
          : { duration_ms: Math.round(value.duration_ms) }),
        at: new Date().toISOString(),
      });
      if (entries.length > 80) entries.splice(0, entries.length - 80);
      res.statusCode = 204;
      res.end();
    } catch {
      reject(400);
    }
  };
}

export function localInteractionDiagnostics(): Plugin {
  return {
    name: "local-interaction-diagnostics",
    apply: "serve",
    configureServer(server) {
      const handler = interactionMiddleware();
      server.middlewares.use((req, res, next) => {
        if (req.url !== "/__debug/interaction") return next();
        void handler(req, res);
      });
    },
  };
}
