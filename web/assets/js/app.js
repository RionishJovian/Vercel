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

// Business states are not connectivity states. Never label a decided payment
// Unknown merely because it is not an up/stale/offline signal.
function businessStatusBadge(state, kind) {
  const states = kind === "payment" ? {
    approved: {text:"Approved", cls:"ok"}, rejected: {text:"Rejected", cls:"bad"},
    pending: {text:"Pending", cls:"warn"},
  } : {
    active: {text:"Active", cls:"ok"}, unused: {text:"Unused", cls:"ok"},
    used: {text:"Used", cls:"warn"}, expired: {text:"Expired", cls:"bad"},
    revoked: {text:"Revoked", cls:"bad"}, inactive: {text:"Inactive", cls:"unknown"},
  };
  const s = states[String(state || "").toLowerCase()] || {text:"Unknown", cls:"unknown"};
  return `<span class="status ${s.cls}">${s.text}</span>`;
}

function guestVoucherCapabilityMessage(result, canEdit) {
  if (!canEdit) return "Creating guest internet vouchers requires an owner or administrator and an enabled deployment. Sample layouts do not grant internet access.";
  if (result && result.failed) {
    if (result.status === 404) return "Guest voucher creation is not enabled on this deployment. Contact your administrator. Sample layouts do not grant internet access.";
    if (result.status === 403) return "You do not have permission to create guest vouchers. Contact your administrator.";
    return "Could not check guest voucher availability. Reload this page to retry. No codes can be issued while this check is unavailable.";
  }
  return "Guest voucher creation is not enabled. Contact your administrator; sample layouts do not grant internet access.";
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
