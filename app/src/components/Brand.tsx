/**
 * MegaMagic mark: a signal and a spark on a dark tile (owner direction 2026-11-13, replacing the wand tile).
 * Two precision arcs and a node in sky cyan and white say "network"; the four-point spark says "magic".
 * It is a brand mark, not a status light: health is always shown in words elsewhere.
 * The spark twinkles once on load and then rests; reduced-motion users see it still.
 * The same artwork lives in web/assets/images/megamagic-mark.svg and inline in the public pages.
 */
export function BrandMark({ size = 32, animated = true }: { size?: number; animated?: boolean }) {
  return (
    <svg className={"brandmark" + (animated ? " is-animated" : "")} width={size} height={size} viewBox="0 0 48 48" fill="none" aria-hidden="true" focusable="false">
      <rect x="1" y="1" width="46" height="46" rx="12" fill="#0F1420" />
      <rect x="1.5" y="1.5" width="45" height="45" rx="11.5" stroke="#F8FAFC" strokeOpacity=".10" />
      <g transform="translate(-2 2)" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round">
        <path d="M23.91 31.75A9 9 0 0 0 16.25 24.09" stroke="#38BDF8" />
        <path d="M30.84 30.77A16 16 0 0 0 17.23 17.16" stroke="#38BDF8" />
        <circle cx="15" cy="33" r="2.3" fill="#F8FAFC" stroke="none" />
        <path className="bm-spark" d="M33 8.5Q33.9 14.1 39.5 15Q33.9 15.9 33 21.5Q32.1 15.9 26.5 15Q32.1 14.1 33 8.5Z" stroke="#F8FAFC" fill="#F8FAFC" />
      </g>
    </svg>
  );
}

export function Wordmark() {
  return <span className="wordmark">Mega<b>Magic</b></span>;
}
