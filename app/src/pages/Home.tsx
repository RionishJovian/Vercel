import { lazy, Suspense, useState } from "react";
import useSWR from "swr";
import { fetcher } from "../lib/api";
import { useFleet } from "../lib/useFleet";
import { asArray, mmk, num, type Me, type Router, type VoucherReport } from "../lib/types";
import { useI18n } from "../i18n";
import { Note } from "../components/ui";
import { HomePorts } from "./routers/HomePorts";
import { stateOf, type Site, type Conn } from "../lib/siteState";
import { Skeleton } from "../components/Skeleton";

const SiteMap = lazy(() => import("../components/SiteMap"));

/** Stock split from the report's own counts: ready, reserved and used. Nothing is estimated. */
function StockMeter({ rep }: { rep?: VoucherReport }) {
  if (!rep) return null;
  const a = num(rep.available_count) ?? 0, r = num(rep.reserved_count) ?? 0, u = num(rep.activated_count) ?? 0;
  const total = a + r + u;
  if (!total) return null;
  const seg = (n: number, cls: string) => (n ? <i className={cls} style={{ flexGrow: n }} /> : null);
  return <div className="meter slim" role="img" aria-label={`${a} / ${r} / ${u}`}>{seg(a, "s-available")}{seg(r, "s-reserved")}{seg(u, "s-activated")}</div>;
}

export function Home({ me, routerId }: { me: Me; routerId?: string | null }) {
  const { t } = useI18n();
  const report = useSWR<VoucherReport>("/voucher-report", fetcher);
  const fleet = useFleet();
  const routers = useSWR<{ routers?: Router[] }>("/routers", fetcher);
  const sitesRes = useSWR<{ sites?: Site[] }>("/sites", fetcher);
  const connections = useSWR<{ connections?: Conn[] }>("/router-connections", fetcher, { shouldRetryOnError: false });
  const sites = asArray<Site>(sitesRes.data?.sites), conns = asArray<Conn>(connections.data?.connections);
  const [site, setSite] = useState("");
  const [selectedId, setSelectedId] = useState(() => {
    if (routerId) return routerId;
    try { return sessionStorage.getItem("mm_selected_router") ?? ""; } catch { return ""; }
  });
  const name = me.user?.full_name || (me.user?.email ?? "").split("@")[0];

  const rep = report.data;
  const up = num(fleet.data?.up), total = num(fleet.data?.total_peers);
  const list = routers.data ? asArray<Router>(routers.data.routers) : null;
  const selected = list?.find(r => r.device_id === selectedId) ?? (list?.length === 1 ? list[0] : undefined);
  const allUp = total !== null && total > 0 && up === total;

  return (
    <>
      <div className="greet">
        <h1>{t("h.greet")} <span className="accent">{name}</span></h1>
      </div>

      <section className="stack home-map" aria-label={t("home.map")}>
        <Suspense fallback={<Skeleton h={320} />}>
          <SiteMap home sites={sites} states={Object.fromEntries(sites.map(s => [s.site_id, stateOf(s, conns)]))} selected={site} onSelect={id => setSite(id === site ? "" : id)} />
        </Suspense>
        {sitesRes.error && <Note kind="warn">{t("c.loadFailed")}</Note>}
      </section>

      <section className="glass pad stack" aria-labelledby="home-ports-title">
        <h2 id="home-ports-title">{t("r.ports")}</h2>
        {routers.isLoading ? <Skeleton h={180} /> : routers.error ? <><Note kind="warn">{t("c.loadFailed")}</Note><button className="pill" type="button" onClick={() => void routers.mutate()}>{t("c.retry")}</button></> : (
          <>
            {list && list.length > 1 && <label className="field"><span>{t("net.working")}</span>
              <select className="input" value={selected?.device_id ?? ""} onChange={e => {
                setSelectedId(e.target.value);
                try { sessionStorage.setItem("mm_selected_router", e.target.value); } catch { /* ignore */ }
              }}>
                <option value="">{t("net.chooseRouter")}</option>
                {list.map(r => <option key={r.device_id} value={r.device_id}>{r.hardware_name || r.device_id}</option>)}
              </select>
            </label>}
            {selected ? <HomePorts key={selected.device_id} router={selected} /> :
              list?.length === 0 && <p className="note warn">{t("home.noRouters")} <a className="link" href="#/router">{t("home.addRouter")}</a></p>}
          </>
        )}
      </section>

      <div className="home-grid">
        <section className="glass pad">
          <div className="row"><h2>{t("home.routers")}</h2>
            {total === null ? null : <span className={"chip " + (allUp ? "ok" : "warn")}>{allUp ? t("home.allOnline") : t("home.someOffline")}</span>}
          </div>
          {fleet.isLoading ? <Skeleton h={64} /> : total === null ? <p className="note bad" role="status">{t("h.fleetFailed")}</p> : (
            <div className="leds" aria-label={`${up} / ${total}`}>
              {Array.from({ length: Math.min(total, 12) }, (_, i) => <i key={i} className={i < (up ?? 0) ? "on" : "off"} />)}
              <b className="num">{up} / {total}</b>
            </div>
          )}
        </section>
        <section className="glass pad">
          <h2>{t("home.vouchers")}</h2>
          {report.isLoading ? <Skeleton h={64} /> : (
            <>
              {report.error && <p className="note bad" role="status">{t("h.reportFailed")}</p>}
              <p className="num stock-figure">{rep && num(rep.available_count) !== null ? mmk(rep.available_count) : t("c.unavailable")}</p>
              <StockMeter rep={rep} />
            </>
          )}
        </section>
      </div>
    </>
  );
}
