import type { CSSProperties, ReactNode } from "react";
import { useI18n } from "../i18n";

/**
 * Loading placeholders. Each layout-matched skeleton reuses the CSS class of the component it stands in for
 * (.metric, .linkrow, .step, .svc-card…), so the loaded content takes exactly the same space and the page
 * does not jump. Each group announces "Loading" once to screen readers; the shapes themselves are hidden.
 * The shimmer stops under reduced motion (base.css).
 */

/** One announcement for a loading region; children are decorative. */
export function Loading({ children, className, style, label }: { children: ReactNode; className?: string; style?: CSSProperties; label?: string }) {
  const { t } = useI18n();
  return (
    <div className={"sk-region" + (className ? " " + className : "")} style={style} role="status" aria-busy="true" aria-live="polite">
      <span className="sr-only">{label ?? t("c.loading")}</span>
      <div aria-hidden="true" className="sk-shapes">{children}</div>
    </div>
  );
}

/** A plain block of a known height (older pages). Still announces once. */
export function Skeleton({ h = 120 }: { h?: number }) {
  return <Loading><div className="skeleton" style={{ height: h }} /></Loading>;
}

/** A text-line placeholder sized to a font size, so it occupies a real line box. */
export const Bar = ({ w = "60%", size = "1rem", lh = 1.5 }: { w?: string; size?: string; lh?: number }) => (
  <span className="sk-line" style={{ width: w, fontSize: size, ["--sk-h" as string]: `calc(${size} * ${lh})`, ["--sk-tall" as string]: lh < 1.4 ? 1.3 : 1.17 }}><i className="skeleton" /></span>
);

/** Metric tiles (label · figure · note) in the same grid the page uses. */
export function SkMetrics({ n = 4, cols = "", sub = true }: { n?: number; cols?: "" | "three" | "four"; sub?: boolean }) {
  return (
    <div className={"metrics" + (cols ? " " + cols : "")}>
      {Array.from({ length: n }, (_, i) => (
        <div key={i} className="metric"><Bar w="55%" size=".8rem" /><Bar w="40%" size="1.55rem" lh={1.2} />{sub ? <Bar w="70%" size=".8rem" /> : null}</div>
      ))}
    </div>
  );
}

/** Navigation rows: icon tile, title and one line of detail. */
export function SkRows({ n = 4, pill = false }: { n?: number; pill?: boolean }) {
  return (
    <div className="linklist">
      {Array.from({ length: n }, (_, i) => (
        <div key={i} className="linkrow"><span className="icontile skeleton" /><span className="linkrow-text"><Bar w="45%" /><Bar w="75%" size=".875rem" />{pill ? <span className="chip skeleton sk-chip" /> : null}</span></div>
      ))}
    </div>
  );
}

/** List rows (subscribers, events, changes). */
export function SkList({ n = 3 }: { n?: number }) {
  return (
    <div className="list-table">
      {Array.from({ length: n }, (_, i) => (
        <div key={i} className="sub-row"><Bar w="50%" /><span className="chip skeleton sk-chip" /><span className="meta"><Bar w="80%" size=".84rem" /></span></div>
      ))}
    </div>
  );
}

/** Quick-setup steps. */
export function SkSteps({ n = 7 }: { n?: number }) {
  return (
    <ol className="steps">
      {Array.from({ length: n }, (_, i) => (
        <li key={i} className="step"><span className="step-n skeleton" /><div className="step-body"><div className="row"><Bar w="45%" /><span className="chip skeleton sk-chip" /></div><Bar w="90%" size=".875rem" /><Bar w="60%" size=".875rem" /></div></li>
      ))}
    </ol>
  );
}

/** Page heading: title and lead line. */
export function SkHead({ lead = true }: { lead?: boolean }) {
  return <div className="ws-head"><Bar w="45%" size="clamp(1.6rem, 4.2vw, 2rem)" lh={1.15} />{lead ? <Bar w="85%" /> : null}</div>;
}

/** A service card (Hotspot / PPPoE): header, four metrics, two buttons. */
export function SkServiceCard() {
  return (
    <div className="glass pad svc-card">
      <div className="sk-cardhead"><span className="icontile skeleton" /><Bar w="40%" size="1.15rem" lh={1.15} /></div>
      <SkMetrics n={4} />
      <div className="actions-row"><span className="btn small skeleton sk-btn" /><span className="btn small skeleton sk-btn" /></div>
    </div>
  );
}

/** Whole-page fallback while a screen's code loads. The variant follows the screen's real layout. */
export function PageSkeleton({ variant = "page" }: { variant?: "home" | "hub" | "steps" | "page" }) {
  return (
    <Loading className="sk-page">
      {variant === "home" ? (
        <><SkHead /><div className="ws-grid cols2"><SkServiceCard /><div className="sk-only-wide"><SkServiceCard /></div></div><div className="glass pad stack"><Bar w="30%" size="1.15rem" lh={1.15} /><SkMetrics n={4} cols="four" sub={false} /></div></>
      ) : variant === "steps" ? (
        <><SkHead /><div className="glass pad stack"><Bar w="50%" /><span className="progress" /></div><SkSteps n={5} /></>
      ) : variant === "hub" ? (
        <><SkHead /><div className="glass pad stack"><Bar w="30%" size="1.15rem" lh={1.15} /><SkMetrics n={4} cols="four" /></div><SkRows n={4} /></>
      ) : (
        <><SkHead lead={false} /><div className="glass pad stack"><Bar w="35%" size="1.15rem" lh={1.15} /><Bar w="90%" /><Bar w="70%" /><div className="skeleton" style={{ height: 160 }} /></div></>
      )}
    </Loading>
  );
}
