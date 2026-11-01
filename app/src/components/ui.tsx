import { useState, type ReactNode } from "react";
import { Icon } from "./icons";
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
