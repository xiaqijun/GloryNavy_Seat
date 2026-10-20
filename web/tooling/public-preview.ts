import type { Plugin } from "vite";

// Match the edge's public-home / private-app split when testing built artifacts.
export function publicPreview(): Plugin {
  return {
    name: "public-home-preview",
    configurePreviewServer(server) {
      server.middlewares.use((request, _response, next) => {
        const url = new URL(request.url ?? "/", "http://localhost");
        if (
          url.pathname !== "/" &&
          !url.pathname.startsWith("/api/") &&
          !/\.[^/]+$/.test(url.pathname) &&
          request.headers.accept?.includes("text/html")
        )
          request.url = `/app.html${url.search}`;
        next();
      });
    },
  };
}
