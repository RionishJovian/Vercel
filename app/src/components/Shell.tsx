import { type ReactNode } from "react";
import { useFleet } from "../lib/useFleet";
import { Icon } from "./icons";
import { useI18n } from "../i18n";
import { MagicDude } from "./MagicDude";
import { useTheme } from "../lib/theme";
import { BrandMark, Wordmark } from "./Brand";
import { RAIL, tabFor } from "../lib/nav";
import { BottomNav } from "./BottomNav";

/** Fleet health in four honest states. "idle" means no evidence, never "offline". */
export function useHealth(): "ok" | "warn" | "bad" | "idle" {
  const { data, error } = useFleet();
  if (error) return "idle";
  if (!data?.total_peers) return "idle";
  const s = data.overall_state;
  return s === "up" ? "ok" : s === "stale" ? "warn" : s === "offline" ? "bad" : "idle";
}

function ThemeToggle() {
  const { t } = useI18n();
  const { theme, setTheme } = useTheme();
  return (
    <button type="button" className="iconbtn" aria-label={`${t("app.theme")}: ${theme === "day" ? t("app.day") : t("app.night")}`} onClick={() => setTheme(theme === "day" ? "night" : "day")}>
      <Icon name={theme === "day" ? "sun" : "moon"} />
    </button>
  );
}

function LangSwitch() {
  const { lang, setLang, t } = useI18n();
  return (
    <div className="seg" role="group" aria-label={t("c.language")}>
      <button type="button" aria-pressed={lang === "my"} onClick={() => setLang("my")}>မြန်မာ</button>
      <button type="button" aria-pressed={lang === "en"} onClick={() => setLang("en")}>EN</button>
    </div>
  );
}

/** Theme and language together, used by sign-in and settings. */
function Toggles() {
  return <div className="tools"><ThemeToggle /><LangSwitch /></div>;
}

export function Shell({ route, business, children }: { route: string; business?: string; children: ReactNode }) {
  const { t } = useI18n();
  const tab = tabFor(route);
  return (
    <div className="shell">
      <a className="skip" href="#view" onClick={(e) => { e.preventDefault(); document.getElementById("view")?.focus(); }}>{t("nav.skip")}</a>
      <aside className="rail" aria-label={t("nav.main")}>
        <a className="rail-brand" href="/app/home" aria-label="MegaMagic"><BrandMark size={36} /><span><Wordmark />{business ? <small>{business}</small> : null}</span></a>
        <nav className="rail-nav">
          {RAIL.map((g) => (
            <div className="rail-group" key={g.label}>
              <p className="rail-label">{t(g.label)}</p>
              {g.items.map((i) => (
                <a key={i.href} href={i.href} className="rail-item" aria-current={i.routes.includes(route) ? "page" : undefined}>
                  <Icon name={i.icon} /><span>{t(i.label)}</span>
                </a>
              ))}
            </div>
          ))}
        </nav>
        <div className="rail-foot"><ThemeToggle /><LangSwitch /></div>
      </aside>
      <header className="topbar">
        <a className="brand" href="/app/home" aria-label="MegaMagic"><BrandMark size={32} /><Wordmark /></a>
        <div className="tools"><MagicDude /><span className="only-phone"><LangSwitch /></span></div>
      </header>
      <main className="view" id="view" tabIndex={-1}>{children}</main>
      <BottomNav active={tab} />
    </div>
  );
}

export { Toggles, ThemeToggle, LangSwitch };
