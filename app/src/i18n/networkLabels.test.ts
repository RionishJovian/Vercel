import { describe, expect, it } from "vitest";
import { networkMy } from "./networkLabels";
import { currentNames, descriptions, schema, workspaceInfo } from "../pages/network/schema";

const texts = () => {
  const all = new Set<string>(["Automatic internet connections", "PPPoE internet connections", "Router routes"]);
  for (const s of Object.values(schema)) {
    all.add(s.title);
    for (const f of s.fields) {
      all.add(f[1]);
      if (f[2] === "select") for (const [, label] of f[3] as Array<[string, string]>) all.add(label);
    }
  }
  for (const [name, lead] of Object.values(workspaceInfo)) { all.add(name); all.add(lead); }
  for (const d of Object.values(descriptions)) all.add(d);
  for (const n of Object.values(currentNames)) all.add(n);
  return [...all];
};

describe("network form Burmese", () => {
  it("has an entry for every form text", () => {
    expect(texts().filter((s) => !networkMy[s])).toEqual([]);
  });
  it("has no stale entries for text that no longer exists", () => {
    const known = new Set(texts());
    expect(Object.keys(networkMy).filter((k) => !known.has(k))).toEqual([]);
  });
  it("keeps example values and limits exactly", () => {
    for (const [en, my] of Object.entries(networkMy)) {
      for (const sample of en.match(/\d[\d./]*\d[A-Za-z]?|\b\d+[hM]\b/g) ?? []) expect(my, en).toContain(sample);
    }
  });
});
