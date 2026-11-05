import { readFileSync } from "node:fs";
import { join } from "node:path";

// vite.config.ts copies this one file beside the bundle. If it ever imports a sibling, that sibling must be copied too or the map loses its layers.
describe("MapLibre worker", () => {
  it("is a single standalone file", () => {
    const source = readFileSync(join(__dirname, "../../node_modules/maplibre-gl/dist/maplibre-gl-worker.mjs"), "utf8");
    expect(source.match(/(?:from\s*|import\s*\(?\s*)["']\.{1,2}\/[^"']+["']/g)).toBeNull();
  });
});
