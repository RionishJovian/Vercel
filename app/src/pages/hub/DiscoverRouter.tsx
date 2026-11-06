import { useState } from "react";
import { useI18n } from "../../i18n";
import { Note } from "../../components/ui";
import { DEFAULT_ROUTER_IP, candidateSource, parsePrivateIPv4 } from "./discovery";

/**
 * Guided, browser-only help to find a router that has never connected to Magic Hub.
 * It makes no network request and changes nothing: it cannot detect routers, so every address it shows is
 * "not verified". Verification only happens through the existing one-paste connection.
 */
export function DiscoverRouter({ onContinue }: { onContinue?: () => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false), [onLan, setOnLan] = useState(false), [ip, setIp] = useState(DEFAULT_ROUTER_IP);
  const address = parsePrivateIPv4(ip);
  const source = candidateSource(ip);
  return (
    <section className="glass pad stack" aria-labelledby="dn-title">
      <div className="row">
        <h3 id="dn-title">{t("dn.title")}</h3>
        <button className="pill" type="button" aria-expanded={open} onClick={() => setOpen((o) => !o)}>{open ? t("dn.hide") : t("dn.show")}</button>
      </div>
      {open && (
        <div className="stack">
          <p className="soft small">{t("dn.lead")}</p>
          <label className="check"><span>{t("dn.onLan")}</span><input type="checkbox" checked={onLan} onChange={(e) => setOnLan(e.target.checked)} /></label>
          {onLan && (
            <div className="stack">
              <label className="field"><span>{t("dn.address")}</span>
                <input className="input num" inputMode="decimal" autoComplete="off" spellCheck={false} value={ip} aria-invalid={address ? undefined : true} onChange={(e) => setIp(e.target.value)} />
              </label>
              {address ? (
                <>
                  <p className="note" role="status"><b className="num">{address}</b> · {t(source === "suggested" ? "dn.suggested" : "dn.manual")} · {t("dn.notVerified")}</p>
                  <a className="btn quiet" href={"http://" + address} target="_blank" rel="noopener noreferrer">{t("dn.open")}</a>
                  <p className="soft small">{t("dn.openHelp")}</p>
                  <button className="btn" type="button" onClick={onContinue}>{t("dn.continue")}</button>
                </>
              ) : <Note kind="warn">{t("dn.badAddress")}</Note>}
            </div>
          )}
          <details className="adv">
            <summary>{t("dn.cannot")}</summary>
            <ul className="stack soft small">
              {["dn.c.guest", "dn.c.isolation", "dn.c.data", "dn.c.moved", "dn.c.noaddr", "dn.c.auto"].map((k) => <li key={k}>{t(k)}</li>)}
            </ul>
          </details>
        </div>
      )}
    </section>
  );
}
