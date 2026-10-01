import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import "./components/ui/dialog.css";
import App from "./App.tsx";
import { BrowserRouter } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./lib/query";
import { initializeLocale } from "./lib/i18n";
import { ToastProvider } from "./components/ui/toast";

initializeLocale();

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

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ToastProvider>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </ToastProvider>
    </QueryClientProvider>
  </StrictMode>,
);
