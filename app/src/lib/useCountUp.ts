import { useEffect, useState } from "react";

/** Counts from 0 to `value` once, easing out. Shows the final value straight away when motion is reduced. */
export function useCountUp(value: number | null, ms = 1100): number | null {
  const [shown, setShown] = useState<number | null>(null);
  useEffect(() => {
    if (value === null) { setShown(null); return; }
    if (matchMedia("(prefers-reduced-motion: reduce)").matches) { setShown(value); return; }
    let raf = 0; const t0 = performance.now();
    const tick = (now: number) => {
      const p = Math.min(1, (now - t0) / ms), e = 1 - Math.pow(1 - p, 3);
      setShown(Math.round(value * e));
      if (p < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [value, ms]);
  return shown;
}
