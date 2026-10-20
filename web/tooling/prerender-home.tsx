import { StrictMode } from "react";
import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToastProvider } from "../src/components/ui/toast";
import App from "../src/App";
export {
  pageHeadItems,
  publicSiteMetadata,
  publicSiteURL,
} from "../src/lib/public-site";

export function renderHome() {
  // An empty, per-build cache prevents sessions or live statistics entering HTML.
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  try {
    return renderToString(
      <StrictMode>
        <QueryClientProvider client={client}>
          <ToastProvider>
            <MemoryRouter initialEntries={["/"]}>
              <App />
            </MemoryRouter>
          </ToastProvider>
        </QueryClientProvider>
      </StrictMode>,
    );
  } finally {
    client.clear();
  }
}
