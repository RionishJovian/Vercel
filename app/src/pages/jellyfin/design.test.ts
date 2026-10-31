import { normalize, guestDocument } from "./design";

describe("guest page design", () => {
  it("keeps only data-URI pictures, never remote or SVG sources", () => {
    const d = normalize({ background: "https://evil.example/x.png", tiles: { movies: { image: "data:image/svg+xml;base64,AAAA" } }, posters: { 0: { image: "data:image/png;base64,iVBORw0KGgo=" } } });
    expect(d.background).toBe("");
    expect(d.tiles.movies.image).toBe("");
    expect(d.posters[0].image).toBe("data:image/png;base64,iVBORw0KGgo=");
  });
  it("falls back to safe values for a bad accent, theme or layout", () => {
    expect(normalize({ accent: "red;}</style>", theme: "x", layout: "y" })).toMatchObject({ accent: "#eeeeee", theme: "charcoal", layout: "shelves" });
  });
  it("accepts only library IDs in the expected shapes and at most 16", () => {
    const ok = Array.from({ length: 20 }, (_, i) => i.toString(16).padStart(32, "0"));
    expect(normalize({ libraries: ["nope", ...ok] }).libraries).toHaveLength(16);
  });
  it("escapes owner text in the preview document", () => {
    const html = guestDocument({ name: "<script>alert(1)</script>" });
    expect(html).not.toContain("<script>alert(1)</script>");
    expect(html).toContain("&lt;script&gt;");
  });
});
