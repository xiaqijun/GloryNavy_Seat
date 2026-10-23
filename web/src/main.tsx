import { StrictMode } from "react";
import { createRoot, hydrateRoot } from "react-dom/client";
import "./index.css";
import "./components/ui/dialog.css";
import App from "./App.tsx";
import { BrowserRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./lib/query";
import { getLocale, initializeLocale } from "./lib/i18n";
import { ToastProvider } from "./components/ui/toast";
import { initializeTheme } from "./lib/theme";

initializeLocale();
const stopTheme = initializeTheme();
import.meta.hot?.dispose(stopTheme);

if (import.meta.env.DEV) {
  let disposed = false;
  let stopDiagnostics: (() => void) | undefined;
  import.meta.hot?.dispose(() => {
    disposed = true;
    stopDiagnostics?.();
  });
  void import("./app/interaction-diagnostics")
    .then(({ startInteractionDiagnostics }) => {
      if (!disposed) stopDiagnostics = startInteractionDiagnostics();
    })
    .catch(() => {
      console.warn("[GloryNavy interaction] diagnostics unavailable");
    });
}

const application = (
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  </StrictMode>
);
const root = document.getElementById("root")!;
// The static snapshot is anonymous Chinese content. Other locales and OAuth
// error responses mount normally rather than hydrating different initial text.
if (
  window.location.pathname === "/" &&
  root.dataset.prerendered === getLocale() &&
  !new URLSearchParams(window.location.search).has("error")
) {
  hydrateRoot(root, application);
} else {
  createRoot(root).render(application);
}
