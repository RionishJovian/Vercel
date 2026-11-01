import { describe, expect, it } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const p = join(dir, name);
    return statSync(p).isDirectory() ? sources(p) : p.endsWith(".tsx") ? [p] : [];
  });
}

/** Body of the function whose opening brace is at `open`, found by balancing braces. */
function bodyFrom(text: string, open: number): string {
  let depth = 0;
  for (let i = open; i < text.length; i++) {
    if (text[i] === "{") depth++;
    else if (text[i] === "}" && --depth === 0) return text.slice(open + 1, i);
  }
  return text.slice(open + 1);
}

// React nulls event.currentTarget after the handler yields, so reading it after an
// await throws (the Add RouterBOARD form saved the router and then showed an error).
describe("event handlers", () => {
  it("never read currentTarget after an await", () => {
    const bad: string[] = [];
    for (const file of sources(join(__dirname, ".."))) {
      const text = readFileSync(file, "utf8");
      for (const m of text.matchAll(/async function (\w+)\(([^)]*)\)[^{]*\{/g)) {
        const body = bodyFrom(text, m.index! + m[0].length - 1);
        const firstAwait = body.search(/\bawait\b/);
        const lastUse = body.lastIndexOf("e.currentTarget");
        if (firstAwait >= 0 && lastUse > firstAwait) bad.push(`${file.split("/src/")[1]}: ${m[1]}`);
      }
    }
    expect(bad).toEqual([]);
  });

  it("the check catches the original bug", () => {
    const src = "async function add(e) { e.preventDefault(); await save(); e.currentTarget.reset(); }";
    const body = bodyFrom(src, src.indexOf("{"));
    expect(body.search(/\bawait\b/) >= 0 && body.lastIndexOf("e.currentTarget") > body.search(/\bawait\b/)).toBe(true);
  });
});
