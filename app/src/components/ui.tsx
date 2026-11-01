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
export function PageHead({ title, help }: { title: string; help?: string }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  return (
    <header className="pagehead">
      <div className="row">
        <h1>{title}</h1>
        {help ? <button type="button" className="helpbtn" aria-expanded={open} aria-label={t("c.help")} onClick={() => setOpen((o) => !o)}>?</button> : null}
      </div>
      {help && open ? <p className="soft help-text">{help}</p> : null}
    </header>
  );
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
