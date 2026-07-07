/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev server proxies /api and /health to the Go API so the browser sees one origin.
// The `test` block configures vitest (node env — the deadline state machine is a
// pure function, no DOM).
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
      "/health": "http://localhost:8080",
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
