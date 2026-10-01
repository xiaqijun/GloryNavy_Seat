// A separate thread can report a stall even while the page thread cannot run.
import {
  reportInteraction,
  type DiagnosticIdentity,
} from "./interaction-report";
let identity: DiagnosticIdentity | undefined;
let lastBeat = 0;
let visible = false;
let page = "other";
let reported = false;
let lastReport = 0;
self.onmessage = (
  event: MessageEvent<{
    visible: boolean;
    page: string;
    identity: DiagnosticIdentity;
  }>,
) => {
  if (!identity)
    reportInteraction({
      ...event.data.identity,
      kind: "ready",
      page: event.data.page,
      source: "worker",
    });
  identity = event.data.identity;
  lastBeat = performance.now();
  visible = event.data.visible;
  page = event.data.page;
  reported = false;
};
setInterval(() => {
  const duration_ms = Math.round(performance.now() - lastBeat);
  if (identity && performance.now() - lastReport > 5000) {
    lastReport = performance.now();
    reportInteraction({
      ...identity,
      kind: "heartbeat",
      page,
      source: "worker",
      visible,
      duration_ms: Math.min(duration_ms, 86400000),
    });
  }
  if (!visible || !lastBeat || reported || duration_ms < 5000) return;
  reported = true;
  const evidence = { kind: "main_thread_stall", page, duration_ms };
  console.warn("[GloryNavy interaction watchdog]", evidence);
  if (identity)
    reportInteraction({ ...identity, ...evidence, source: "worker" });
  self.postMessage(evidence);
}, 1000);
