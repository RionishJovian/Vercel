import { type ReactNode } from "react";
import useSWR from "swr";
import { fetcher } from "../lib/api";
import { Icon, Wand, type IconName } from "./icons";
import { useI18n } from "../i18n";
import { useTheme } from "../lib/theme";
import type { Fleet } from "../lib/types";

const TABS: Array<[string, IconName]> = [["home", "home"], ["router", "router"], ["vouchers", "ticket"], ["jellyfin", "play"], ["more", "dots"]];

export function useHealth(): "ok" | "warn" | "bad" | "idle" {
  const { data, error } = useSWR<Fleet>("/fleet/health", fetcher, { refreshInterval: 30000, revalidateOnFocus: true });
  if (error) return "bad";
  const s = data?.overall_state;
  return s === "up" ? "ok" : s === "stale" ? "warn" : s === "offline" ? "bad" : "idle";
}

function Toggles() {
  const { lang, setLang, t } = useI18n();
  const { theme, setTheme } = useTheme();
  return (
    <div className="tools">
      <button type="button" className="iconbtn" aria-label={`${t("app.theme")}: ${theme === "day" ? t("app.day") : t("app.night")}`} onClick={() => setTheme(theme === "day" ? "night" : "day")}>
        <Icon name={theme === "day" ? "sun" : "moon"} />
      </button>
      <div className="seg" role="group" aria-label={t("c.language")}>
        <button type="button" aria-pressed={lang === "my"} onClick={() => setLang("my")}>မြန်မာ</button>
        <button type="button" aria-pressed={lang === "en"} onClick={() => setLang("en")}>EN</button>
      </div>
    </div>
  );
}

export function Shell({ active, children }: { active: string; children: ReactNode }) {
  const { t } = useI18n();
  const health = useHealth();
  return (
    <div className="shell">
      <header className="topbar">
        <div className="brand"><Wand state={health} /><span>Mega Magic</span></div>
        <Toggles />
      </header>
      <main className="view" id="view">{children}</main>
      <nav className="tabbar" aria-label="Main">
        {TABS.map(([key, icon]) => (
          <a key={key} className="tab" href={"#/" + key} aria-current={key === active ? "page" : undefined}>
            <Icon name={icon} />{t("tab." + key)}
          </a>
        ))}
      </nav>
    </div>
  );
}

export { Toggles };
