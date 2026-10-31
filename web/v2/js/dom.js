// Tiny DOM helpers. Text is always inserted as text nodes (never innerHTML), so
// server data cannot inject markup.
const SVG_NS = "http://www.w3.org/2000/svg";

function append(el, kids) {
  for (const k of kids.flat(Infinity)) {
    if (k === null || k === undefined || k === false) continue;
    el.append(k.nodeType ? k : document.createTextNode(String(k)));
  }
}

export function h(tag, attrs, ...kids) {
  const el = document.createElement(tag);
  if (attrs) {
    for (const [k, v] of Object.entries(attrs)) {
      if (v === null || v === undefined || v === false) continue;
      if (k === "class") el.className = v;
      else if (k.startsWith("on") && typeof v === "function") el.addEventListener(k.slice(2).toLowerCase(), v);
      else if (v === true) el.setAttribute(k, "");
      else el.setAttribute(k, String(v));
    }
  }
  append(el, kids);
  return el;
}

const ICONS = {
  home: "M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z",
  router: "M3 14h18v6H3zM7 17h.01M11 17h.01M8 14V8M16 14V8M6 5l2 3M18 5l-2 3",
  ticket: "M3 9a2 2 0 0 0 0 6v3a1 1 0 0 0 1 1h16a1 1 0 0 0 1-1v-3a2 2 0 0 1 0-6V6a1 1 0 0 0-1-1H4a1 1 0 0 0-1 1zM14 5v14",
  play: "M3 4h18v16H3zM10 9l5 3-5 3z",
  dots: "M5 12h.01M12 12h.01M19 12h.01",
  bell: "M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9M10 21a2 2 0 0 0 4 0",
};

export function icon(name) {
  const s = document.createElementNS(SVG_NS, "svg");
  s.setAttribute("viewBox", "0 0 24 24");
  s.setAttribute("aria-hidden", "true");
  s.setAttribute("class", "ico");
  const p = document.createElementNS(SVG_NS, "path");
  p.setAttribute("d", ICONS[name] || "");
  s.append(p);
  return s;
}

export function clear(el) { while (el.firstChild) el.removeChild(el.firstChild); }
export function mmk(n) { return Number(n || 0).toLocaleString("en-US"); }
export function asArray(v) { return Array.isArray(v) ? v : []; }
