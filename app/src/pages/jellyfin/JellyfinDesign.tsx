import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { api, ApiError, isUnauthorized } from "../../lib/api";
import { asArray, type Me } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Field, Note } from "../../components/ui";
import { applyTemplate, guestDocument, normalize, posterNames, prepareImage, templates, tileDefaults } from "./design";

/* eslint-disable @typescript-eslint/no-explicit-any */
interface Pub { revision: number; public_id: string; design: any }
interface Lib { ItemId?: string; Name?: string; CollectionType?: string }
const errText = (e: unknown, f: string) => { const m = e instanceof Error ? e.message : ""; return m && !/^http-\d+$/.test(m) ? m : f; };

function Section({ title, children }: { title: string; children: ReactNode }) { return <section className="glass pad stack"><h2>{title}</h2>{children}</section>; }

/** Picks a picture, re-encodes it in the browser (no SVG, remote links or metadata survive) and hands back a data URI. */
function Picture({ label, value, onChange, onError }: { label: string; value: string; onChange: (v: string) => void; onError: (m: string) => void }) {
  const { t } = useI18n();
  return (
    <div className="stack">
      <label className="field"><span>{label}</span><input className="input" type="file" accept="image/jpeg,image/png,image/webp" onChange={async (e) => { const f = e.target.files?.[0]; e.target.value = ""; if (!f) return; try { onChange(await prepareImage(f)); } catch (err) { onError(errText(err, t("jd.pictureFail"))); } }} /></label>
      {value && <div className="row" style={{ justifyContent: "flex-start" }}><img src={value} alt="" width={64} height={64} style={{ objectFit: "cover", borderRadius: 10 }} /><button className="pill" type="button" onClick={() => onChange("")}>{t("jd.removePicture")}</button></div>}
    </div>
  );
}

export function JellyfinDesign({ me }: { me: Me }) {
  const { t } = useI18n();
  const key = me.business?.business_id && me.user?.email ? `mm_media_design:${me.business.business_id}:${me.user.email}` : null;
  const [d, setD] = useState<any>(() => {
    const base = normalize({ name: me.business?.company_name || undefined });
    if (key) { try { const raw = localStorage.getItem(key); if (raw) return normalize(JSON.parse(raw)); } catch { /* ignore */ } }
    return base;
  });
  const [pub, setPub] = useState<Pub | null>(null), [loaded, setLoaded] = useState(false), [paid, setPaid] = useState(false);
  const [libs, setLibs] = useState<Lib[]>([]), [chosen, setChosen] = useState<string[]>([]), [libMsg, setLibMsg] = useState(""), [connector, setConnector] = useState<boolean | null>(null);
  const [status, setStatus] = useState(""), [busy, setBusy] = useState(false), [pubMsg, setPubMsg] = useState("");
  const edited = useRef(false);
  const set = (patch: Record<string, any>) => { edited.current = true; setD((x: any) => ({ ...x, ...patch })); };
  const design = useMemo(() => normalize({ ...d, libraries: chosen, payment: d.payment }), [d, chosen]);
  const doc = useMemo(() => guestDocument(design), [design]);

  useEffect(() => { if (key) { try { localStorage.setItem(key, JSON.stringify(design)); } catch { /* ignore */ } } }, [key, design]);
  const reload = useCallback(async (explicit: boolean) => {
    try {
      const r = await api.get<{ publication?: Pub | null; paid_access_ready?: boolean }>("/media/publication");
      setPaid(r.paid_access_ready === true); setPub(r.publication ?? null); setLoaded(true);
      if (explicit || !edited.current) { if (r.publication) { setD(normalize(r.publication.design)); setChosen(asArray<string>(r.publication.design?.libraries)); } edited.current = false; }
      setPubMsg(r.publication ? "" : t("jd.notPublished"));
    } catch (e) { if (isUnauthorized(e)) throw e; setLoaded(false); setPubMsg(e instanceof ApiError && (e.status === 404 || e.status === 503) ? t("jd.unavailable") : errText(e, t("jd.loadFail"))); }
  }, [t]);
  useEffect(() => { void reload(false); }, [reload]);
  useEffect(() => { api.get<{ connector?: { available?: boolean } }>("/media/capabilities").then((v) => setConnector(v.connector?.available === true)).catch(() => setConnector(false)); }, []);

  async function loadLibraries() {
    setLibMsg(t("jd.readingLibs")); setBusy(true);
    try {
      const id = crypto.randomUUID();
      await api.post("/media/jobs", { id, operation: "libraries", payload: {} });
      const end = Date.now() + 125000; let list: Lib[] | null = null;
      while (Date.now() < end) {
        const j = await api.get<{ state?: string; result?: { libraries?: Lib[] } }>("/media/jobs/" + id);
        if (j.state === "succeeded") { list = j.result?.libraries ?? null; break; }
        if (["failed", "cancelled", "expired"].includes(j.state ?? "")) throw new Error(t("jd.libJobFail") + " " + j.state);
        await new Promise((r) => setTimeout(r, 2000));
      }
      if (!Array.isArray(list)) throw new Error(t("jd.libUnconfirmed"));
      const ok = list.filter((l) => /^(?:[0-9a-f]{32}|[0-9a-f-]{36})$/.test(l.ItemId ?? "") && !["photos", "books"].includes(l.CollectionType ?? ""));
      setLibs(ok); setLibMsg(ok.length ? t("jd.pickLibs") : t("jd.noLibs"));
    } catch (e) { if (isUnauthorized(e)) throw e; setLibMsg(errText(e, t("jd.libUnconfirmed"))); }
    setBusy(false);
  }
  async function publish() {
    setBusy(true); setStatus(t("jd.publishing"));
    try { const r = await api.put<{ publication: Pub; paid_access_ready?: boolean }>(`/media/publication?revision=${pub?.revision ?? 0}`, design); setPub(r.publication); setPaid(r.paid_access_ready === true); edited.current = false; setStatus(t("jd.published")); }
    catch (e) { if (isUnauthorized(e)) throw e; setStatus(errText(e, t("jd.publishFail"))); }
    setBusy(false);
  }
  async function unpublish() {
    if (!pub || !window.confirm(t("jd.unpublishConfirm"))) return;
    setBusy(true);
    try { await api.del(`/media/publication?revision=${pub.revision}`); setPub(null); setStatus(t("jd.unpublished")); setPubMsg(t("jd.notPublished")); }
    catch (e) { if (isUnauthorized(e)) throw e; setStatus(errText(e, t("jd.unpublishFail"))); }
    setBusy(false);
  }
  const text = (name: string, label: string, max = 160) => <Field label={label}><input className="input" maxLength={max} value={d[name] ?? ""} onChange={(e) => set({ [name]: e.target.value })} /></Field>;
  const setTile = (k: string, patch: Record<string, string>) => set({ tiles: { ...d.tiles, [k]: { ...d.tiles[k], ...patch } } });
  const setPoster = (i: number, patch: Record<string, string>) => set({ posters: { ...d.posters, [i]: { ...d.posters[i], ...patch } } });
  const pay = d.payment ?? { bank_name: "", bank_account: "", bank_holder: "", mmqr: "" };
  const setPay = (patch: Record<string, string>) => { const next = { ...pay, ...patch }; set({ payment: Object.values(next).some(Boolean) ? next : null }); };
  const pictureErr = (m: string) => setStatus(m);

  return (
    <div className="gp-layout">
      <section className="glass pad stack gp-preview" aria-labelledby="jd-prev">
        <h2 id="jd-prev">{t("g.preview")}</h2>
        <div className="phone"><iframe className="gp-frame" sandbox="" title={t("g.preview")} srcDoc={doc} /></div>
        <p className="soft small">{t("jd.previewNote")}</p>
      </section>
      <form className="stack gp-form" onSubmit={(e) => { e.preventDefault(); void publish(); }}>
        <Section title={t("jd.title")}>
          <p className="soft small">{t("jd.lead")}</p>
          <p className="soft small" role="status">{pub ? <>{t("jd.revision")} {pub.revision}. <a className="link" href={"/media.html?id=" + encodeURIComponent(pub.public_id)} target="_blank" rel="noopener">{t("jd.openPage")}</a> {paid ? t("jd.paidReady") : t("jd.paidNot")}</> : pubMsg}</p>
          <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}>
            <button className="pill" type="button" disabled={busy} onClick={() => { if (edited.current && !window.confirm(t("jd.discard"))) return; void reload(true); }}>{t("jd.reload")}</button>
            <button className="pill danger" type="button" disabled={busy || !loaded || !pub} onClick={() => void unpublish()}>{t("jd.unpublish")}</button>
          </div>
        </Section>
        <Section title={t("jd.templates")}>
          <p className="soft small">{t("jd.templatesHelp")}</p>
          <div className="tasks">{Object.entries(templates).map(([k, tpl]) => <button key={k} className="task" type="button" onClick={() => { edited.current = true; setD((x: any) => applyTemplate(x, k)); }}><b>{tpl.label}</b><span className="soft small">{tpl.description}</span></button>)}</div>
        </Section>
        <Section title={t("jd.brand")}>
          {text("name", t("jd.name"))}{text("headline", t("jd.headline"))}
          <Field label={t("jd.welcome")}><textarea className="input" rows={3} maxLength={160} value={d.subtitle ?? ""} onChange={(e) => set({ subtitle: e.target.value })} style={{ padding: "12px 16px", minHeight: 88 }} /></Field>
          <div className="two">
            <Field label={t("jd.palette")}><select className="input" value={d.theme} onChange={(e) => set({ theme: e.target.value })}><option value="charcoal">{t("jd.p.charcoal")}</option><option value="paper">{t("jd.p.paper")}</option><option value="slate">{t("jd.p.slate")}</option></select></Field>
            <Field label={t("jd.accent")}><input className="input colour" type="color" value={d.accent} onChange={(e) => set({ accent: e.target.value })} /></Field>
          </div>
          <Field label={t("jd.layout")}><select className="input" value={d.layout} onChange={(e) => set({ layout: e.target.value })}><option value="lounge">{t("jd.l.lounge")}</option><option value="shelves">{t("jd.l.shelves")}</option></select></Field>
          <Picture label={t("jd.background")} value={d.background} onChange={(v) => set({ background: v })} onError={pictureErr} />
        </Section>
        <Section title={t("jd.homepage")}>
          <Field label={t("jd.density")}><select className="input" value={d.posterDensity} onChange={(e) => set({ posterDensity: e.target.value })}><option value="comfortable">{t("jd.d.comfortable")}</option><option value="compact">{t("jd.d.compact")}</option></select></Field>
          {["movies", "series", "music", "featured"].map((n) => <label className="check" key={n}><span>{t("jd.show." + n)}</span><input type="checkbox" checked={d[n] === true} onChange={(e) => set({ [n]: e.target.checked })} /></label>)}
        </Section>
        <Section title={t("jd.tiles")}>
          <p className="soft small">{t("jd.tilesHelp")}</p>
          {Object.keys(tileDefaults).map((k) => (
            <details className="glass pad" key={k} open><summary>{d.tiles[k].title}</summary><div className="stack">
              <Field label={t("jd.tileTitle")}><input className="input" maxLength={80} value={d.tiles[k].title} onChange={(e) => setTile(k, { title: e.target.value })} /></Field>
              <Field label={t("jd.caption")}><input className="input" maxLength={160} value={d.tiles[k].caption} onChange={(e) => setTile(k, { caption: e.target.value })} /></Field>
              <Picture label={t("jd.tilePicture")} value={d.tiles[k].image} onChange={(v) => setTile(k, { image: v })} onError={pictureErr} />
            </div></details>
          ))}
        </Section>
        <Section title={t("jd.featured")}>
          <p className="soft small">{t("jd.featuredHelp")}</p>
          {posterNames.map((_, i) => (
            <details className="glass pad" key={i}><summary>{t("jd.card")} {i + 1}</summary><div className="stack">
              <Field label={t("jd.sampleTitle")}><input className="input" maxLength={80} value={d.posters[i].title} onChange={(e) => setPoster(i, { title: e.target.value })} /></Field>
              <Picture label={t("jd.previewPicture")} value={d.posters[i].image} onChange={(v) => setPoster(i, { image: v })} onError={pictureErr} />
            </div></details>
          ))}
        </Section>
        <Section title={t("jd.libraries")}>
          <p className="soft small">{t("jd.librariesHelp")}</p>
          <button className="btn quiet" type="button" disabled={busy || connector !== true} onClick={() => void loadLibraries()}>{t("jd.loadLibs")}</button>
          {connector === false && <p className="soft small">{t("jd.noConnector")}</p>}
          {libs.map((l) => <label className="check" key={l.ItemId}><span>{l.Name} <span className="chip">{l.CollectionType}</span></span><input type="checkbox" checked={chosen.includes(l.ItemId!)} onChange={(e) => { if (e.target.checked && chosen.length >= 16) { setLibMsg(t("jd.max16")); return; } edited.current = true; setChosen((c) => (e.target.checked ? [...new Set([...c, l.ItemId!])] : c.filter((x) => x !== l.ItemId))); }} /></label>)}
          {chosen.length > 0 && !libs.length && <p className="soft small">{chosen.length} {t("jd.savedLibs")}</p>}
          {libMsg && <p className="soft small" role="status">{libMsg}</p>}
        </Section>
        <Section title={t("jd.sales")}>
          <p className="soft small">{t("jd.salesHelp")}</p>
          <div className="two">
            <Field label={t("jd.price")}><input className="input" type="number" min={1} max={1000000} step={1} inputMode="numeric" value={d.price} onChange={(e) => set({ price: e.target.value })} /></Field>
            <Field label={t("jd.duration")}><input className="input" type="number" min={1} max={720} step={1} inputMode="numeric" value={d.duration} onChange={(e) => set({ duration: e.target.value })} /></Field>
          </div>
          <h3>{t("jd.payment")}</h3><p className="soft small">{t("jd.paymentHelp")}</p>
          <Field label={t("g.bank")}><input className="input" maxLength={100} value={pay.bank_name} onChange={(e) => setPay({ bank_name: e.target.value })} /></Field>
          <Field label={t("g.acc")}><input className="input" maxLength={100} value={pay.bank_account} onChange={(e) => setPay({ bank_account: e.target.value })} /></Field>
          <Field label={t("g.holder")}><input className="input" maxLength={160} value={pay.bank_holder} onChange={(e) => setPay({ bank_holder: e.target.value })} /></Field>
          <Picture label={t("jd.mmqr")} value={pay.mmqr} onChange={(v) => setPay({ mmqr: v })} onError={pictureErr} />
        </Section>
        {status && <Note kind={/fail|could not|not /i.test(status) ? "warn" : "ok"}>{status}</Note>}
        <button className="btn" type="submit" disabled={busy || !loaded}>{t("jd.publish")}</button>
      </form>
    </div>
  );
}
