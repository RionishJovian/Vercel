import { useEffect, useId, useState } from "react";

interface Props { fraction: number; size?: number; children?: React.ReactNode; label: string }

/** A dial with 60 fine ticks and a jade sweep. The sweep animates once on mount; that is the app's one orchestrated moment. */
export function Ring({ fraction, size = 232, children, label }: Props) {
  const id = useId();
  const r = 92, c = 2 * Math.PI * r;
  const clamped = Math.max(0, Math.min(1, fraction));
  const [shown, setShown] = useState(0);
  useEffect(() => { const raf = requestAnimationFrame(() => setShown(clamped)); return () => cancelAnimationFrame(raf); }, [clamped]);
  return (
    <div className="ring" style={{ width: size, height: size }} role="img" aria-label={label}>
      <svg viewBox="0 0 220 220" aria-hidden="true">
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="var(--jade)" /><stop offset="1" stopColor="color-mix(in srgb, var(--jade) 55%, white)" />
          </linearGradient>
        </defs>
        {Array.from({ length: 60 }, (_, i) => {
          const a = (i / 60) * 2 * Math.PI, long = i % 5 === 0;
          const r1 = 108, r2 = long ? 101 : 104;
          return <line key={i} className={long ? "tick long" : "tick"} x1={110 + r1 * Math.sin(a)} y1={110 - r1 * Math.cos(a)} x2={110 + r2 * Math.sin(a)} y2={110 - r2 * Math.cos(a)} />;
        })}
        <circle className="track" cx="110" cy="110" r={r} />
        <circle className="sweep" cx="110" cy="110" r={r} stroke={`url(#${id})`} strokeDasharray={c} strokeDashoffset={c * (1 - shown)} transform="rotate(-90 110 110)" />
      </svg>
      <div className="ring-center">{children}</div>
    </div>
  );
}
