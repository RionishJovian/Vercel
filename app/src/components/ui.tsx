import { useState, type ReactNode } from "react";
import { Icon, type IconName } from "./icons";
import { useI18n } from "../i18n";

export function Note({ kind = "", children }: { kind?: "" | "bad" | "warn" | "ok"; children: ReactNode }) {
  return <div className={"note " + kind} role={kind === "bad" ? "alert" : "status"}>{children}</div>;
}
export function Field({ label, children, htmlFor }: { label: string; children: ReactNode; htmlFor?: string }) {
  return <label className="field" htmlFor={htmlFor}><span>{label}</span>{children}</label>;
}
export function Pills<T extends string>({ items, current, href, label }: { items: readonly T[]; current: T; href: (k: T) => string; label: (k: T) => string }) {
  return <div className="scroll-x">{items.map((k) => <a key={k} className="pill" href={href(k)} aria-current={k === current ? "page" : undefined}>{label(k)}</a>)}</div>;
}
export function WorkspaceTabs({ items }: { items: Array<{ href: string; label: string; current: boolean }> }) {
  return <div className="tabs-ws">{items.map((i) => <a key={i.href} href={i.href} aria-current={i.current ? "page" : undefined}>{i.label}</a>)}</div>;
}
export function BackLink({ href, children }: { href: string; children: ReactNode }) {
  return <a className="backlink" href={href}><Icon name="back" />{children}</a>;
}

/** Page title with optional Help: the explanation stays one tap away instead of filling the page. */
function HelpTitle({ Tag, title, help }: { Tag: "h1" | "h2" | "h3"; title: string; help?: string }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  return (
    <header className="pagehead">
      <div className="row">
        <Tag>{title}</Tag>
        {help ? <button type="button" className="helpbtn" aria-expanded={open} aria-label={t("c.help")} onClick={() => setOpen((o) => !o)}>?</button> : null}
      </div>
      {help && open ? <p className="soft help-text">{help}</p> : null}
    </header>
  );
}

/** A page title. Its explanation stays behind the ? button, so the page opens on its controls. */
export function PageHead({ title, help }: { title: string; help?: string }) { return <HelpTitle Tag="h1" title={title} help={help} />; }

/** The same for a heading inside a page. */
export function SectionHead({ title, help, level = 2 }: { title: string; help?: string; level?: 2 | 3 }) { return <HelpTitle Tag={level === 3 ? "h3" : "h2"} title={title} help={help} />; }

/** A glass card with a heading. */
export function Section({ title, help, children }: { title: string; help?: string; children: ReactNode }) {
  return <section className="glass pad stack"><SectionHead title={title} help={help} />{children}</section>;
}

/** Help for a group of form fields that has no heading of its own to sit beside. */
export function Hint({ children }: { children: ReactNode }) {
  const { t } = useI18n();
  return <details className="hint"><summary><span className="helpbtn" aria-hidden="true">?</span>{t("c.help")}</summary><p className="soft help-text">{children}</p></details>;
}

/** A small rounded tile with an icon; the tone tints it for status (ok, warn, bad). */
export function IconTile({ name, tone = "" }: { name: IconName; tone?: string }) {
  return <span className="icontile" data-tone={tone || undefined}><Icon name={name} /></span>;
}

/** Initials in a sky gradient disc. The hue shifts a little per name so rows are easy to tell apart. */
export function Avatar({ name }: { name?: string }) {
  const text = (name ?? "").trim();
  const initials = text.split(/[\s@._-]+/).filter(Boolean).slice(0, 2).map((w) => w[0]!.toUpperCase()).join("") || "·";
  let h = 0; for (const c of text) h = (h * 31 + c.charCodeAt(0)) % 40;
  return <span className="avatar" style={{ ["--hue" as string]: 190 + h }} aria-hidden="true">{initials}</span>;
}

/** Status in words and colour. "idle" means no trustworthy evidence — never presented as offline. */
export type Tone = "ok" | "warn" | "bad" | "idle" | "info";
export function StatePill({ tone, children }: { tone: Tone; children: ReactNode }) {
  return <span className={"chip state " + tone}><i className="dot" aria-hidden="true" />{children}</span>;
}

/** "3 min ago" for evidence timestamps, with the exact time on hover and for screen readers. */
export function Ago({ at, prefix }: { at?: string | number | null; prefix?: string }) {
  const { t, lang } = useI18n();
  const ms = typeof at === "number" ? at : Date.parse(at ?? "");
  if (!Number.isFinite(ms)) return <span className="mute small">{prefix ? prefix + " " : ""}{t("ws.never")}</span>;
  const s = Math.max(0, Math.round((Date.now() - ms) / 1000));
  const rtf = new Intl.RelativeTimeFormat(lang === "my" ? "my" : "en", { numeric: "auto" });
  const text = s < 45 ? rtf.format(-s, "second") : s < 3600 ? rtf.format(-Math.round(s / 60), "minute") : s < 86400 ? rtf.format(-Math.round(s / 3600), "hour") : rtf.format(-Math.round(s / 86400), "day");
  const exact = new Date(ms).toLocaleString();
  return <time className="mute small num" dateTime={new Date(ms).toISOString()} title={exact}>{prefix ? prefix + " " : ""}{text}</time>;
}

/** One figure with its label. A null value says "not available" rather than showing zero. */
export function Metric({ label, value, sub, tone }: { label: string; value: ReactNode | null; sub?: ReactNode; tone?: Tone }) {
  const { t } = useI18n();
  return (
    <div className="metric" data-tone={tone}>
      <span className="metric-label">{label}</span>
      <b className="metric-value num">{value ?? <span className="mute metric-na">{t("c.unavailable")}</span>}</b>
      {sub ? <span className="metric-sub num">{sub}</span> : null}
    </div>
  );
}

/** A navigation row: icon, title, one line of context and a chevron. The whole row is the target. */
export function LinkRow({ href, icon, title, detail, badge }: { href: string; icon: IconName; title: string; detail?: ReactNode; badge?: ReactNode }) {
  return (
    <a className="linkrow" href={href}>
      <IconTile name={icon} />
      <span className="linkrow-text"><b>{title}</b>{detail ? <span className="soft small">{detail}</span> : null}</span>
      {badge}
      <Icon name="chevron" className="linkrow-chev" />
    </a>
  );
}

/** The shared failure panel: what failed, what still works, and one recovery action. */
export function Recovery({ title, detail, still, action, onRetry, advanced }: { title: string; detail?: ReactNode; still?: ReactNode; action?: { href: string; label: string }; onRetry?: () => void; advanced?: ReactNode }) {
  const { t } = useI18n();
  return (
    <div className="recovery" role="status">
      <div className="recovery-head"><IconTile name="alert" tone="warn" /><b>{title}</b></div>
      {detail ? <p className="soft small">{detail}</p> : null}
      {still ? <p className="small"><span className="mute">{t("ws.stillWorks")}</span> {still}</p> : null}
      <div className="recovery-actions">
        {action ? <a className="btn small" href={action.href}>{action.label}</a> : null}
        {onRetry ? <button type="button" className="btn small quiet" onClick={onRetry}>{t("c.retry")}</button> : null}
      </div>
      {advanced ? <details className="adv"><summary>{t("ws.advanced")}</summary><div className="small soft">{advanced}</div></details> : null}
    </div>
  );
}

/** Segmented switch between two or three views (e.g. Hotspot / PPPoE, Map / Tree). */
export function Segmented<T extends string>({ value, options, onChange, label }: { value: T; options: Array<[T, string]>; onChange: (v: T) => void; label: string }) {
  return (
    <div className="segmented" role="tablist" aria-label={label}>
      {options.map(([k, l]) => <button key={k} type="button" role="tab" aria-selected={value === k} onClick={() => onChange(k)}>{l}</button>)}
    </div>
  );
}
