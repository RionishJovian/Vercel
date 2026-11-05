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

/** Initials in a jade gradient disc. The hue shifts a little per name so rows are easy to tell apart. */
export function Avatar({ name }: { name?: string }) {
  const text = (name ?? "").trim();
  const initials = text.split(/[\s@._-]+/).filter(Boolean).slice(0, 2).map((w) => w[0]!.toUpperCase()).join("") || "·";
  let h = 0; for (const c of text) h = (h * 31 + c.charCodeAt(0)) % 40;
  return <span className="avatar" style={{ ["--hue" as string]: 150 + h }} aria-hidden="true">{initials}</span>;
}
