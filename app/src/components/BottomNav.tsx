import { Icon } from "./icons";
import { useI18n } from "../i18n";
import { TABS, type Tab } from "../lib/nav";
import "./BottomNav.css";

/**
 * Phone and tablet bottom navigation: a floating capsule with a filled mint bubble that slides to the
 * selected destination. Same five destinations, routes, icons and labels as the rest of the app (lib/nav.ts).
 * Hidden from 900px, where the left rail takes over (shell.css).
 */
export function BottomNav({ active }: { active: Tab }) {
  const { t } = useI18n();
  const index = TABS.findIndex((x) => x.key === active);
  return (
    <nav className="bnav" aria-label={t("nav.main")} style={{ ["--bnav-count" as string]: TABS.length, ["--bnav-i" as string]: Math.max(0, index) }}>
      {index >= 0 ? <span className="bnav-bubble" aria-hidden="true" /> : null}
      {TABS.map((x) => (
        <a key={x.key} className="bnav-tab" href={"/app/" + x.key} aria-current={x.key === active ? "page" : undefined}>
          <span className="bnav-ico"><Icon name={x.icon} /></span>
          <span className="bnav-label">{t(x.label)}</span>
        </a>
      ))}
    </nav>
  );
}
