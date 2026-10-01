import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";
import { localInteractionDiagnostics } from "./tooling/interaction-diagnostics.ts";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss(), localInteractionDiagnostics()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  preview: {
    proxy: { "/api": process.env.API_PROXY_TARGET ?? "http://127.0.0.1:8080" },
  },
  server: {
    host: "127.0.0.1",
    port: 5173,
    strictPort: true,
    proxy: { "/api": process.env.API_PROXY_TARGET ?? "http://127.0.0.1:8080" },
  },
});
