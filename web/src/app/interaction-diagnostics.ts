// Development-only evidence collection. Never changes focus, locks or UI state.
import {
  diagnosticPages as pages,
  reportInteraction,
  type DiagnosticIdentity,
} from "./interaction-report";
const storageKey = "gnv:interaction-diagnostics";
const prefix = "[GloryNavy interaction]";
type Evidence = {
  kind: string;
  page: string;
  at: string;
  duration_ms?: number;
};

export function startInteractionDiagnostics() {
  const identity: DiagnosticIdentity = {
    tab: crypto.randomUUID(),
    browser: /Edg\//.test(navigator.userAgent)
      ? "edge"
      : /Chrome\//.test(navigator.userAgent)
        ? "chromium"
        : "other",
  };
  const pageName = () =>
    pages.has(location.pathname) ? location.pathname : "other";
  let history: Evidence[] = [];
  try {
    const previous: unknown = JSON.parse(
      sessionStorage.getItem(storageKey) ?? "[]",
    );
    if (Array.isArray(previous))
      history = previous
        .slice(-40)
        .filter(
          (e) =>
            e &&
            [
              "orphan_pointer_lock",
              "main_thread_stall",
              "render_loop",
              "script_error",
              "unhandled_rejection",
            ].includes(e.kind) &&
            (pages.has(e.page) || e.page === "other") &&
            typeof e.at === "string",
        )
        .map((e) => ({
          kind: e.kind,
          page: e.page,
          at: e.at,
          ...(typeof e.duration_ms === "number"
            ? { duration_ms: e.duration_ms }
            : {}),
        }));
  } catch {
    /* Storage can be disabled; console evidence remains available. */
  }
  if (history.length) console.info(prefix, "previous", history.slice(-5));
  const record = (kind: string, duration_ms?: number, publish = true) => {
    const entry: Evidence = {
      kind,
      page: pageName(),
      at: new Date().toISOString(),
      ...(duration_ms === undefined
        ? {}
        : { duration_ms: Math.round(duration_ms) }),
    };
    history = [...history, entry].slice(-40);
    try {
      sessionStorage.setItem(storageKey, JSON.stringify(history));
    } catch {
      /* Best effort only. */
    }
    console.warn(prefix, entry);
    if (publish) reportInteraction({ ...identity, ...entry, source: "main" });
  };
  let lockSince = 0;
  let lastReport = 0;
  let reportedLock = false;
  const worker = new Worker(
    new URL("./interaction-watchdog.worker.ts", import.meta.url),
    { type: "module" },
  );
  worker.onmessage = (
    event: MessageEvent<{ kind: string; duration_ms: number }>,
  ) => {
    if (event.data.kind === "main_thread_stall")
      record(event.data.kind, event.data.duration_ms, false);
  };
  const heartbeat = () => {
    const visible = document.visibilityState === "visible";
    worker.postMessage({ visible, page: pageName(), identity });
    // An open modal/select intentionally disables outside pointer events.
    // Observe only locks with no visible modal surface; never force-unlock them.
    const surfaces = [
      ...document.querySelectorAll(
        '[role="dialog"], [role="alertdialog"], [role="listbox"]',
      ),
    ].filter(
      (element) =>
        element.getClientRects().length > 0 &&
        getComputedStyle(element).visibility !== "hidden",
    );
    const blocked = getComputedStyle(document.body).pointerEvents === "none";
    if (performance.now() - lastReport > 5000) {
      lastReport = performance.now();
      const link = document.querySelector(
        'nav[aria-label="主导航"] a[aria-current="page"]',
      );
      const bounds = link?.getBoundingClientRect();
      const hit = bounds
        ? document.elementFromPoint(
            bounds.x + bounds.width / 2,
            bounds.y + bounds.height / 2,
          )
        : null;
      reportInteraction({
        ...identity,
        kind: "heartbeat",
        page: pageName(),
        source: "main",
        visible,
        body_blocked: blocked,
        overlays: Math.min(surfaces.length, 100),
        ...(link && bounds && bounds.width > 0
          ? { navigation_hit: !!hit && link.contains(hit) }
          : {}),
      });
    }
    const orphan = visible && blocked && !surfaces.length;
    if (!orphan) {
      lockSince = 0;
      reportedLock = false;
      return;
    }
    if (!lockSince) lockSince = performance.now();
    if (!reportedLock && performance.now() - lockSince >= 2000) {
      reportedLock = true;
      record("orphan_pointer_lock");
    }
  };
  const onError = (event: ErrorEvent) =>
    record(
      /Maximum update depth|Too many re-renders/i.test(event.message)
        ? "render_loop"
        : "script_error",
    );
  const onRejection = () => record("unhandled_rejection");
  const onNavigation = (event: MouseEvent) => {
    if (
      event.button !== 0 ||
      event.ctrlKey ||
      event.metaKey ||
      event.altKey ||
      event.shiftKey
    )
      return;
    const link =
      event.target instanceof Element
        ? event.target.closest('nav[aria-label="主导航"] a[href]')
        : null;
    if (!(link instanceof HTMLAnchorElement)) return;
    const url = new URL(link.href);
    if (url.origin === location.origin && pages.has(url.pathname)) {
      reportInteraction({
        ...identity,
        kind: "navigation",
        page: url.pathname,
        source: "main",
      });
    }
  };
  const timer = window.setInterval(heartbeat, 1000);
  window.addEventListener("error", onError);
  window.addEventListener("unhandledrejection", onRejection);
  document.addEventListener("visibilitychange", heartbeat);
  document.addEventListener("click", onNavigation, true);
  reportInteraction({
    ...identity,
    kind: "ready",
    page: pageName(),
    source: "main",
  });
  heartbeat();
  return () => {
    clearInterval(timer);
    worker.terminate();
    window.removeEventListener("error", onError);
    window.removeEventListener("unhandledrejection", onRejection);
    document.removeEventListener("visibilitychange", heartbeat);
    document.removeEventListener("click", onNavigation, true);
  };
}
