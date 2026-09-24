type Props = {
  values: number[];
  loading?: boolean;
};

/** Seven-day MMK voucher revenue sparkline with sage gradient fill. */
export function RevenueSparkline({ values, loading }: Props) {
  if (loading) {
    return (
      <div
        className="mt-4 h-16 animate-pulse rounded-2xl border border-[color:var(--glass-border)] bg-gradient-to-r from-primary/20 via-primary/10 to-transparent sm:h-20"
        aria-hidden
      />
    );
  }

  const series = values.length >= 2 ? values : [0, 0];
  const w = 100;
  const h = 40;
  const max = Math.max(...series, 1);
  const coords = series.map((v, i) => {
    const x = series.length === 1 ? w / 2 : (i / (series.length - 1)) * w;
    const y = h - (v / max) * (h - 6) - 3;
    return { x, y };
  });
  const line = coords.map((p) => `${p.x},${p.y}`).join(" ");
  const area = `0,${h} ${line} ${w},${h}`;

  return (
    <svg
      viewBox={`0 0 ${w} ${h}`}
      className="mt-4 h-16 w-full rounded-2xl border border-[color:var(--glass-border)] bg-[color:var(--surface-tint)] sm:h-20"
      preserveAspectRatio="none"
      role="img"
      aria-label="Seven-day voucher revenue trend"
    >
      <defs>
        <linearGradient id="mm-revenue-spark-fill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--primary)" stopOpacity="0.38" />
          <stop offset="100%" stopColor="var(--primary)" stopOpacity="0.02" />
        </linearGradient>
      </defs>
      <polygon points={area} fill="url(#mm-revenue-spark-fill)" />
      <polyline
        points={line}
        fill="none"
        stroke="var(--primary)"
        strokeWidth="1.75"
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  );
}
