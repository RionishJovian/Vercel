import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The app is served under /v2/ until it replaces the old frontend.
export default defineConfig({
  base: "/v2/",
  plugins: [react()],
  build: { outDir: "dist", sourcemap: false, target: "es2022", assetsInlineLimit: 2048, chunkSizeWarningLimit: 1200 /* the map library is one lazy chunk, loaded only on Sites */ },
  server: { proxy: { "/api": "http://localhost:8080" } },
  test: { environment: "jsdom", globals: true },
});
