import { errorText } from "../../lib/errors";
import { useState, type FormEvent } from "react";
import { ApiError, api, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import type { PortInfo, TNode } from "./project";

interface Review { steps: string[]; warnings: string[]; confirmation: string }
interface Context { routerId: string; routerName: string; locked: boolean; guestLocked: boolean; onChanged: () => void; onPending: (revertAt: string) => void }

const KINDS = ["other", "switch", "ap", "computer", "printer", "controller"];

/** Review, consent, apply: the same gate as every other router change. The server re-checks the plan on apply. */
function Reviewed({ ctx, kind, values, label, done, danger, disabled }: { ctx: Context; kind: string; values: Record<string, string>; label: string; done: string; danger?: boolean; disabled?: boolean }) {
  const { t } = useI18n();
  const [review, setReview] = useState<Review | null>(null), [consent, setConsent] = useState(false), [busy, setBusy] = useState(false), [msg, setMsg] = useState<{ bad: boolean; text: string } | null>(null);
  const path = (action: string) => `/routers/${encodeURIComponent(ctx.routerId)}/network/${action}`;
  async function preview() {
    setBusy(true); setMsg(null); setReview(null); setConsent(false);
    try { const r = await api.post<{ review?: Review }>(path("preview"), { kind, id: "", values }); setReview(r.review ?? null); }
    catch (e) { if (isUnauthorized(e)) throw e; setMsg({ bad: true, text: errorText(e, t("dc.failed")) }); }
    setBusy(false);
  }
  async function apply() {
    if (!review) return;
    setBusy(true); setMsg(null);
    try {
      const r = await api.post<{ result?: { verified?: boolean; pending_confirm?: boolean; revert_at?: string } }>(path("apply"), { kind, id: "", values, confirmation: review.confirmation });
      if (!r.result?.verified) throw new ApiError(t("dc.notVerified"));
      setReview(null); setConsent(false);
      if (r.result.pending_confirm && r.result.revert_at) ctx.onPending(r.result.revert_at); else setMsg({ bad: false, text: done });
      ctx.onChanged();
    } catch (e) { if (isUnauthorized(e)) throw e; setReview(null); setMsg({ bad: true, text: errorText(e, t("dc.failed")) }); }
    setBusy(false);
  }
  return (
    <div className="stack">
      {!review && <button className={"pill" + (danger ? " danger" : "")} type="button" disabled={busy || ctx.locked || disabled} onClick={() => void preview()}>{label}</button>}
      {review && (
        <div className="note warn stack" role="region" aria-label={t("net.reviewTitle")}>
          <b>{t("net.reviewTitle")}</b>
          <p className="soft small">{t("net.router")}: <b>{ctx.routerName}</b></p>
          <ul className="steps">{review.steps.map((x, i) => <li key={i}>{x}</li>)}</ul>
          {review.warnings.map((w, i) => <p className="small" key={i}>{w}</p>)}
          <label className="check"><span>{t("net.consent")}</span><input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} /></label>
          <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
            <button className="pill" type="button" disabled={busy} onClick={() => setReview(null)}>{t("c.cancel")}</button>
            <button className={"pill" + (danger ? " danger" : " on")} type="button" disabled={!consent || busy} onClick={() => void apply()}>{t("net.apply")}</button>
          </div>
        </div>
      )}
      {msg && <Note kind={msg.bad ? "bad" : "ok"}>{msg.text}</Note>}
    </div>
  );
}

function RenameForm({ node, routerId, onChanged }: { node: TNode; routerId: string; onChanged: () => void }) {
  const { t } = useI18n();
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), name = String(f.get("name")).trim(), kind = String(f.get("kind"));
    setBusy(true); setMsg(t("dv.saving"));
    try {
      const saved = await api.put<{ labels?: Array<{ mac: string; name: string; kind: string }> }>(`/routers/${encodeURIComponent(routerId)}/discovery-labels`, { mac: node.mac, name, kind });
      if (!saved.labels?.some((l) => l.mac === node.mac && l.name === name && l.kind === kind)) throw new Error("unconfirmed");
      setMsg(t("dv.saved")); onChanged();
    } catch (err) { if (isUnauthorized(err)) throw err; setMsg(t("dv.saveFail")); }
    setBusy(false);
  }
  return (
    <form className="stack" onSubmit={save} key={node.id}>
      <label className="field"><span>{t("dv.name")}</span><input className="input" name="name" maxLength={120} required defaultValue={node.name === "Unknown device" ? "" : node.name} /></label>
      <label className="field"><span>{t("dv.type")}</span><select className="input" name="kind" defaultValue={KINDS.includes(node.kind) ? node.kind : "other"}>{KINDS.map((k) => <option key={k} value={k}>{t("dv.k." + k)}</option>)}</select></label>
      <button className="pill" type="submit" disabled={busy}>{t("dv.save")}</button>
      {msg && <span className="soft small" role="status">{msg}</span>}
    </form>
  );
}

/** The guest actions are explicit one-step changes: the owner confirms here and the server reads the result back. */
function GuestAction({ ctx, path, body, label, confirm, done, danger }: { ctx: Context; path: string; body: Record<string, unknown>; label: string; confirm: string; done: string; danger?: boolean }) {
  const { t } = useI18n();
  const [confirming, setConfirming] = useState(false), [busy, setBusy] = useState(false), [msg, setMsg] = useState<{ bad: boolean; text: string } | null>(null);
  async function run() {
    setBusy(true); setMsg(null);
    try {
      const r = await api.post<{ verified?: boolean }>(`/routers/${encodeURIComponent(ctx.routerId)}/guests/${path}`, { ...body, confirm: true });
      if (r.verified !== true) throw new ApiError(t("dc.notVerified"));
      setMsg({ bad: false, text: done }); ctx.onChanged();
    } catch (e) { if (isUnauthorized(e)) throw e; setMsg({ bad: true, text: errorText(e, t("dc.failed")) }); }
    setConfirming(false); setBusy(false);
  }
  const style = "pill" + (danger ? " danger" : "");
  return (
    <div className="stack">
      {confirming ? (
        <div className="note warn stack"><p className="small">{confirm}</p><div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><button className="pill" type="button" disabled={busy} onClick={() => setConfirming(false)}>{t("c.cancel")}</button><button className={style} type="button" disabled={busy} onClick={() => void run()}>{label}</button></div></div>
      ) : <button className={style} type="button" disabled={ctx.guestLocked} onClick={() => setConfirming(true)}>{label}</button>}
      {msg && <Note kind={msg.bad ? "bad" : "ok"}>{msg.text}</Note>}
    </div>
  );
}

function PortCard({ port, ctx }: { port: PortInfo; ctx: Context }) {
  const { t } = useI18n();
  const [label, setLabel] = useState(port.label);
  const state = port.disabled ? t("r.off") : port.running ? t("c.online") : t("r.down");
  return (
    <details className="rowitem stack">
      <summary><b>{port.name}</b> <span className={"chip " + (port.disabled ? "bad" : port.running ? "ok" : "warn")}>{state}</span>{port.label && <span className="soft small"> {port.label}</span>}</summary>
      <label className="field"><span>{t("dc.portLabel")}</span><input className="input" maxLength={64} value={label} disabled={ctx.locked} onChange={(e) => setLabel(e.target.value)} /></label>
      <Reviewed ctx={ctx} kind="port_label" values={{ interface: port.name, comment: label }} label={t("dc.portLabelSave")} done={t("dc.portLabelDone")} disabled={label.trim() === port.label} />
      <Reviewed ctx={ctx} kind="port_state" values={{ interface: port.name, disabled: port.disabled ? "no" : "yes" }} label={port.disabled ? t("dc.portOn") : t("dc.portOff")} done={t("dc.portStateDone")} danger={!port.disabled} />
      {port.poe && <Reviewed ctx={ctx} kind="poe_cycle" values={{ interface: port.name }} label={t("dc.poe")} done={t("dc.poeDone")} danger />}
    </details>
  );
}

export function RouterPorts({ ports, ctx }: { ports: PortInfo[]; ctx: Context }) {
  const { t } = useI18n();
  if (!ports.length) return null;
  return (
    <section className="stack"><h3>{t("dc.ports")}</h3>
      {ctx.locked && <Note>{t("dc.locked")}</Note>}
      {ports.map((p) => <PortCard key={p.name} port={p} ctx={ctx} />)}
    </section>
  );
}

/** Everything here changes the router or this app. The app holds no login for the device itself, and the page says so. */
export function DeviceControls({ node, ports, ctx }: { node: TNode; ports: PortInfo[]; ctx: Context }) {
  const { t } = useI18n();
  const mac = node.mac ?? "";
  const reserve = new URLSearchParams({ task: "lease", "mac-address": mac, ...(node.ip ? { address: node.ip } : {}), ...(node.dhcpServer ? { server: node.dhcpServer } : {}) });
  const limit = new URLSearchParams({ task: "queue", name: "dev-" + mac.replaceAll(":", ""), ...(node.ip ? { target: node.ip + "/32" } : {}) });
  const own = ports.filter((p) => node.ports.includes(p.name));
  const hotspot = !!node.session || !!node.guestBanned;
  return (
    <>
      {!node.upstream && mac && <section className="stack"><h3>{t("dc.naming")}</h3><RenameForm node={node} routerId={ctx.routerId} onChanged={ctx.onChanged} /></section>}
      <section className="stack">
        <h3>{t("dc.title")}</h3>
        <p className="soft small">{t("dc.lead")}</p>
        {node.upstream ? <p className="soft small">{t("dc.upstream")}</p> : mac && (
          <>
            <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
              <a className="pill" href={"#/network/existing?" + reserve.toString()}>{t("dc.reserve")}</a>
              {node.ip && <a className="pill" href={"#/network/existing?" + limit.toString()}>{t("dc.limit")}</a>}
            </div>
            {(hotspot ? ctx.guestLocked : ctx.locked) && <Note>{t("dc.locked")}</Note>}
            {node.session && <GuestAction ctx={ctx} path="disconnect" body={{ id: node.session.id, mac: node.session.mac, address: node.session.address, user: node.session.user }} label={t("dc.guest")} confirm={`${t("dc.guestConfirm")} ${node.session.user}`} done={t("dc.guestDone")} danger />}
            {(node.blocked || node.guestBanned) && <Note kind="warn">{t("dc.blockedNow")}</Note>}
            {/* A Hotspot IP binding only acts on a Hotspot network, so anything else gets the firewall rule. Each block is removed the way it was made. */}
            {node.guestBanned ? <GuestAction ctx={ctx} path="unblock" body={{ mac }} label={t("dc.unblock")} confirm={t("dc.guestUnblockConfirm")} done={t("dc.unblockDone")} />
              : hotspot && !node.blocked ? <GuestAction ctx={ctx} path="block" body={{ mac }} label={t("dc.block")} confirm={t("dc.guestBlockConfirm")} done={t("dc.guestBlockDone")} danger />
              : <Reviewed key={String(node.blocked)} ctx={ctx} kind={node.blocked ? "device_unblock" : "device_block"} values={{ mac }} label={node.blocked ? t("dc.unblock") : t("dc.block")} done={node.blocked ? t("dc.unblockDone") : t("dc.blockDone")} danger={!node.blocked} />}
          </>
        )}
        {own.map((p) => <PortCard key={p.name} port={p} ctx={ctx} />)}
      </section>
    </>
  );
}
