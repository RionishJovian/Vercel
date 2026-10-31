import { lazy, Suspense, useState, type FormEvent } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../lib/api";
import { asArray, num, type Me } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Field, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";

const SiteMap = lazy(() => import("../components/SiteMap"));

interface Site { site_id: string; name: string; location?: string | null; timezone?: string | null; latitude?: number | null; longitude?: number | null; notes?: string | null; device_count?: number }
interface Conn { site_id?: string | null; cloud_status?: string; cloud_last_seen_at?: string }
const EDITORS = ["OWNER", "ADMIN", "DEVELOPER"];
const SELECTED = "mm_selected_site";
const errText = (e: unknown, fallback: string) => { const d = (e as { data?: { error?: string } } | null)?.data; return d?.error || (e instanceof Error ? e.message : "") || fallback; };

/** A path is live only when its last report is recent (5 minutes) and not from the future. */
export function connectionIsFresh(c: Conn, now = Date.now()): boolean {
  const seen = Date.parse(c.cloud_last_seen_at ?? "");
  return String(c.cloud_status ?? "").toLowerCase() === "online" && Number.isFinite(seen) && seen <= now + 30000 && now - seen <= 300000;
}
type PathState = "online" | "offline" | "unassigned";
const stateOf = (site: Site, conns: Conn[]): PathState => { const p = conns.filter((c) => c.site_id === site.site_id); return p.some((c) => connectionIsFresh(c)) ? "online" : p.length ? "offline" : "unassigned"; };

function SiteForm({ site, coords, setCoords, onDone, onCancel }: { site: Site | null; coords: { lat: string; lng: string }; setCoords: (c: { lat: string; lng: string }) => void; onDone: () => void; onCancel: () => void }) {
  const { t } = useI18n();
  const [msg, setMsg] = useState(""), [busy, setBusy] = useState(false);
  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const f = new FormData(e.currentTarget), s = (k: string) => String(f.get(k) ?? "").trim();
    const n = (k: string) => (s(k) === "" ? null : Number(s(k)));
    const body = { name: s("name"), location: s("location") || null, timezone: s("tz") || null, latitude: n("lat"), longitude: n("lng"), notes: s("notes") || null };
    if (!body.name) { setMsg(t("site.nameNeeded")); return; }
    // Half a coordinate is worse than none: require both or neither.
    if ((body.latitude === null) !== (body.longitude === null)) { setMsg(t("site.bothCoords")); return; }
    setBusy(true); setMsg("");
    try { if (site) await api.put("/sites/" + encodeURIComponent(site.site_id), body); else await api.post("/sites", body); onDone(); }
    catch (err) { if (isUnauthorized(err)) throw err; setMsg(errText(err, t("site.saveFail"))); setBusy(false); }
  }
  return (
    <form className="glass pad stack" onSubmit={save}>
      <h2>{site ? t("site.edit") : t("site.new")}</h2>
      <Field label={t("site.name")} htmlFor="s-name"><input className="input" id="s-name" name="name" required maxLength={120} defaultValue={site?.name ?? ""} /></Field>
      <Field label={t("site.location")} htmlFor="s-loc"><input className="input" id="s-loc" name="location" placeholder={t("site.locationHint")} defaultValue={site?.location ?? ""} /></Field>
      <Field label={t("site.tz")} htmlFor="s-tz"><input className="input" id="s-tz" name="tz" defaultValue={site?.timezone ?? "Asia/Yangon"} /></Field>
      <div className="two">
        <Field label={t("site.lat")} htmlFor="s-lat"><input className="input" id="s-lat" name="lat" type="number" step="any" min={-90} max={90} placeholder="16.866" value={coords.lat} onChange={(e) => setCoords({ ...coords, lat: e.target.value })} /></Field>
        <Field label={t("site.lng")} htmlFor="s-lng"><input className="input" id="s-lng" name="lng" type="number" step="any" min={-180} max={180} placeholder="96.195" value={coords.lng} onChange={(e) => setCoords({ ...coords, lng: e.target.value })} /></Field>
      </div>
      <p className="soft small">{t("site.pinHelp")}</p>
      <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill" type="button" onClick={() => setCoords({ lat: "", lng: "" })}>{t("site.clearPin")}</button>
      </div>
      <Field label={t("site.notes")} htmlFor="s-notes"><input className="input" id="s-notes" name="notes" defaultValue={site?.notes ?? ""} /></Field>
      {msg && <Note kind="bad">{msg}</Note>}
      <button className="btn" type="submit" disabled={busy}>{site ? t("site.save") : t("site.create")}</button>
      <button className="btn quiet" type="button" onClick={onCancel}>{t("c.cancel")}</button>
    </form>
  );
}

export function Sites({ me }: { me: Me }) {
  const { t } = useI18n();
  const canEdit = EDITORS.includes(me.user?.user_role ?? "");
  const sitesRes = useSWR<{ sites?: Site[] }>("/sites", fetcher);
  const connRes = useSWR<{ connections?: Conn[] }>("/router-connections", fetcher, { shouldRetryOnError: false });
  const [form, setForm] = useState<{ site: Site | null } | null>(null), [coords, setCoords] = useState({ lat: "", lng: "" });
  const [selected, setSelected] = useState(() => { try { return localStorage.getItem(SELECTED) ?? ""; } catch { return ""; } });
  const [err, setErr] = useState("");
  const [confirmDel, setConfirmDel] = useState("");
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  if (sitesRes.isLoading) return <>{back}<Skeleton h={300} /></>;
  if (sitesRes.error) return <>{back}<Note kind="bad">{t("c.loadFailed")}</Note></>;
  const sites = asArray<Site>(sitesRes.data?.sites), conns = asArray<Conn>(connRes.data?.connections);
  const pick = (id: string) => { setSelected(id); try { localStorage.setItem(SELECTED, id); } catch { /* ignore */ } };
  const sel = sites.find((s) => s.site_id === selected);
  const paths = sel ? conns.filter((c) => c.site_id === sel.site_id) : [];
  const checks: Array<[boolean, string]> = sel ? [[sel.latitude != null && sel.longitude != null, "site.chk.coords"], [(num(sel.device_count) ?? 0) > 0, "site.chk.device"], [paths.length > 0, "site.chk.path"], [paths.some((c) => connectionIsFresh(c)), "site.chk.live"]] : [];
  const done = () => { setForm(null); void sitesRes.mutate(); };
  async function remove(id: string) {
    setErr("");
    try { await api.del("/sites/" + encodeURIComponent(id)); setConfirmDel(""); if (selected === id) pick(""); void sitesRes.mutate(); }
    catch (e) { if (isUnauthorized(e)) throw e; setErr(errText(e, t("site.delFail"))); setConfirmDel(""); }
  }

  return (
    <>
      {back}<h1>{t("title.sites")}</h1><p className="soft">{t("site.lead")}</p>
      {err && <Note kind="bad">{err}</Note>}
      {canEdit && !form && <button className="btn" type="button" onClick={() => { setCoords({ lat: "", lng: "" }); setForm({ site: null }); }}>{t("site.add")}</button>}
      {(sites.some((x) => x.latitude != null) || form) && (
        <Suspense fallback={<Skeleton h={280} />}>
          <SiteMap sites={sites} states={Object.fromEntries(sites.map((x) => [x.site_id, stateOf(x, conns)]))} selected={selected} onSelect={(id) => pick(id === selected ? "" : id)} pick={form ? { lat: coords.lat, lng: coords.lng, onPick: (la, lo) => setCoords({ lat: la.toFixed(6), lng: lo.toFixed(6) }) } : undefined} />
          <p className="soft small">{t("site.mapCredit")}</p>
        </Suspense>
      )}
      {form && <SiteForm site={form.site} coords={coords} setCoords={setCoords} onDone={done} onCancel={() => setForm(null)} />}
      {!sites.length ? <Note>{t("site.none")}</Note> : (
        <div className="stack">
          {sites.map((s) => {
            const st = stateOf(s, conns), count = num(s.device_count) ?? 0;
            return (
              <div className="glass pad stack site-card" key={s.site_id} data-selected={s.site_id === selected}>
                <button className="site-pick" type="button" aria-pressed={s.site_id === selected} onClick={() => pick(s.site_id === selected ? "" : s.site_id)}>
                  <span className={"dot-state " + st} aria-hidden="true" />
                  <span className="stack" style={{ gap: 2, textAlign: "left" }}><b>{s.name}</b><span className="soft small">{s.location || t("site.noLocation")}</span></span>
                  <span className="stack" style={{ gap: 2, textAlign: "right" }}><b className="small num">{count} {t(count === 1 ? "site.device" : "site.devices")}</b><span className="soft small">{t("site.path." + st)}</span></span>
                </button>
                {canEdit && (confirmDel === s.site_id ? (
                  <div className="note warn stack"><p className="small">{t("site.delConfirm")}</p>
                    <div className="row" style={{ justifyContent: "flex-start" }}><button className="pill" type="button" onClick={() => setConfirmDel("")}>{t("c.cancel")}</button><button className="pill danger" type="button" onClick={() => void remove(s.site_id)}>{t("site.delete")}</button></div></div>
                ) : (
                  <div className="row" style={{ justifyContent: "flex-start" }}>
                    <button className="pill" type="button" disabled={!!form} onClick={() => { setCoords({ lat: s.latitude != null ? String(s.latitude) : "", lng: s.longitude != null ? String(s.longitude) : "" }); setForm({ site: s }); }}>{t("site.edit")}</button>
                    <button className="pill danger" type="button" disabled={!!form} onClick={() => setConfirmDel(s.site_id)}>{t("site.delete")}</button>
                  </div>
                ))}
              </div>
            );
          })}
        </div>
      )}
      {sel && (
        <section className="glass pad stack" aria-label={t("site.ready")}>
          <h2>{t("site.ready")}: {sel.name}</h2>
          <ul className="legend">{checks.map(([ok, key]) => <li key={key}><i className={ok ? "s-available" : "s-expired"} /><span>{t(key)}</span><b className={ok ? "tone-ok" : "mute"}>{ok ? t("site.ok") : t("site.missing")}</b></li>)}</ul>
          <a className="btn quiet" href="#/hub">{t("hub.title")}</a>
        </section>
      )}
    </>
  );
}
