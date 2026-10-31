import { useEffect, useRef, useState } from "react";
import useSWR from "swr";
import { api, fetcher, isUnauthorized } from "../../lib/api";
import { asArray, type Router } from "../../lib/types";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";

interface Plan { profile: string; steps: string[]; file?: string; version: string; confirmation: string; preview_html?: string; media_host?: string; media_port?: string }
const errText = (e: unknown, f: string) => { const d = (e as { data?: { error?: string; reason?: string } } | null)?.data; return d?.error || d?.reason || (e instanceof Error && !/^http-\d+$/.test(e.message) ? e.message : "") || f; };

/** Installs the saved guest login page on a gateway: review first, then a backed-up, read-back-verified apply. */
export function GuestDeploy() {
  const { t } = useI18n();
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const gateways = asArray<Router>(routers.data?.routers).filter((r) => r.deployment_layer === "MAIN_GATEWAY");
  const [id, setId] = useState(() => { try { return sessionStorage.getItem("mm_selected_router") ?? ""; } catch { return ""; } });
  const [profiles, setProfiles] = useState<string[] | null>(null), [profile, setProfile] = useState("");
  const [plan, setPlan] = useState<Plan | null>(null), [status, setStatus] = useState(""), [busy, setBusy] = useState(false);
  const rev = useRef(0);
  const gw = gateways.find((g) => g.device_id === id) ?? (gateways.length === 1 ? gateways[0] : undefined);

  useEffect(() => {
    setPlan(null); setProfile(""); setProfiles(null); setStatus("");
    if (!gw) return;
    const mine = ++rev.current;
    setStatus(t("gd2.loadingProfiles"));
    api.get<{ profiles?: string[] }>(`/routers/${encodeURIComponent(gw.device_id)}/portal/profiles`).then((r) => {
      if (mine !== rev.current) return;
      const list = asArray<string>(r.profiles); setProfiles(list);
      if (list.length === 1) { setProfile(list[0]); setStatus(t("gd2.profileFound")); } else setStatus(list.length ? "" : t("gd2.noProfile"));
    }).catch((e) => { if (isUnauthorized(e)) throw e; if (mine === rev.current) { setProfiles([]); setStatus(errText(e, t("gd2.connectFirst"))); } });
  }, [gw?.device_id, t]); // eslint-disable-line react-hooks/exhaustive-deps

  async function review() {
    if (!gw || !profile) return;
    setBusy(true); setPlan(null); setStatus(t("gd2.preparing"));
    try { const r = await api.post<{ plan: Plan }>(`/routers/${encodeURIComponent(gw.device_id)}/portal/preview`, { profile, media_host: "", media_port: "" }); setPlan(r.plan); setStatus(t("gd2.reviewReady")); }
    catch (e) { if (isUnauthorized(e)) throw e; setStatus(errText(e, t("gd2.reviewFail"))); }
    setBusy(false);
  }
  async function apply() {
    if (!gw || !plan) return;
    const approved = plan;
    setBusy(true); setPlan(null); setStatus(t("gd2.applying"));
    try {
      const r = await api.post<{ result?: { verified?: boolean; backup?: string } }>(`/routers/${encodeURIComponent(gw.device_id)}/portal/apply`, { profile: approved.profile, version: approved.version, confirmation: approved.confirmation, media_host: approved.media_host || "", media_port: approved.media_port || "" });
      if (!r.result?.verified) throw new Error(t("gd2.notVerified"));
      setStatus(`${t("gd2.verified")} ${r.result.backup ?? ""}. ${t("gd2.testGuest")}`);
    } catch (e) {
      if (isUnauthorized(e)) throw e;
      const rolled = (e as { data?: { result?: { rolled_back?: boolean } } } | null)?.data?.result?.rolled_back;
      setStatus(`${errText(e, t("gd2.applyFail"))} ${rolled ? t("gd2.rolledBack") : t("gd2.checkRouter")}`);
    }
    setBusy(false);
  }

  return (
    <section className="glass pad stack">
      <h2>{t("gd2.title")}</h2><p className="soft small">{t("gd2.lead")}</p>
      {!gateways.length && !routers.isLoading ? <Note>{t("gd2.noGateway")} <a className="link" href="#/hub">{t("hub.title")}</a></Note> : (
        <>
          {gateways.length > 1 && <label className="field"><span>{t("v.f.gateway")}</span><select className="input" value={gw?.device_id ?? ""} disabled={busy} onChange={(e) => { setId(e.target.value); try { sessionStorage.setItem("mm_selected_router", e.target.value); } catch { /* ignore */ } }}><option value="">{t("net.chooseRouter")}</option>{gateways.map((g) => <option key={g.device_id} value={g.device_id}>{g.hardware_name || g.device_id}</option>)}</select></label>}
          <label className="field"><span>{t("gd2.profile")}</span><select className="input" value={profile} disabled={busy || !profiles?.length} onChange={(e) => { setProfile(e.target.value); setPlan(null); setStatus(""); }}><option value="">{profiles === null ? t("gd2.selectGateway") : t("gd2.selectProfile")}</option>{(profiles ?? []).map((p) => <option key={p} value={p}>{p}</option>)}</select></label>
          <p className="soft small">{t("gd2.internetOnly")}</p>
          <button className="btn quiet" type="button" disabled={busy || !profile} onClick={() => void review()}>{t("gd2.review")}</button>
          {plan && (
            <div className="stack" role="region" aria-label={t("gd2.reviewTitle")}>
              <h3>{t("gd2.reviewFor")} {plan.profile}</h3>
              <ol className="steps">{plan.steps.map((s, i) => <li key={i}>{s}</li>)}</ol>
              <p className="soft small">{t("gd2.file")}: {plan.file}. {t("gd2.scope")}</p>
              {typeof plan.preview_html === "string" && <><p className="soft small">{t("gd2.savedDesign")}</p><div className="phone"><iframe className="gp-frame" sandbox="allow-scripts" title={t("gd2.savedDesign")} srcDoc={plan.preview_html} /></div></>}
              <button className="btn" type="button" disabled={busy} onClick={() => void apply()}>{t("gd2.apply")}</button>
            </div>
          )}
          {status && <p className="note" role="status" aria-live="polite">{status}</p>}
        </>
      )}
    </section>
  );
}
