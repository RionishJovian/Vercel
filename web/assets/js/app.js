/* Shared UI helpers + session — used by the app pages. */
"use strict";

const stateLabel = {
  up:      { text: "Up",      cls: "ok" },
  stale:   { text: "Stale",   cls: "warn" },
  offline: { text: "Offline", cls: "bad" },
  unknown: { text: "Unknown", cls: "unknown" },
};

function badge(state) {
  const s = stateLabel[state] || stateLabel.unknown;
  return `<span class="status ${s.cls}">${s.text}</span>`;
}

function esc(s) {
  return String(s ?? "")
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function ts(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  try { return d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }); }
  catch (_) { return esc(iso); }
}

async function requireMe() {
  return await MM.get("/me");
}

async function whoami() {
  try { return await requireMe(); }
  catch (_) { location.href = "/signin.html"; return null; }
}