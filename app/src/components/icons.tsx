import type { SVGProps } from "react";

// Precision geometric set: 24px grid, 1.7 stroke, round joins. No filled blobs.
const paths = {
  home: "M4 11.2 12 4l8 7.2V19a1 1 0 0 1-1 1h-4.5v-5.5h-5V20H5a1 1 0 0 1-1-1z",
  router: "M3.5 14.5h17V19h-17zM7 16.75h.01M10.5 16.75h.01M8 14.5V9M16 14.5V9M5.5 6.5 8 9M18.5 6.5 16 9",
  ticket: "M3.5 9.2a2.3 2.3 0 0 0 0 4.6V17a1 1 0 0 0 1 1h15a1 1 0 0 0 1-1v-3.2a2.3 2.3 0 0 1 0-4.6V7a1 1 0 0 0-1-1h-15a1 1 0 0 0-1 1zM14.5 6v12",
  play: "M3.5 5.5h17v13h-17zM10 9.2l4.6 2.8L10 14.8z",
  dots: "M5.5 12h.01M12 12h.01M18.5 12h.01",
  back: "M15 5l-7 7 7 7",
  sun: "M12 8.2a3.8 3.8 0 1 0 0 7.6 3.8 3.8 0 0 0 0-7.6zM12 3.5v2M12 18.5v2M3.5 12h2M18.5 12h2M6 6l1.4 1.4M16.6 16.6 18 18M6 18l1.4-1.4M16.6 7.4 18 6",
  moon: "M19.5 14.2A7.5 7.5 0 0 1 9.8 4.5a7.5 7.5 0 1 0 9.7 9.7z",
} as const;
export type IconName = keyof typeof paths;
export function Icon({ name, ...rest }: { name: IconName } & SVGProps<SVGSVGElement>) {
  return <svg viewBox="0 0 24 24" aria-hidden="true" {...rest}><path d={paths[name]} /></svg>;
}

/** The brand wand: a slim stick with a four-point spark. The spark colour carries router health. */
export function Wand({ state }: { state: "ok" | "warn" | "bad" | "idle" }) {
  return (
    <svg className="wand" data-state={state} viewBox="0 0 32 32" aria-hidden="true">
      <line className="stick" x1="6" y1="27" x2="19" y2="14" />
      <path className="tip" d="M23 3 24.7 9.3 31 11l-6.3 1.7L23 19l-1.7-6.3L15 11l6.3-1.7z" />
    </svg>
  );
}
