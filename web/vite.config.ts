import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build output is embedded into the hub binary (internal/webui).
// In development, API calls are proxied to a hub running on :8080 (`make run`).
const hub = "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: "../internal/webui/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/api": hub,
      "/healthz": hub,
      "/readyz": hub,
    },
  },
});
