import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { choices, currentNames, descriptions, editNetworkValues, initialValues, moveNetworkValues, isSecretField, labelOf, listFor, objectsOf, payloadValues, schema, workspaceInfo, type Field, type Inventory, type Workspace } from "./schema";
import type { RouterRow } from "../routers";

interface Review { steps: string[]; warnings: string[]; confirmation: string }

/**
 * A change that reshaped something in use is applied but not final: the router's own
 * timer puts it back unless the owner presses Keep. The countdown is a display only;
 * the server and the router decide what happens at the deadline.
 */
function PendingPanel({ routerId, revertAt, onDone }: { routerId: string; revertAt: string; onDone: (message: string) => void }) {
  const { t } = useI18n();
  const deadline = useMemo(() => new Date(revertAt).getTime(), [revertAt]);
  const [now, setNow] = useState(() => Date.now()), [busy, setBusy] = useState(false), [error, setError] = useState("");
  useEffect(() => { const id = window.setInterval(() => setNow(Date.now()), 1000); return () => window.clearInterval(id); }, []);
  const left = Math.max(0, Math.round((deadline - now) / 1000));
  useEffect(() => { if (left === 0) onDone(t("net.timeUp")); }, [left, onDone, t]);
  async function decide(action: "confirm" | "revert") {
    setBusy(true); setError("");
    try {
      await api.post(`/routers/${encodeURIComponent(routerId)}/network/${action}`, {});
      onDone(action === "confirm" ? t("net.kept") : t("net.reverted"));
    } catch (err) { if (isUnauthorized(err)) throw err; setError(errMsg(err)); }
    finally { setBusy(false); }
  }
  return (
    <div className="glass pad stack" role="region" aria-label={t("net.pendingTitle")}>
      <h3>{t("net.pendingTitle")}</h3>
      <p className="soft small">{t("net.pendingBody")}</p>
      <p role="timer" aria-live="off"><b>{t("net.timeLeft")}: {Math.floor(left / 60)}:{String(left % 60).padStart(2, "0")}</b></p>
      {error && <Note kind="bad">{error}</Note>}
      <div className="row" style={{ justifyContent: "flex-start" }}>
        <button className="btn" type="button" disabled={busy} onClick={() => void decide("confirm")}>{t("net.keep")}</button>
        <button className="pill" type="button" disabled={busy} onClick={() => void decide("revert")}>{t("net.revertNow")}</button>
      </div>
    </div>
  );
}
const errMsg = (e: unknown) => (e as { data?: { error?: string } } | null)?.data?.error || (e instanceof Error ? e.message : "") || "Request failed";
const isOn = (v?: string) => v === "yes" || v === "true";

function CurrentSettings({ inv, kind }: { inv: Inventory; kind: string }) {
  const { t } = useI18n();
  const s = schema[kind];
  const menus: Array<[string, string]> = kind === "wan" ? [["dhcp_clients", "Automatic internet connections"], ["pppoe_clients", "PPPoE internet connections"], ["routes", "Router routes"]] : s.menu ? [[s.menu, s.title]] : [];
  if (!menus.length) return null;
  return (
    <div className="stack current">
      <h3>{t("net.current")}</h3><p className="soft small">{t("net.currentNote")}</p>
      {menus.map(([menu, title]) => (
        <section key={menu} className="stack">
          <h4>{title}</h4>
          {!inv.available?.[menu] ? <p className="soft small">{t("net.unavailable")}</p> : objectsOf(inv, menu).length === 0 ? <p className="soft small">{t("net.noRecords")}</p> : objectsOf(inv, menu).map((o) => (
            <article className="rowitem recordcard" key={o.id}>
              <b>{labelOf(o)}</b>
              <dl className="facts">{Object.entries(o.values ?? {}).filter(([k]) => !isSecretField(k)).map(([k, v]) => (
                <div key={k} style={{ display: "contents" }}><dt>{currentNames[k] || s.fields.find((f) => f[0] === k)?.[1] || k}</dt><dd>{k === "disabled" ? (isOn(v) ? t("net.disabled") : t("net.enabled")) : v || t("hub.notReported")}</dd></div>
              ))}</dl>
            </article>
          ))}
        </section>
      ))}
    </div>
  );
}

function TaskForm({ kind, inv, defaults, routerId, routerName, locked, setLocked, onPending }: { kind: string; inv: Inventory; defaults: Record<string, string>; routerId: string; routerName: string; locked: boolean; setLocked: (b: boolean) => void; onPending: (revertAt: string) => void }) {
  const { t } = useI18n();
  const s = schema[kind], list = useMemo(() => listFor(inv, kind), [inv, kind]);
  const [recordId, setRecordId] = useState("");
  const selected = list.find((o) => o.id === recordId);
  const start = (record: typeof selected) => (kind === "edit_network" && record ? { ...initialValues(kind, record, defaults), ...editNetworkValues(inv, record) } : kind === "move_network" && record ? { ...initialValues(kind, record, defaults), ...moveNetworkValues(inv, record) } : initialValues(kind, record, defaults));
  const [values, setValues] = useState<Record<string, string>>(() => start(undefined));
  const [review, setReview] = useState<Review | null>(null);
  const [consent, setConsent] = useState(false), [status, setStatus] = useState("");
  const [applied, setApplied] = useState(false), [removing, setRemoving] = useState(false);
  const stamp = useRef(0);
  useEffect(() => { setValues(start(selected)); setReview(null); setConsent(false); setStatus(""); setApplied(false); setRemoving(false); stamp.current++; }, [kind, recordId, selected, defaults]);

  const portTypes = kind === "vlan" ? ["ether", "bridge", "bond"] : kind === "ip_address" ? ["ether", "bridge", "bond", "vlan"] : ["ether", "bridge", "vlan", "bond", "pppoe-out"];
  const eligible = (inv.interfaces ?? []).filter((i) => portTypes.includes(i.type));
  // Fields the router cannot change on an existing record.
  const fixed = (key: string) => !!selected && ((kind === "ip_address" && key === "interface") || (kind === "vlan" && key === "name"));
  const change = (key: string, value: string) => {
    stamp.current++; setReview(null); setConsent(false); setStatus(t("net.changed")); setApplied(false); setRemoving(false);
    setValues((v) => {
      const next = { ...v, [key]: value };
      if (kind === "wan" && (key === "wan1" || key === "wan2")) { const n = key.slice(-1); const c = objectsOf(inv, "dhcp_clients").find((o) => o.values.interface === value); next["gateway" + n] = c?.values.gateway || ""; }
      return next;
    });
  };
  function control(f: Field) {
    const [key, , type, arg] = f;
    let value = values[key] ?? "";
    if (key === "disabled") value = isOn(value) ? "yes" : "no";
    let options: Array<[string, string]> | null = null;
    if (type === "state") options = [["no", t("net.enabledOpt")], ["yes", t("net.disabledOpt")]];
    if (type === "select") options = arg as Array<[string, string]>;
    if (type === "interface") options = [["", t("net.choosePort")], ...eligible.map((i): [string, string] => [i.name, `${i.name}${i.running ? " · " + t("net.running") : ""}`])];
    if (type === "record") options = [["", t("net.chooseRecord")], ...objectsOf(inv, arg as string).filter((o) => arg !== "ppp_profiles" || !o.values.name?.startsWith("mm-bb-")).map((o): [string, string] => [o.values.name, o.values.name])];
    const id = "nf-" + key;
    return options
      ? <select className="input" id={id} name={key} required disabled={locked || fixed(key)} value={value} onChange={(e) => change(key, e.target.value)}>{options.map(([v, label]) => <option key={v} value={v}>{label}</option>)}</select>
      : <input className="input" id={id} name={key} required disabled={locked} maxLength={128} type={type === "password" ? "password" : type === "number" ? "number" : "text"} autoComplete={type === "password" ? "new-password" : undefined}
          readOnly={(!!selected && kind === "fairshare" && ["name", "target"].includes(key)) || fixed(key)} value={type === "password" ? values[key] ?? "" : value} onChange={(e) => change(key, e.target.value)} />;
  }
  const fields = s.fields;
  const groups: Array<[string, Field[]]> = kind === "wan" ? [[t("net.g.strategy"), fields.slice(0, 1)], [t("net.g.primary"), fields.slice(1, 3)], [t("net.g.backup"), fields.slice(3)]]
    : kind === "pppoe_server" ? [[t("net.g.service"), fields.slice(0, 2)], [t("net.g.addresses"), fields.slice(2, 5)], [t("net.g.dnsSpeed"), fields.slice(5)]] : [[selected ? t("net.g.edit") : t("net.g.new"), fields]];

  // Only records this app created can be deleted; the server enforces the same rule.
  const deletable = !!selected && (kind === "ip_address" || kind === "vlan") && selected.values.comment === "mega-magic" && !isOn(selected.values.dynamic);
  const wireKind = (remove: boolean) => (remove ? "remove_" + kind : kind);
  const wireValues = (remove: boolean) => (remove ? {} : payloadValues(kind, values));
  async function preview(e: FormEvent<HTMLFormElement>) { e.preventDefault(); await ask(false); }
  async function ask(remove: boolean) {
    if (!routerId) return;
    setRemoving(remove);
    const mine = ++stamp.current;
    setReview(null); setConsent(false); setStatus(t("net.reviewing"));
    try {
      const data = await api.post<{ review?: Review }>(`/routers/${encodeURIComponent(routerId)}/network/preview`, { kind: wireKind(remove), id: selected?.id ?? "", values: wireValues(remove) });
      if (mine !== stamp.current || !data.review) return;
      setReview(data.review); setStatus("");
    } catch (err) { if (isUnauthorized(err)) throw err; if (mine === stamp.current) setStatus(`${t("net.reviewFail")} ${errMsg(err)}`); }
  }
  async function apply() {
    if (locked || !review || !routerId) return;
    const request = { kind: wireKind(removing), id: selected?.id ?? "", values: wireValues(removing), confirmation: review.confirmation };
    setLocked(true); setReview(null); setConsent(false); setStatus(t("net.applying")); const mine = ++stamp.current;
    try {
      const data = await api.post<{ result?: { verified?: boolean; backup?: string; pending_confirm?: boolean; revert_at?: string } }>(`/routers/${encodeURIComponent(routerId)}/network/apply`, request);
      if (!data.result?.verified) throw new Error("Router changes were not verified");
      // Secrets never linger in the form once applied.
      setValues((v) => Object.fromEntries(Object.entries(v).map(([k, x]) => [k, isSecretField(k) ? "" : x])));
      if (data.result.pending_confirm && data.result.revert_at) { onPending(data.result.revert_at); setStatus(t("net.heldApplied")); return; }
      if (mine === stamp.current) { setStatus(`${t("net.verified")} ${data.result.backup ?? ""}. ${t("net.testClient")}`); setApplied(true); }
    } catch (err) {
      if (isUnauthorized(err)) throw err;
      const rolled = err instanceof ApiError && (err.data as { result?: { rolled_back?: boolean } } | null)?.result?.rolled_back;
      if (mine === stamp.current) setStatus(`${t("net.applyFail")} ${errMsg(err)} ${rolled ? t("net.rolledBack") : t("net.rollbackUnconfirmed")}`);
    } finally { setLocked(false); }
  }

  return (
    <div className="stack">
      {s.menu && (
        <label className="field"><span>{t("net.chooseRecordLabel")}</span>
          <select className="input" value={recordId} disabled={locked} onChange={(e) => setRecordId(e.target.value)}>
            {s.create && <option value="">{t("net.createNew")}</option>}
            {list.map((o) => <option key={o.id} value={o.id}>{labelOf(o)}</option>)}
          </select>
        </label>
      )}
      {s.menu && !s.create && !list.length ? <Note>{t("net.noExisting")}</Note> : (
        <form className="stack" onSubmit={preview}>
          {groups.map(([title, items]) => (
            <fieldset className="glass pad stack fs" key={title} disabled={locked}><legend>{title}</legend>
              {items.map((f) => <label className="field" key={f[0]} htmlFor={"nf-" + f[0]}><span>{f[1]}</span>{control(f)}</label>)}
            </fieldset>
          ))}
          <p className="soft small">{kind === "wan" ? t("net.hint.wan") : kind === "wifi" || kind === "wireless" ? t("net.hint.wifi") : t("net.hint.default")}</p>
          <div className="row" style={{ justifyContent: "flex-start" }}>
            <button className="btn" type="submit" disabled={locked || ((kind === "remove_network" || kind === "edit_network" || kind === "move_network") && !selected)}>{t("net.review")}</button>
            {deletable && <button className="pill" type="button" disabled={locked} onClick={() => void ask(true)}>{t("net.delete")}</button>}
          </div>
        </form>
      )}
      {status && <p className="note" role="status" aria-live="polite">{status}</p>}
      {review && (
        <div className="glass pad stack" role="region" aria-label={t("net.reviewTitle")}>
          <h3>{t("net.reviewTitle")}</h3>
          <p className="soft small">{t("net.router")}: <b>{routerName}</b></p>
          <ul className="steps">{review.steps.map((x, i) => <li key={i}>{x}</li>)}</ul>
          {review.warnings.map((w, i) => <Note kind="warn" key={i}>{w}</Note>)}
          <label className="check"><span>{t("net.consent")}</span><input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} /></label>
          <button className="btn" type="button" disabled={!consent || locked} onClick={() => void apply()}>{t("net.apply")}</button>
        </div>
      )}
      {applied && <p className="soft small">{t("net.reloadHint")}</p>}
    </div>
  );
}

const EMPTY_INVENTORY: Inventory = { interfaces: [], available: {}, objects: {}, write_enabled: false };

/**
 * Every task stays visible. With no router, or one that cannot be read, the forms
 * are shown locked under a clear banner, so nothing is hidden and nothing can be
 * applied. `reason` overrides the banner when the caller already knows why.
 */
export function NetworkEditor({ workspace, router, canEdit, reason }: { workspace: Workspace; router?: RouterRow; canEdit: boolean; reason?: string }) {
  const { t } = useI18n();
  const kinds = choices[workspace];
  const [load, setLoad] = useState<{ kind: "loading" } | { kind: "error"; err: unknown } | { kind: "ready"; inv: Inventory; defaults: Record<string, string> }>(router ? { kind: "loading" } : { kind: "error", err: null });
  const [task, setTask] = useState(kinds[0]);
  const [locked, setLocked] = useState(false);
  const [pendingAt, setPendingAt] = useState(""), [pendingNote, setPendingNote] = useState("");
  const rev = useRef(0);
  const readController = useRef<AbortController | null>(null);
  useEffect(() => { setTask(choices[workspace][0]); }, [workspace]);

  const routerId = router?.device_id ?? "";
  const reload = useCallback(async () => {
    if (!routerId) { setLoad({ kind: "error", err: null }); return; }
    readController.current?.abort();
    const controller = new AbortController();
    readController.current = controller;
    const timer = window.setTimeout(() => controller.abort(), 20000);
    const mine = ++rev.current;
    setLoad({ kind: "loading" });
    try {
      const data = await api.get<{ inventory: Inventory }>(`/routers/${encodeURIComponent(routerId)}/network/inventory`, controller.signal);
      let defaults: Record<string, string> = {};
      if (workspace === "pppoe") { try { defaults = (await api.get<{ defaults?: Record<string, string> }>(`/routers/${encodeURIComponent(routerId)}/gateway/defaults`, controller.signal)).defaults ?? {}; } catch { /* never invent an unchecked default */ } }
      if (controller.signal.aborted) throw new Error(t("net.readTimeout"));
      if (mine === rev.current) { setLoad({ kind: "ready", inv: data.inventory, defaults }); setPendingAt(data.inventory.pending_confirm?.revert_at ?? ""); }
    } catch (err) { if (isUnauthorized(err)) throw err; if (mine === rev.current) setLoad({ kind: "error", err: controller.signal.aborted ? new Error(t("net.readTimeout")) : err }); }
    finally { window.clearTimeout(timer); if (readController.current === controller) readController.current = null; }
  }, [routerId, workspace, t]);
  useEffect(() => { const generation = rev; void reload(); return () => { generation.current++; readController.current?.abort(); }; }, [reload]);

  const finishPending = useCallback((message: string) => { setPendingAt(""); setPendingNote(message); void reload(); }, [reload]);
  const [name, lead] = workspaceInfo[workspace];
  const loading = load.kind === "loading";
  const offline = load.kind !== "ready";
  const inv = load.kind === "ready" ? load.inv : EMPTY_INVENTORY, defaults = load.kind === "ready" ? load.defaults : {};
  const s = schema[task];
  const needsHub = !router || (load.kind === "error" && load.err instanceof ApiError && (load.err.data as { code?: string } | null)?.code === "router_management_required");
  const banner = loading ? t("r.checking") : !offline ? "" : reason ? reason : !router ? t("net.noRouterBanner") : needsHub ? t("net.connectFirst") : `${t("net.offlineBanner")} ${load.kind === "error" ? errMsg(load.err) : ""}`;
  // Mirror the server's gates. Nothing is hidden: a task that cannot be applied yet
  // still shows its form, locked, with the reason above it. Only a task with nothing
  // to show (read-only, or unsupported on this router) is replaced by a note.
  const waiting = !!pendingAt;
  const writeHold = offline ? "" : !canEdit ? t("net.needAdmin") : inv.write_enabled !== true ? t("net.writesOff")
    : (task === "wifi" || task === "wireless") && inv.wifi_write_enabled !== true ? t("net.wifiOff")
    : (task === "ip_address" || task === "vlan" || task === "remove_network" || task === "edit_network" || task === "move_network") && inv.address_write_enabled !== true ? t("net.addressOff") : "";
  const blocked = offline ? "" : s.readOnly ? descriptions[task] : s.menu && !inv.available?.[s.menu] ? t("net.menuUnsupported") : "";
  return (
    <div className="stack">
      {offline && (
        <div className="stack">
          <Note kind={loading ? "warn" : "bad"}>{banner}</Note>
          <div className="row" style={{ justifyContent: "flex-start" }}>
            {needsHub && <a className="pill on" href="#/hub">{t("hub.title")}</a>}
            {router && !loading && <button className="pill" type="button" onClick={() => void reload()}>{t("net.refresh")}</button>}
          </div>
        </div>
      )}
      {pendingNote && !waiting && <Note>{pendingNote}</Note>}
      {waiting && <PendingPanel routerId={routerId} revertAt={pendingAt} onDone={finishPending} />}
      <details className="glass pad"><summary>{t("net.about")}</summary><p className="soft small">{lead} {t("net.reviewRequired")}</p></details>
      {!offline && <div className="row"><div className="stack" style={{ gap: 2 }}><b>{t("net.loaded")}</b><span className="soft small">{(inv.interfaces ?? []).length} {t("net.interfaces")}. {t("net.openingSafe")}</span></div><button className="pill" type="button" disabled={locked || waiting} onClick={() => void reload()}>{t("net.refresh")}</button></div>}
      <div className="tasks" role="group" aria-label={name}>
        {kinds.map((k) => (
          <button type="button" className="task" key={k} aria-pressed={k === task} disabled={locked || waiting} onClick={() => setTask(k)}>
            <b>{schema[k].title}</b><span className="soft small">{descriptions[k]}</span>
            {!offline && <span className="mute small">{schema[k].menu ? `${objectsOf(inv, schema[k].menu!).length} ${t("net.records")}` : t("net.wizard")}</span>}
          </button>
        ))}
      </div>
      <section className="stack" aria-live="polite">
        {!offline && <CurrentSettings inv={inv} kind={task} />}
        <h3>{s.title}</h3><p className="soft small">{descriptions[task]}</p>
        {blocked ? <Note>{blocked}</Note> : (
          <>
            {writeHold && <Note kind="warn">{writeHold}</Note>}
            {!writeHold && (task === "ip_address" || task === "vlan" || task === "remove_network" || task === "edit_network" || task === "move_network") && inv.address_preview_only === true && <Note kind="warn">{t("net.addressPreview")}</Note>}
            <fieldset className="nolock" disabled={offline || !!writeHold || waiting}>
              <TaskForm key={task + (offline ? "-unavailable" : "-ready")} kind={task} inv={inv} defaults={defaults} routerId={routerId} routerName={router?.hardware_name ?? ""} locked={locked || offline || !!writeHold || waiting} setLocked={setLocked} onPending={(at) => { setPendingNote(""); setPendingAt(at); }} />
            </fieldset>
          </>
        )}
      </section>
    </div>
  );
}
