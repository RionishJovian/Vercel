import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import base from "./dict.json";
import extra from "./extra";

// Every t("key") used in the app must exist in both languages; otherwise the raw key shows on screen.
function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => { const p = join(dir, f); return statSync(p).isDirectory() ? sources(p) : /\.tsx?$/.test(f) && !f.endsWith(".test.ts") ? [p] : []; });
}
const en = new Set([...Object.keys(base.en), ...Object.keys(extra.en)]);
const my = new Set([...Object.keys(base.my), ...Object.keys(extra.my)]);

describe("i18n keys", () => {
  const used = new Set<string>();
  for (const f of sources(join(__dirname, ".."))) {
    const s = readFileSync(f, "utf8");
    for (const m of s.matchAll(/\bt\("([a-zA-Z0-9_.]+)"\)/g)) used.add(m[1]);
    for (const m of s.matchAll(/\bt\("([a-zA-Z0-9_.]+\.)" \+/g)) for (const k of en) if (k.startsWith(m[1])) used.add(k);
  }
  it("exist in English", () => { expect([...used].filter((k) => !en.has(k))).toEqual([]); });
  it("exist in Burmese", () => { expect([...used].filter((k) => !my.has(k))).toEqual([]); });
});
