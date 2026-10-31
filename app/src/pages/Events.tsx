import { useState } from "react";
import useSWR from "swr";
import { fetcher } from "../lib/api";
import { asArray } from "../lib/types";
import { useI18n } from "../i18n";
import { BackLink, Note } from "../components/ui";
import { Skeleton } from "../components/Skeleton";
import { tone } from "./Incidents";

interface Ev { received_at?: string; severity?: string; ai_severity?: string; program?: string; source_ip?: string; message?: string }
interface Res { events?: Ev[]; severities?: string[]; programs?: string[]; total?: number; has_more?: boolean }
const PAGE = 50;
const clamp = (s: string | undefined, n: number) => { const x = String(s ?? "").replace(/\s+/g, " "); return x.length > n ? x.slice(0, n - 1) + "…" : x; };

export function Events() {
  const { t } = useI18n();
  const [f, setF] = useState({ severity: "", program: "", q: "" });
  const [draft, setDraft] = useState(f);
  const [offset, setOffset] = useState(0);
  const qs = new URLSearchParams();
  if (f.severity) qs.set("severity", f.severity); if (f.program) qs.set("program", f.program); if (f.q) qs.set("q", f.q);
  qs.set("limit", String(PAGE)); qs.set("offset", String(offset));
  const { data, error, isLoading } = useSWR<Res>("/syslog-events?" + qs.toString(), fetcher, { keepPreviousData: true });
  const back = <BackLink href="#/more">{t("tab.more")}</BackLink>;
  const events = asArray<Ev>(data?.events);
  const total = data?.total ?? 0, from = total ? offset + 1 : 0, to = offset + events.length;
  const apply = () => { setOffset(0); setF(draft); };
  return (
    <>
      {back}<h1>{t("ev.title")}</h1><p className="soft">{t("ev.lead")}</p>
      <form className="glass pad stack" onSubmit={(e) => { e.preventDefault(); apply(); }}>
        <div className="two">
          <label className="field"><span>{t("ev.severity")}</span><select className="input" value={draft.severity} onChange={(e) => setDraft({ ...draft, severity: e.target.value })}><option value="">{t("ev.allSeverities")}</option>{asArray<string>(data?.severities).map((s) => <option key={s}>{s}</option>)}</select></label>
          <label className="field"><span>{t("ev.program")}</span><select className="input" value={draft.program} onChange={(e) => setDraft({ ...draft, program: e.target.value })}><option value="">{t("ev.allPrograms")}</option>{asArray<string>(data?.programs).map((s) => <option key={s}>{s}</option>)}</select></label>
        </div>
        <label className="field"><span>{t("ev.contains")}</span><input className="input" value={draft.q} placeholder={t("ev.example")} onChange={(e) => setDraft({ ...draft, q: e.target.value })} /></label>
        <div className="row" style={{ justifyContent: "flex-start" }}>
          <button className="btn" type="submit">{t("ev.apply")}</button>
          <button className="btn quiet" type="button" onClick={() => { const z = { severity: "", program: "", q: "" }; setDraft(z); setF(z); setOffset(0); }}>{t("ev.reset")}</button>
        </div>
      </form>
      {isLoading && !data ? <Skeleton h={240} /> : error ? <Note kind="bad">{t("ev.failed")}</Note> : (
        <>
          <p className="soft small" role="status">{t("ev.showing")} {from}–{to} / {total}{f.severity || f.program || f.q ? ` (${t("ev.filtered")})` : ""}</p>
          {!events.length ? <Note>{t("ev.none")}</Note> : (
            <div className="stack">
              {events.map((e, i) => (
                <div className="rowitem evrow" key={i} title={e.message}>
                  <div className="stack" style={{ gap: 2, minWidth: 0 }}>
                    <div className="row" style={{ justifyContent: "flex-start", flexWrap: "wrap" }}><span className={"chip " + tone(e.severity)}>{e.severity || "info"}</span>{e.ai_severity && <span className="soft small">ai:{e.ai_severity}</span>}<span className="soft small num">{e.received_at ? new Date(e.received_at).toLocaleString() : ""}</span></div>
                    <span className="small">{clamp(e.message, 160)}</span>
                    <span className="soft small num">{[e.program, e.source_ip].filter(Boolean).join(" · ")}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
          <div className="row"><button className="pill" type="button" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>{t("ev.newer")}</button><button className="pill" type="button" disabled={!data?.has_more} onClick={() => setOffset(offset + PAGE)}>{t("ev.older")}</button></div>
        </>
      )}
    </>
  );
}
