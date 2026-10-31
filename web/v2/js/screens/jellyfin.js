import { api } from "../api.js";
import { h, asArray } from "../dom.js";
import { t } from "../i18n.js";

const LEGACY = "/app.html";
const PARTS = ["dash", "vouchers", "guest"];

function nav(current) {
  return h("div", { class: "tabs-ws" },
    h("a", { href: "#/jellyfin/dash", "aria-current": current === "dash" ? "page" : null }, t("j.ws.dash")),
    h("a", { href: "#/jellyfin/vouchers", "aria-current": current === "vouchers" ? "page" : null }, t("j.ws.vouchers")),
    h("a", { href: "#/jellyfin/guest", "aria-current": current === "guest" ? "page" : null }, t("j.ws.guest")));
}


const num = (v) => (typeof v === "number" && Number.isFinite(v) ? v.toLocaleString("en-US") : "—");
const uuid = () => (crypto.randomUUID ? crypto.randomUUID() : "10000000-1000-4000-8000-100000000000".replace(/[018]/g, (c) => (c ^ (crypto.getRandomValues(new Uint8Array(1))[0] & (15 >> (c / 4)))).toString(16)));
const kv = (k, v) => h("div", { class: "row small" }, h("span", { class: "soft" }, k), h("b", { class: "mono" }, v));

// Queue one job and poll it until the owner's PC has answered (or ~60 s pass).
async function runJob(operation, payload) {
  const id = uuid();
  await api.post("/media/jobs", { id, operation, payload: payload || {} });
  for (let i = 0; i < 30; i++) {
    await new Promise((r) => setTimeout(r, 2000));
    const j = await api.get("/media/jobs/" + id);
    if (j.state === "succeeded") return j.result || {};
    if (["failed", "cancelled", "expired"].includes(j.state)) throw new Error(j.error_code || j.state);
  }
  throw new Error("timeout");
}

async function dashboard() {
  const box = h("div", { class: "stack" });
  const conn = h("section", { class: "glass" });
  const live = h("section", { class: "glass" });
  box.append(conn, live);
  const msg = (el, key, cls) => { el.replaceChildren(h("p", { class: "note " + (cls || "") , role: cls === "bad" ? "alert" : null }, t(key))); };

  async function drawLive() {
    live.replaceChildren(h("h2", { class: "num" }, t("j.status")), h("p", { class: "soft" }, t("j.checking")));
    try {
      const [dash, libs] = await Promise.all([runJob("dashboard"), runJob("libraries")]);
      const d = dash.dashboard || {};
      const c = d.counts || {};
      const cap = h("div", { class: "stack" });
      cap.append(
        kv(t("j.server"), ((d.server && d.server.ServerName) || "—") + (d.server && d.server.Version ? " " + d.server.Version : "")),
        kv(t("j.watching"), num(d.active_sessions)),
        kv(t("j.movies"), num(c.MovieCount === undefined ? c.movies : c.MovieCount)),
        kv(t("j.series"), num(c.SeriesCount === undefined ? c.series : c.SeriesCount)));
      const list = asArray(libs.libraries);
      const libBox = h("div", { class: "stack" }, h("h3", null, t("j.libraries")));
      if (!list.length) libBox.append(h("p", { class: "soft" }, t("j.noLibraries")));
      for (const l of list) {
        libBox.append(h("div", { class: "row" }, h("span", null, l.Name || "—", " ", h("span", { class: "chip" }, l.CollectionType || "—")),
          h("span", { class: "soft small" }, asArray(l.Locations)[0] || "")));
      }
      const scan = h("button", { class: "pill", type: "button" }, t("j.scan"));
      scan.addEventListener("click", async () => {
        if (!window.confirm(t("j.scanConfirm"))) return;
        scan.disabled = true;
        try { await runJob("scan", { confirm: true }); scan.textContent = t("j.scanned"); }
        catch (_) { scan.textContent = t("j.failed"); scan.disabled = false; }
      });
      live.replaceChildren(h("h2", { class: "num" }, t("j.status")), cap, libBox, scan);
    } catch (e) {
      if (e && e.message === "unauthorized") throw e;
      const retry = h("button", { class: "pill", type: "button", onclick: drawLive }, t("c.retry"));
      live.replaceChildren(h("h2", { class: "num" }, t("j.status")), h("p", { class: "note warn", role: "alert" }, t("j.noAnswer")), retry);
    }
  }

  async function drawConn() {
    let c = null;
    try { c = await api.get("/media/connection"); } catch (e) {
      if (e && e.message === "unauthorized") throw e;
      if (e && e.status === 403) { conn.replaceChildren(h("p", { class: "note warn" }, t("j.locked"))); live.remove(); return; }
    }
    const linked = c && c.server_name && !c.revoked_at;
    const head = h("div", { class: "row" }, h("h2", { class: "num" }, t("j.link")), h("span", { class: "chip " + (linked ? "ok" : "warn") }, linked ? t("j.linked") : t("j.notLinked")));
    const kids = [head];
    if (linked) {
      kids.push(kv(t("j.server"), c.server_name), kv(t("j.lastSeen"), c.last_seen_at ? new Date(c.last_seen_at).toLocaleString() : "—"));
      const rev = h("button", { class: "pill danger", type: "button" }, t("j.unlink"));
      rev.addEventListener("click", async () => {
        if (!window.confirm(t("j.unlinkConfirm"))) return;
        try { await api.del("/media/connection"); } catch (_) { /* shown on redraw */ }
        drawConn();
      });
      kids.push(rev);
    } else {
      kids.push(h("p", { class: "soft" }, t("j.pairHelp")));
      const code = h("div", { class: "big mono", "aria-live": "polite" });
      const pair = h("button", { class: "pill on", type: "button" }, t("j.pair"));
      pair.addEventListener("click", async () => {
        pair.disabled = true;
        try {
          const r = await api.post("/media/pairing", {});
          code.textContent = r.pairing_code;
          pair.textContent = t("j.pairWait");
          let tries = 0;
          const timer = setInterval(async () => {
            tries++;
            try { const n = await api.get("/media/connection"); if (n && n.server_name && !n.revoked_at) { clearInterval(timer); drawConn(); drawLive(); } } catch (_) { /* keep waiting */ }
            if (tries > 100 || !document.body.contains(conn)) clearInterval(timer);
          }, 6000);
        } catch (_) { pair.disabled = false; code.textContent = ""; code.append(h("span", { class: "note bad", role: "alert" }, t("j.failed"))); }
      });
      kids.push(pair, code, h("p", { class: "soft small" }, t("j.pairExpiry")));
    }
    conn.replaceChildren(...kids);
    if (linked) drawLive(); else live.replaceChildren(h("h2", { class: "num" }, t("j.status")), h("p", { class: "soft" }, t("j.needLink")));
  }
  await drawConn();
  return box;
}

export default {
  titleKey: "title.jellyfin",
  tab: "jellyfin",
  async render(ctx) {
    const part = PARTS.includes(ctx.sub) ? ctx.sub : "dash";
    let body;
    if (part === "vouchers") {
      // Honest state: no Jellyfin voucher backend exists yet.
      body = h("section", { class: "glass" }, h("h2", { class: "num" }, t("j.notReady")), h("p", { class: "soft" }, t("j.notReadyBody")));
    } else if (part === "guest") {
      body = h("section", { class: "glass" },
        h("h2", { class: "num" }, t("j.ws.guest")),
        h("p", { class: "soft" }, t("j.dashHelp")),
        h("a", { class: "pill on", href: LEGACY + "#jellyfin-portal" }, t("c.openLegacy")));
    } else {
      body = await dashboard();
    }
    return h("div", { class: "stack" }, nav(part), body);
  },
};
