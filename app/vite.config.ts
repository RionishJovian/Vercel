import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

// MapLibre 6 starts its worker from ./maplibre-gl-worker.mjs beside the running script, and bundling does not emit that file.
// Without it the map draws no vector tiles or GeoJSON layers.
const mapLibreWorker = () => ({
  name: "emit-maplibre-worker",
  generateBundle(this: { emitFile(file: { type: "asset"; fileName: string; source: string }): string }) {
    this.emitFile({ type: "asset", fileName: "assets/maplibre-gl-worker.mjs", source: readFileSync(new URL("./node_modules/maplibre-gl/dist/maplibre-gl-worker.mjs", import.meta.url), "utf8") });
  },
});

// The console is served at /app/ (old /v2/ links are redirected there by the web server).
export default defineConfig({
  base: "/app/",
  plugins: [react(), mapLibreWorker()],
  // satellite.js also ships optional WebAssembly builds that use top-level await and Node modules. Only its plain JavaScript SGP4 is used here.
  resolve: { alias: [{ find: /^#wasm-(single|multi)-thread$/, replacement: fileURLToPath(new URL("./src/lib/noWasm.ts", import.meta.url)) }] },
  // Pre-bundling would move MapLibre away from its worker file, and would pull in the WebAssembly above.
  optimizeDeps: { exclude: ["satellite.js", "maplibre-gl"] },
  build: { outDir: "dist", sourcemap: false, target: "es2022", assetsInlineLimit: 2048, chunkSizeWarningLimit: 1200 /* the map library is one lazy chunk, loaded only on Sites */ },
  server: { proxy: { "/api": "http://localhost:8080" } },
  test: { environment: "jsdom", globals: true },
});
