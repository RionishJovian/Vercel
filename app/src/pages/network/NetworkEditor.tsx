import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { Skeleton } from "../../components/Skeleton";
import { choices, currentNames, descriptions, initialValues, isSecretField, labelOf, listFor, objectsOf, schema, workspaceInfo, type Field, type Inventory, type Workspace } from "./schema";
import type { RouterRow } from "../routers";

interface Review { steps: string[]; warnings: string[]; confirmation: string }
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

function TaskForm({ kind, inv, defaults, routerId, routerName, locked, setLocked }: { kind: string; inv: Inventory; defaults: Record<string, string>; routerId: string; routerName: string; locked: boolean; setLocked: (b: boolean) => void }) {
  const { t } = useI18n();
  const s = schema[kind], list = useMemo(() => listFor(inv, kind), [inv, kind]);
  const [recordId, setRecordId] = useState("");
  const selected = list.find((o) => o.id === recordId);
  const [values, setValues] = useState<Record<string, string>>(() => initialValues(kind, undefined, defaults));
  const [review, setReview] = useState<Review | null>(null);
  const [consent, setConsent] = useState(false), [status, setStatus] = useState("");
  const [applied, setApplied] = useState(false);
  const stamp = useRef(0);
  useEffect(() => { setValues(initialValues(kind, selected, defaults)); setReview(null); setConsent(false); setStatus(""); setApplied(false); stamp.current++; }, [kind, recordId, selected, defaults]);

  const eligible = (inv.interfaces ?? []).filter((i) => ["ether", "bridge", "vlan", "bond", "pppoe-out"].includes(i.type));
  const change = (key: string, value: string) => {
    stamp.current++; setReview(null); setConsent(false); setStatus(t("net.changed")); setApplied(false);
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
    if (type === "record") options = [["", t("net.chooseRecord")], ...objectsOf(inv, arg as string).map((o): [string, string] => [o.values.name, o.values.name])];
    const id = "nf-" + key;
    return options
      ? <select className="input" id={id} name={key} required disabled={locked} value={value} onChange={(e) => change(key, e.target.value)}>{options.map(([v, label]) => <option key={v} value={v}>{label}</option>)}</select>
      : <input className="input" id={id} name={key} required disabled={locked} maxLength={128} type={type === "password" ? "password" : type === "number" ? "number" : "text"} autoComplete={type === "password" ? "new-password" : undefined}
          readOnly={!!selected && kind === "fairshare" && ["name", "target"].includes(key)} value={type === "password" ? values[key] ?? "" : value} onChange={(e) => change(key, e.target.value)} />;
  }
  const fields = s.fields;
  const groups: Array<[string, Field[]]> = kind === "wan" ? [[t("net.g.strategy"), fields.slice(0, 1)], [t("net.g.primary"), fields.slice(1, 3)], [t("net.g.backup"), fields.slice(3)]]
    : kind === "pppoe_server" ? [[t("net.g.service"), fields.slice(0, 2)], [t("net.g.addresses"), fields.slice(2, 5)], [t("net.g.dnsSpeed"), fields.slice(5)]] : [[selected ? t("net.g.edit") : t("net.g.new"), fields]];

  async function preview(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const mine = ++stamp.current;
    setReview(null); setConsent(false); setStatus(t("net.reviewing"));
    try {
      const data = await api.post<{ review?: Review }>(`/routers/${encodeURIComponent(routerId)}/network/preview`, { kind, id: selected?.id ?? "", values });
      if (mine !== stamp.current || !data.review) return;
      setReview(data.review); setStatus("");
    } catch (err) { if (isUnauthorized(err)) throw err; if (mine === stamp.current) setStatus(`${t("net.reviewFail")} ${errMsg(err)}`); }
  }
  async function apply() {
    if (locked || !review) return;
    const request = { kind, id: selected?.id ?? "", values, confirmation: review.confirmation };
    setLocked(true); setReview(null); setConsent(false); setStatus(t("net.applying")); const mine = ++stamp.current;
    try {
      const data = await api.post<{ result?: { verified?: boolean; backup?: string } }>(`/routers/${encodeURIComponent(routerId)}/network/apply`, request);
      if (!data.result?.verified) throw new Error("Router changes were not verified");
      // Secrets never linger in the form once applied.
      setValues((v) => Object.fromEntries(Object.entries(v).map(([k, x]) => [k, isSecretField(k) ? "" : x])));
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
          <button className="btn" type="submit" disabled={locked}>{t("net.review")}</button>
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

export function NetworkEditor({ workspace, router, canEdit }: { workspace: Workspace; router: RouterRow; canEdit: boolean }) {
  const { t } = useI18n();
  const kinds = choices[workspace];
  const [load, setLoad] = useState<{ kind: "loading" } | { kind: "error"; err: unknown } | { kind: "ready"; inv: Inventory; defaults: Record<string, string> }>({ kind: "loading" });
  const [task, setTask] = useState(kinds[0]);
  const [locked, setLocked] = useState(false);
  const rev = useRef(0);
  useEffect(() => { setTask(choices[workspace][0]); }, [workspace]);

  const reload = useCallback(async () => {
    const mine = ++rev.current;
    setLoad({ kind: "loading" });
    try {
      const data = await api.get<{ inventory: Inventory }>(`/routers/${encodeURIComponent(router.device_id)}/network/inventory`);
      let defaults: Record<string, string> = {};
      if (workspace === "pppoe") { try { defaults = (await api.get<{ defaults?: Record<string, string> }>(`/routers/${encodeURIComponent(router.device_id)}/gateway/defaults`)).defaults ?? {}; } catch { /* never invent an unchecked address */ } }
      if (mine === rev.current) setLoad({ kind: "ready", inv: data.inventory, defaults });
    } catch (err) { if (isUnauthorized(err)) throw err; if (mine === rev.current) setLoad({ kind: "error", err }); }
  }, [router.device_id, workspace]);
  useEffect(() => { void reload(); }, [reload]);

  const [name, lead] = workspaceInfo[workspace];
  if (load.kind === "loading") return <Skeleton h={300} />;
  if (load.kind === "error") {
    const needs = load.err instanceof ApiError && (load.err.data as { code?: string } | null)?.code === "router_management_required";
    return (
      <div className="stack">
        <Note kind="warn">{needs ? t("net.connectFirst") : t("net.readFail")} {errMsg(load.err)}</Note>
        <div className="row" style={{ justifyContent: "flex-start" }}>{needs && <a className="pill on" href="#/hub">{t("hub.title")}</a>}<button className="pill" type="button" onClick={() => void reload()}>{t("net.refresh")}</button></div>
      </div>
    );
  }
  const { inv, defaults } = load, s = schema[task];
  // Mirror the server's gates so an unsupported change is never offered.
  const blocked = s.readOnly ? descriptions[task] : !canEdit ? t("net.needAdmin") : inv.write_enabled !== true ? t("net.writesOff")
    : (task === "wifi" || task === "wireless") && inv.wifi_write_enabled !== true ? t("net.wifiOff") : s.menu && !inv.available?.[s.menu] ? t("net.menuUnsupported") : "";
  return (
    <div className="stack">
      <details className="glass pad"><summary>{t("net.about")}</summary><p className="soft small">{lead} {t("net.reviewRequired")}</p></details>
      <div className="row"><div className="stack" style={{ gap: 2 }}><b>{t("net.loaded")}</b><span className="soft small">{(inv.interfaces ?? []).length} {t("net.interfaces")}. {t("net.openingSafe")}</span></div><button className="pill" type="button" disabled={locked} onClick={() => void reload()}>{t("net.refresh")}</button></div>
      <div className="tasks" role="group" aria-label={name}>
        {kinds.map((k) => (
          <button type="button" className="task" key={k} aria-pressed={k === task} disabled={locked} onClick={() => setTask(k)}>
            <b>{schema[k].title}</b><span className="soft small">{descriptions[k]}</span>
            <span className="mute small">{schema[k].menu ? `${objectsOf(inv, schema[k].menu!).length} ${t("net.records")}` : t("net.wizard")}</span>
          </button>
        ))}
      </div>
      <section className="stack" aria-live="polite">
        <CurrentSettings inv={inv} kind={task} />
        <h3>{s.title}</h3><p className="soft small">{descriptions[task]}</p>
        {blocked ? <Note>{blocked}</Note> : <TaskForm key={task} kind={task} inv={inv} defaults={defaults} routerId={router.device_id} routerName={router.hardware_name ?? ""} locked={locked} setLocked={setLocked} />}
      </section>
    </div>
  );
}
