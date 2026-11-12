import { useId } from "react";

/**
 * MegaMagic header mark: a refined wand with a ring of sparkling stars.
 * It is a brand mark, not a status light — health is always shown in words elsewhere.
 * The stars twinkle once on load and then rest; reduced-motion users see them still.
 */
export function BrandMark({ size = 32, animated = true }: { size?: number; animated?: boolean }) {
  const id = useId().replace(/:/g, "");
  return (
    <svg className={"brandmark" + (animated ? " is-animated" : "")} width={size} height={size} viewBox="0 0 48 48" aria-hidden="true" focusable="false">
      <defs>
        <linearGradient id={`${id}-tile`} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#0e8a63" />
          <stop offset="1" stopColor="#053d2c" />
        </linearGradient>
        <linearGradient id={`${id}-stick`} x1="0" y1="1" x2="1" y2="0">
          <stop offset="0" stopColor="#fdfbf5" />
          <stop offset="1" stopColor="#d9f7ea" />
        </linearGradient>
      </defs>
      <rect x="1" y="1" width="46" height="46" rx="14" fill={`url(#${id}-tile)`} />
      <rect x="1.5" y="1.5" width="45" height="45" rx="13.5" fill="none" stroke="rgba(255,255,255,.18)" />
      {/* wand */}
      <g transform="rotate(-45 24 24)">
        <rect x="21.6" y="17" width="4.8" height="25" rx="2.4" fill={`url(#${id}-stick)`} />
        <rect x="21.6" y="17" width="4.8" height="5.2" rx="2.2" fill="#052e22" opacity=".9" />
      </g>
      {/* lead star at the tip */}
      <path className="bm-star bm-s0" d="M31 6.5 32.6 12.4 38.5 14 32.6 15.6 31 21.5 29.4 15.6 23.5 14 29.4 12.4z" fill="#f4d27a" />
      {/* companion sparkles */}
      <path className="bm-star bm-s1" d="M40 22.5 40.8 25.2 43.5 26 40.8 26.8 40 29.5 39.2 26.8 36.5 26 39.2 25.2z" fill="#8ff0c8" />
      <path className="bm-star bm-s2" d="M18 6 18.6 8 20.6 8.6 18.6 9.2 18 11.2 17.4 9.2 15.4 8.6 17.4 8z" fill="#fdfbf5" />
      <circle className="bm-star bm-s3" cx="41" cy="9" r="1.3" fill="#fdfbf5" />
      <circle className="bm-star bm-s4" cx="25" cy="23" r="1" fill="#8ff0c8" />
    </svg>
  );
}

export function Wordmark() {
  return <span className="wordmark">Mega<b>Magic</b></span>;
}
